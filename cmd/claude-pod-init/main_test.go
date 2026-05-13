package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSelectMode_FromArgv0(t *testing.T) {
	cases := []struct {
		name     string
		argv     []string
		wantMode mode
		wantArgs []string
	}{
		{"init-basename", []string{"/usr/local/bin/claude-pod-init"}, modeInit, []string{}},
		{"tmux-basename", []string{"/usr/local/bin/claude-tmux"}, modeTmux, []string{}},
		{"unknown-basename-defaults-init", []string{"foo"}, modeInit, []string{}},
		{"init-with-flags", []string{"claude-pod-init", "--auto", "--rc"}, modeInit, []string{"--auto", "--rc"}},
		{"tmux-with-flags", []string{"claude-tmux", "--resume"}, modeTmux, []string{"--resume"}},
		{"explicit-init-subcommand", []string{"claude-tmux", "init", "--auto"}, modeInit, []string{"--auto"}},
		{"explicit-tmux-subcommand", []string{"claude-pod-init", "tmux", "--no-auto"}, modeTmux, []string{"--no-auto"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotMode, gotArgs, err := selectMode(tc.argv)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMode != tc.wantMode {
				t.Errorf("mode = %v, want %v", gotMode, tc.wantMode)
			}
			if !reflect.DeepEqual(gotArgs, tc.wantArgs) {
				t.Errorf("args = %v, want %v", gotArgs, tc.wantArgs)
			}
		})
	}
}

