// claude-pod-init is the container's startup binary. Replaces the
// earlier bash heredocs for claude-pod-init and claude-tmux with a
// single Go binary that:
//
//   - prepares the writable home tree (CLAUDE_WORK_DIR, ~/.claude,
//     ~/.local/bin) so a fresh PVC works on first boot
//   - re-creates the ~/.local/bin/claude symlink Claude Code's
//     "native install" self-check expects
//   - overlays ConfigMap-mounted JSON fragments onto Claude Code's
//     writable state via claude-pod-config (mcp.json, settings.json)
//   - launches `claude` inside a persistent tmux session named
//     "claude", with Claude Code 2.1.x auto mode and remote control
//     enabled by default (see CLAUDE_POD_AUTO / CLAUDE_POD_REMOTE_CONTROL)
//   - on the boot path, execs claude-pod-logger so the pod's PID 1
//     becomes the JSONL log streamer
//
// Two modes, selected by argv[0] (or the explicit `init` / `tmux`
// subcommand):
//
//   - claude-pod-init: boot mode. Starts a *detached* tmux session and
//     execs claude-pod-logger. The pod stays up on the logger; the
//     session runs as a child. Used as the container's command.
//   - claude-tmux: interactive mode. Runs `tmux new-session -A` so a
//     `kubectl exec`/`docker exec` either creates the session or
//     attaches to the existing one started at boot.
//
// Flags:
//
//	--auto / --no-auto              enable/disable --permission-mode auto
//	--remote-control / --no-...     enable/disable --remote-control
//	--work-dir DIR                  override CLAUDE_WORK_DIR
//	--                              everything after is forwarded to claude
//
// Both auto mode and remote control are research-preview features in
// current Claude Code; they default ON here because the container's
// whole reason to exist is unattended remote-driven sessions. Disable
// per-deployment with `CLAUDE_POD_AUTO=0` / `CLAUDE_POD_REMOTE_CONTROL=0`
// in the chart's env, or per-invocation with `claude-tmux --no-auto`.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	tmuxSession       = "claude"
	claudeBinary      = "/usr/local/bin/claude"
	loggerBinary      = "claude-pod-logger"
	configMergeBinary = "claude-pod-config"
	configDir         = "/etc/claude-pod"
)

// mode selects which workflow main runs.
type mode int

const (
	modeInit mode = iota // boot: detached tmux + exec logger
	modeTmux             // interactive: attach-or-create
)

func (m mode) String() string {
	if m == modeTmux {
		return "tmux"
	}
	return "init"
}

// config is the resolved set of knobs after env vars + CLI parsing.
// claudeArgs is the final argv tail to forward to `claude` inside tmux.
type config struct {
	mode          mode
	workDir       string
	auto          bool
	remoteControl bool
	help          bool
	claudeArgs    []string
}

func main() {
	setupLogger()
	code := dispatch(os.Args, os.Environ())
	os.Exit(code)
}

func setupLogger() {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))
}

// dispatch is the testable entry point: pure argv + env in, exit code
// out. main() is just `os.Exit(dispatch(os.Args, os.Environ()))`.
func dispatch(argv, env []string) int {
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "claude-pod-init: missing argv[0]")
		return 2
	}
	m, args, err := selectMode(argv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "claude-pod-init:", err)
		return 2
	}
	cfg, err := parseArgs(m, args, envMap(env))
	if err != nil {
		fmt.Fprintln(os.Stderr, "claude-pod-init:", err)
		usage(os.Stderr)
		return 2
	}
	if cfg.help {
		usage(os.Stdout)
		return 0
	}
	return run(cfg)
}

// selectMode decides which mode to run based on argv[0]'s basename and
// optionally an explicit `init` / `tmux` first positional. Returns the
// remaining args (the explicit subcommand consumed) to be parsed.
func selectMode(argv []string) (mode, []string, error) {
	base := filepath.Base(argv[0])
	def := modeInit
	if base == "claude-tmux" {
		def = modeTmux
	}
	if len(argv) > 1 {
		switch argv[1] {
		case "init":
			return modeInit, argv[2:], nil
		case "tmux":
			return modeTmux, argv[2:], nil
		}
	}
	return def, argv[1:], nil
}

