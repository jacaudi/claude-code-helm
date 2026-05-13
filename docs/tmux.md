# tmux integration

Claude Code wants a few specific terminal/tmux settings to render correctly (desktop notifications, progress bar, Shift+Enter for newlines). The image ships those baked in and provides one binary plus a system-wide tmux config:

- **`claude-pod-init`** — the chart's default container command. Starts a detached tmux session at boot, then execs the log streamer.
- **`claude-tmux`** — symlink to the same binary; interactive entry used by `kubectl exec` / `docker exec`.
- **`/etc/tmux.conf`** — the system-wide tmux settings Claude Code needs.

Both binary names dispatch to a single Go program (`cmd/claude-pod-init/`); the active mode is selected by `argv[0]`. Same code path, same defaults, same flags.

## Auto mode and remote control (defaults)

The binary launches `claude` with two Claude Code 2.1.x research-preview flags enabled by default:

- **`--permission-mode auto`** — Claude makes tool-call permission decisions on its own. A classifier model reviews actions before execution.
- **`--remote-control`** — bridges the local session to claude.ai/code and the mobile apps. Outbound HTTPS only, no inbound ports.

These default ON because an unattended container is the natural fit for both. Disable per-deployment via env, or per-invocation via flags:

```yaml
# values.yaml
controllers:
  app:
    containers:
      app:
        env:
          CLAUDE_POD_AUTO: "0"
          CLAUDE_POD_REMOTE_CONTROL: "0"
```

```bash
# ad-hoc, plain session for one kubectl exec
kubectl exec -it deploy/claude-pod -- claude-tmux --no-auto --no-rc
```

Plan-gating: auto mode needs Max / Team / Enterprise; remote control is Pro / Max. Disable whichever you're not entitled to.

## Working directory

`claude` launches with CWD = `$CLAUDE_WORK_DIR`, default `$HOME/projects`. The directory is `mkdir -p`'d on first run so a fresh PVC works without setup. Override via the chart:

```yaml
controllers:
  app:
    containers:
      app:
        env:
          CLAUDE_WORK_DIR: /home/claude/work/myrepo
```

## `claude-pod-init` (boot mode)

The pod's default `PID 1` (chart `command: [claude-pod-init]`). Equivalent flow:

1. `mkdir -p` `$CLAUDE_WORK_DIR`, `~/.claude`, `~/.local/bin`
2. `ln -sf /usr/local/bin/claude ~/.local/bin/claude` (Claude's native-install self-check)
3. Overlay `/etc/claude-pod/{mcp,settings}.json` onto Claude's writable state (best-effort)
4. `tmux new-session -d -s claude -c "$WORK_DIR" claude --permission-mode auto --remote-control` (skipped if a session named `claude` already exists)
5. `exec claude-pod-logger` — the JSONL log streamer takes over PID 1

No supervision: if `claude` exits, the tmux session ends. Run `claude-tmux` to start a fresh session. The pod itself stays up on the logger regardless.

## `claude-tmux` (interactive mode)

Interactive entry — same binary, different `argv[0]`. Equivalent to:

```
tmux new-session -A -s claude -c "$WORK_DIR" claude [--permission-mode auto] [--remote-control] "$@"
```

- `-A` attaches if a session named `claude` already exists (the one `claude-pod-init` started), otherwise creates a new one in `$WORK_DIR`.
- `kubectl exec` disconnects don't kill the session — `claude` keeps running, you reattach by re-running the same command.
- Any unrecognized arg (or anything after `--`) passes through to `claude` (`claude-tmux --resume`, `claude-tmux -- --model claude-opus-4-7`, etc.).
- Mode is recoverable from a session-died state: a second `claude-tmux` call creates a new session with the same flag posture as boot.

Detach: `Ctrl-b d`. Reattach: `kubectl exec -it deploy/claude-pod -- claude-tmux`.

## Flag reference

```
claude-pod-init [init|tmux] [flags] [-- claude-args...]
claude-tmux                 [flags] [-- claude-args...]

  --auto / --no-auto             enable / disable --permission-mode auto
  --remote-control, --rc         enable --remote-control
  --no-remote-control, --no-rc   disable --remote-control
  --work-dir DIR                 tmux session cwd (overrides CLAUDE_WORK_DIR)
  -h, --help                     usage
```

Env defaults (read at startup): `CLAUDE_POD_AUTO`, `CLAUDE_POD_REMOTE_CONTROL` (both default on; recognised values: `1/0`, `true/false`, `yes/no`, `on/off`). `CLAUDE_WORK_DIR` for the session cwd.

## `/etc/tmux.conf`

Three settings, system-wide (survives the PVC mount over `/home/claude`):

```tmux
set -g allow-passthrough on
set -s extended-keys on
set -as terminal-features 'xterm*:extkeys'
```

- `allow-passthrough` — lets Claude's desktop notifications and progress bar reach the outer terminal.
- `extended-keys` + the matching `terminal-features` — tmux distinguishes Shift+Enter from Enter, so Claude's "newline without submit" shortcut works.

Source: [Claude Code Terminal Configuration docs](https://code.claude.com/docs/en/terminal-config.md#configure-tmux).

## Agent teams (experimental)

`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` is set in the image, which combined with running inside tmux lets Claude spawn teammates in adjacent tmux panes when you ask it to "create a team."

The three `tmux.conf` settings above are the only tmux config the feature needs. The user-level `~/.claude/settings.json` controls per-teammate display:

```json
{
  "teammateMode": "tmux"   // panes/splits in the active tmux session
}
```

Alternative modes are `auto` (pick based on environment) and the default in-process cycling via Shift+Down.

## Skipping tmux

To disable the auto-launch but keep log streaming, override the chart's container command:

```yaml
controllers:
  app:
    containers:
      app:
        command: [claude-pod-logger]
```

Then `kubectl exec -it deploy/claude-pod -- zsh` and run `claude` directly — Claude Code's native-install path is set up by the `/etc/claude-pod-init.sh` hook (different file — this one's sourced by `/etc/zsh/zshenv` and `/etc/bash.bashrc`, sets up the symlink for interactive shells). A `kubectl exec` disconnect ends the session.