func TestParseArgs_DefaultsAreOn(t *testing.T) {
	// Empty env, no args -- both auto and remote-control default ON
	// because the container's whole point is unattended remote use.
	cfg, err := parseArgs(modeInit, nil, map[string]string{"HOME": "/h"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.auto || !cfg.remoteControl {
		t.Errorf("auto=%v remote=%v, want both true", cfg.auto, cfg.remoteControl)
	}
	want := []string{"--permission-mode", "auto", "--remote-control"}
	if !reflect.DeepEqual(cfg.claudeArgs, want) {
		t.Errorf("claudeArgs = %v, want %v", cfg.claudeArgs, want)
	}
	if cfg.workDir != "/h/projects" {
		t.Errorf("workDir = %q, want /h/projects", cfg.workDir)
	}
}

func TestParseArgs_EnvDisables(t *testing.T) {
	env := map[string]string{
		"HOME":                       "/home/claude",
		"CLAUDE_POD_AUTO":            "0",
		"CLAUDE_POD_REMOTE_CONTROL":  "false",
	}
	cfg, err := parseArgs(modeInit, nil, env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.auto || cfg.remoteControl {
		t.Errorf("auto=%v remote=%v, want both false", cfg.auto, cfg.remoteControl)
	}
	if len(cfg.claudeArgs) != 0 {
		t.Errorf("claudeArgs = %v, want empty", cfg.claudeArgs)
	}
}

func TestParseArgs_FlagsOverrideEnv(t *testing.T) {
	env := map[string]string{
		"CLAUDE_POD_AUTO":           "0",
		"CLAUDE_POD_REMOTE_CONTROL": "0",
	}
	cfg, err := parseArgs(modeTmux, []string{"--auto", "--rc"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.auto || !cfg.remoteControl {
		t.Errorf("flags should override env; got auto=%v remote=%v", cfg.auto, cfg.remoteControl)
	}
}

func TestParseArgs_NoFlagsOverrideEnv(t *testing.T) {
	cfg, err := parseArgs(modeInit, []string{"--no-auto", "--no-rc"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.auto || cfg.remoteControl {
		t.Errorf("--no-* flags ignored; got auto=%v remote=%v", cfg.auto, cfg.remoteControl)
	}
}

func TestParseArgs_Passthrough(t *testing.T) {
	// Unknown args (like claude's --resume or --model X) pass through
	// to claude untouched, after our injected flags.
	cfg, err := parseArgs(modeTmux, []string{"--resume", "--model", "claude-opus-4-7"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--permission-mode", "auto",
		"--remote-control",
		"--resume", "--model", "claude-opus-4-7",
	}
	if !reflect.DeepEqual(cfg.claudeArgs, want) {
		t.Errorf("claudeArgs = %v, want %v", cfg.claudeArgs, want)
	}
}

func TestParseArgs_DoubleDashSeparator(t *testing.T) {
	// After `--`, everything passes through verbatim, including args
	// that *look* like our flags but are meant for claude.
	cfg, err := parseArgs(modeInit, []string{"--no-auto", "--", "--auto", "--foo"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.auto {
		t.Errorf("--no-auto before -- should have disabled auto")
	}
	wantTail := []string{"--auto", "--foo"}
	gotTail := cfg.claudeArgs[len(cfg.claudeArgs)-2:]
	if !reflect.DeepEqual(gotTail, wantTail) {
		t.Errorf("tail after --: got %v, want %v", gotTail, wantTail)
	}
}

func TestParseArgs_WorkDirFlag(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--work-dir", "/tmp/x"}, "/tmp/x"},
		{[]string{"--work-dir=/tmp/y"}, "/tmp/y"},
	}
	for _, tc := range cases {
		cfg, err := parseArgs(modeInit, tc.args, nil)
		if err != nil {
			t.Fatalf("args=%v: %v", tc.args, err)
		}
		if cfg.workDir != tc.want {
			t.Errorf("args=%v: workDir=%q, want %q", tc.args, cfg.workDir, tc.want)
		}
	}
}

func TestParseArgs_WorkDirMissingValue(t *testing.T) {
	_, err := parseArgs(modeInit, []string{"--work-dir"}, nil)
	if err == nil {
		t.Fatal("expected error for --work-dir with no value")
	}
}

func TestParseArgs_HelpFlag(t *testing.T) {
	for _, a := range []string{"-h", "--help"} {
		cfg, err := parseArgs(modeInit, []string{a}, nil)
		if err != nil {
			t.Fatalf("%s: %v", a, err)
		}
		if !cfg.help {
			t.Errorf("%s: help not set", a)
		}
	}
}

func TestParseArgs_EnvWorkDir(t *testing.T) {
	cfg, err := parseArgs(modeInit, nil, map[string]string{
		"HOME":            "/h",
		"CLAUDE_WORK_DIR": "/work/repo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.workDir != "/work/repo" {
		t.Errorf("workDir = %q, want /work/repo", cfg.workDir)
	}
}

func TestBuildClaudeArgs(t *testing.T) {
	cases := []struct {
		name        string
		cfg         config
		passthrough []string
		want        []string
	}{
		{
			"both-on",
			config{auto: true, remoteControl: true},
			nil,
			[]string{"--permission-mode", "auto", "--remote-control"},
		},
		{
			"both-off",
			config{},
			nil,
			nil,
		},
		{
			"auto-only",
			config{auto: true},
			[]string{"--model", "opus"},
			[]string{"--permission-mode", "auto", "--model", "opus"},
		},
		{
			"rc-only-with-passthrough",
			config{remoteControl: true},
			[]string{"--resume"},
			[]string{"--remote-control", "--resume"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildClaudeArgs(tc.cfg, tc.passthrough)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnvBool(t *testing.T) {
	cases := []struct {
		val      string
		fallback bool
		want     bool
	}{
		{"1", false, true},
		{"0", true, false},
		{"true", false, true},
		{"True", false, true},
		{"FALSE", true, false},
		{"yes", false, true},
		{"no", true, false},
		{"on", false, true},
		{"off", true, false},
		{"", true, true},      // unset → fallback
		{"", false, false},    // unset → fallback
		{"banana", true, true}, // unparseable → fallback
		{"banana", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.val+"-"+boolStr(tc.fallback), func(t *testing.T) {
			env := map[string]string{}
			if tc.val != "" {
				env["X"] = tc.val
			}
			got := envBool(env, "X", tc.fallback)
			if got != tc.want {
				t.Errorf("envBool(%q, fallback=%v) = %v, want %v",
					tc.val, tc.fallback, got, tc.want)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "T"
	}
	return "F"
}

func TestEnvOr(t *testing.T) {
	env := map[string]string{"FOO": "bar"}
	if envOr(env, "FOO", "fallback") != "bar" {
		t.Error("FOO present should return its value")
	}
	if envOr(env, "MISSING", "fallback") != "fallback" {
		t.Error("MISSING should return fallback")
	}
	if envOr(map[string]string{"FOO": ""}, "FOO", "fb") != "fb" {
		t.Error("empty value should treat as missing")
	}
}

func TestEnvMap(t *testing.T) {
	got := envMap([]string{"A=1", "B=2", "C=", "malformed"})
	want := map[string]string{"A": "1", "B": "2", "C": ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSetupDirs(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	work := filepath.Join(tmp, "work")

	if err := setupDirs(work); err != nil {
		t.Fatalf("setupDirs: %v", err)
	}
	for _, d := range []string{work, filepath.Join(tmp, ".claude"), filepath.Join(tmp, ".local", "bin")} {
		fi, err := os.Stat(d)
		if err != nil {
			t.Errorf("missing dir %s: %v", d, err)
			continue
		}
		if !fi.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}

func TestEnsureSymlink_Creates(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, ".local", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureSymlink(); err != nil {
		t.Fatalf("ensureSymlink: %v", err)
	}
	target, err := os.Readlink(filepath.Join(tmp, ".local", "bin", "claude"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != claudeBinary {
		t.Errorf("symlink target = %q, want %q", target, claudeBinary)
	}
}

func TestEnsureSymlink_Idempotent(t *testing.T) {
	// If the symlink already exists (even pointing elsewhere), don't
	// clobber it: matches the bash `[ -L X ] || ln -sf` guard.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, ".local", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, ".local", "bin", "claude")
	if err := os.Symlink("/some/other/path", link); err != nil {
		t.Fatal(err)
	}
	if err := ensureSymlink(); err != nil {
		t.Fatalf("ensureSymlink: %v", err)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if target != "/some/other/path" {
		t.Errorf("existing symlink was overwritten: %s", target)
	}
}

func TestModeString(t *testing.T) {
	if modeInit.String() != "init" {
		t.Errorf("modeInit = %q", modeInit.String())
	}
	if modeTmux.String() != "tmux" {
		t.Errorf("modeTmux = %q", modeTmux.String())
	}
}

// TestUsage_PrintsKnownFlags is a smoke test that --help output
// mentions each documented flag, so renaming one without updating the
// help text trips CI instead of shipping a confused UX.
func TestUsage_PrintsKnownFlags(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	usage(pw)
	pw.Close()
	buf := make([]byte, 4096)
	n, _ := pr.Read(buf)
	text := string(buf[:n])
	for _, want := range []string{
		"--auto", "--no-auto",
		"--remote-control", "--no-remote-control",
		"--work-dir",
		"$CLAUDE_POD_AUTO",
		"$CLAUDE_POD_REMOTE_CONTROL",
		"$CLAUDE_WORK_DIR",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("usage text missing %q", want)
		}
	}
}