// parseArgs is a deliberately tiny hand-rolled parser instead of the
// stdlib flag package: we need to recognize a small fixed set of our
// own flags, then pass *everything* else (including unknown
// claude-style flags like `--resume` or `--model X`) through to claude
// untouched. flag.Parse would error on unknown flags.
func parseArgs(m mode, args []string, env map[string]string) (config, error) {
	cfg := config{
		mode:          m,
		workDir:       envOr(env, "CLAUDE_WORK_DIR", defaultWorkDir(env)),
		auto:          envBool(env, "CLAUDE_POD_AUTO", true),
		remoteControl: envBool(env, "CLAUDE_POD_REMOTE_CONTROL", true),
	}

	var passthrough []string
	i := 0
	for i < len(args) {
		a := args[i]
		switch {
		case a == "--":
			passthrough = append(passthrough, args[i+1:]...)
			i = len(args)
		case a == "--auto":
			cfg.auto = true
		case a == "--no-auto":
			cfg.auto = false
		case a == "--remote-control", a == "--rc":
			cfg.remoteControl = true
		case a == "--no-remote-control", a == "--no-rc":
			cfg.remoteControl = false
		case a == "--work-dir":
			if i+1 >= len(args) {
				return cfg, errors.New("--work-dir requires a value")
			}
			cfg.workDir = args[i+1]
			i++
		case strings.HasPrefix(a, "--work-dir="):
			cfg.workDir = strings.TrimPrefix(a, "--work-dir=")
		case a == "-h", a == "--help":
			cfg.help = true
		default:
			passthrough = append(passthrough, a)
		}
		i++
	}

	cfg.claudeArgs = buildClaudeArgs(cfg, passthrough)
	return cfg, nil
}

// buildClaudeArgs prepends the auto / remote-control flags (when
// enabled) and appends user-supplied passthrough args. The order
// matters only in that our flags are visible first in `ps`, which
// makes the running session easy to identify.
func buildClaudeArgs(cfg config, passthrough []string) []string {
	var out []string
	if cfg.auto {
		out = append(out, "--permission-mode", "auto")
	}
	if cfg.remoteControl {
		out = append(out, "--remote-control")
	}
	out = append(out, passthrough...)
	return out
}

func usage(w *os.File) {
	fmt.Fprintln(w, `usage: claude-pod-init [init|tmux] [flags] [-- claude-args...]
       claude-tmux             [flags] [-- claude-args...]

  init   start a detached tmux session, then exec claude-pod-logger
         (the container's default boot path; PID 1 becomes the logger)
  tmux   attach to the existing "claude" tmux session, or create one
         (the interactive entry; what kubectl/docker exec runs)

Flags (any unrecognized arg is forwarded to claude):
  --auto / --no-auto             enable / disable --permission-mode auto
                                 (default: $CLAUDE_POD_AUTO, else on)
  --remote-control, --rc         enable --remote-control
  --no-remote-control, --no-rc   disable --remote-control
                                 (default: $CLAUDE_POD_REMOTE_CONTROL, else on)
  --work-dir DIR                 tmux session cwd (default: $CLAUDE_WORK_DIR
                                 or $HOME/projects)
  -h, --help                     print this message

Both auto and remote-control are Claude Code research-preview features;
their plan-gating may differ across Pro / Max / Team / Enterprise plans.`)
}

// run executes the resolved config. In init mode it returns the
// logger's exit code (via execLogger, which only returns on error
// because it execs); in tmux mode it likewise either execs tmux or
// returns an error code.
func run(cfg config) int {
	slog.Info("starting",
		"mode", cfg.mode,
		"work_dir", cfg.workDir,
		"auto", cfg.auto,
		"remote_control", cfg.remoteControl,
		"claude_args", cfg.claudeArgs,
	)

	if err := setupDirs(cfg.workDir); err != nil {
		slog.Error("setup dirs", "err", err)
		return 1
	}
	if err := ensureSymlink(); err != nil {
		// Symlink failure is non-fatal: `claude` resolves through PATH
		// regardless; only the self-check displays a warning. Log and
		// keep going.
		slog.Warn("ensure symlink", "err", err)
	}

	switch cfg.mode {
	case modeInit:
		overlayConfigs()
		if err := startDetachedTmux(cfg); err != nil {
			slog.Error("start tmux", "err", err)
			return 1
		}
		return execLogger()
	case modeTmux:
		return execAttachTmux(cfg)
	}
	return 0
}

func setupDirs(workDir string) error {
	home := homeDir()
	for _, d := range []string{workDir, filepath.Join(home, ".claude"), filepath.Join(home, ".local", "bin")} {
		// 0o700 matches Claude Code's own ~/.claude perms; the
		// projects dir matches the bash version's implicit umask
		// behaviour (0o755 in the script). We pick 0o755 for the
		// work dir to keep parity, 0o700 for the claude-private dirs.
		perm := os.FileMode(0o700)
		if d == workDir {
			perm = 0o755
		}
		if err := os.MkdirAll(d, perm); err != nil && !errors.Is(err, fs.ErrPermission) {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

// ensureSymlink creates ~/.local/bin/claude -> /usr/local/bin/claude
// if it doesn't already exist. Mirrors the `[ -L X ] || ln -sf` guard
// in the bash version: don't clobber a symlink the user retargeted.
func ensureSymlink() error {
	link := filepath.Join(homeDir(), ".local", "bin", "claude")
	if _, err := os.Lstat(link); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Symlink(claudeBinary, link)
}

// overlayConfigs is best-effort: any failure is logged and ignored so
// a bad ConfigMap can't keep the pod from starting. Matches the
// `|| true` semantics of the bash version.
func overlayConfigs() {
	pairs := []struct{ src, dst string }{
		{filepath.Join(configDir, "mcp.json"), filepath.Join(homeDir(), ".claude.json")},
		{filepath.Join(configDir, "settings.json"), filepath.Join(homeDir(), ".claude", "settings.json")},
	}
	for _, p := range pairs {
		if _, err := os.Stat(p.src); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		// #nosec G204 -- src/dst are hardcoded; binary name is a constant.
		cmd := exec.Command(configMergeBinary, "merge", p.src, p.dst)
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			slog.Warn("config merge failed", "src", p.src, "dst", p.dst, "err", err)
		}
	}
}

// startDetachedTmux is a no-op if a session named `tmuxSession`
// already exists (e.g. claude-pod-init re-running after a crash with a
// surviving session). Otherwise launches `claude` detached.
func startDetachedTmux(cfg config) error {
	// #nosec G204 -- session name is a constant.
	if exec.Command("tmux", "has-session", "-t", tmuxSession).Run() == nil {
		slog.Info("tmux session already exists, not relaunching", "session", tmuxSession)
		return nil
	}
	args := append([]string{"new-session", "-d", "-s", tmuxSession, "-c", cfg.workDir, "claude"}, cfg.claudeArgs...)
	// #nosec G204 -- args are flag-controlled with passthrough for
	// claude itself; this is the same surface as the bash version.
	cmd := exec.Command("tmux", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// execAttachTmux replaces the current process with tmux, so the
// terminal is fully owned by tmux (kubectl exec disconnects map to
// tmux client disconnects, not session destruction). Equivalent to
// the bash `exec tmux new-session -A ...`.
func execAttachTmux(cfg config) int {
	path, err := exec.LookPath("tmux")
	if err != nil {
		slog.Error("tmux not found", "err", err)
		return 1
	}
	args := append([]string{path, "new-session", "-A", "-s", tmuxSession, "-c", cfg.workDir, "claude"}, cfg.claudeArgs...)
	if err := syscall.Exec(path, args, os.Environ()); err != nil {
		slog.Error("exec tmux", "err", err)
		return 1
	}
	return 0 // unreachable on success
}

// execLogger hands PID 1 off to claude-pod-logger. Matches the
// `exec claude-pod-logger` in the bash version. The logger has its
// own flags but the boot path passes none: the container's command
// can be overridden directly in the chart if logger flags are needed.
func execLogger() int {
	path, err := exec.LookPath(loggerBinary)
	if err != nil {
		slog.Error("logger not found", "err", err)
		return 1
	}
	if err := syscall.Exec(path, []string{path}, os.Environ()); err != nil {
		slog.Error("exec logger", "err", err)
		return 1
	}
	return 0 // unreachable on success
}

// homeDir returns $HOME with the same fallback the bash script
// implicitly relied on (we deploy as user `claude` with $HOME set).
func homeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return "/home/claude"
}

// defaultWorkDir is $HOME/projects, matching the bash version.
func defaultWorkDir(env map[string]string) string {
	home := env["HOME"]
	if home == "" {
		home = "/home/claude"
	}
	return filepath.Join(home, "projects")
}

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if ok {
			m[k] = v
		}
	}
	return m
}

func envOr(env map[string]string, key, fallback string) string {
	if v, ok := env[key]; ok && v != "" {
		return v
	}
	return fallback
}

// envBool parses common boolean forms (1/0, true/false, yes/no, on/off).
// Unparseable or unset returns the fallback.
func envBool(env map[string]string, key string, fallback bool) bool {
	v, ok := env[key]
	if !ok || v == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	}
	// Fall through to strconv as a last resort so "True" / "FALSE"
	// etc. (already lowercased above) is not the only path.
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	return fallback
}
