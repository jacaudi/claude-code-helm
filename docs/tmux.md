# tmux integration

Claude Code wants a few specific terminal/tmux settings to render correctly (desktop notifications, progress bar, Shift+Enter for newlines). The image ships those baked in and provides three entrypoints around tmux:

- **`claude-pod-init`** — the chart's default container command. Starts a detached tmux session at boot, then execs the log streamer.
- **`claude-tmux`** — interactive entry. Used by `kubectl exec` to attach to (or create) the session.
- **`/etc/tmux.conf`** — the system-wide tmux settings Claude Code needs.

## Working directory

Both `claude-pod-init` and `claude-tmux` launch `claude` with CWD = `$CLAUDE_WORK_DIR`, default `$HOME/projects`. The directory is `mkdir -p`'d on first run so a fresh PVC works without setup. Override via the chart:

```yaml
controllers:
  app:
    containers:
      app:
        env:
          CLAUDE_WORK_DIR: /home/claude/work/myrepo
```

## `claude-pod-init`

The pod's default `PID 1` (chart `command: [claude-pod-init]`):

```bash
#!/bin/bash
set -u
WORK_DIR="${CLAUDE_WORK_DIR:-$HOME/projects}"
mkdir -p "$WORK_DIR" "$HOME/.claude" "$HOME/.local/bin"
ln -sf /usr/local/bin/claude "$HOME/.local/bin/claude"
if ! tmux has-session -t claude 2>/dev/null; then
  tmux new-session -d -s claude -c "$WORK_DIR" claude
fi
exec claude-pod-logger "$@"
```

Boot sequence: mkdir → symlink → detached tmux session running `claude` → exec `claude-pod-logger` so the log streamer becomes PID 1. The pod stays alive on the logger; the tmux session runs alongside as a child.

No supervision: if `claude` exits, the tmux session ends. Run `claude-tmux` to start a fresh session.

## `claude-tmux`

Interactive entrypoint:

```bash
#!/bin/bash
WORK_DIR="${CLAUDE_WORK_DIR:-$HOME/projects}"
mkdir -p "$WORK_DIR" "$HOME/.claude" "$HOME/.local/bin"
ln -sf /usr/local/bin/claude "$HOME/.local/bin/claude"
exec tmux new-session -A -s claude -c "$WORK_DIR" claude "$@"
```

- `tmux new-session -A` attaches if a session named `claude` already exists (the one `claude-pod-init` started), otherwise creates a new one in `$WORK_DIR`.
- `kubectl exec` disconnects don't kill the session — `claude` keeps running, you reattach by re-running the same command.
- Extra args pass through to `claude` (`claude-tmux --permission-mode auto`, `claude-tmux --resume`, etc.).
- The `mkdir` + `ln -sf` ensures Claude Code's "native install" self-check finds the binary at `~/.local/bin/claude`.

Detach: `Ctrl-b d`. Reattach: `kubectl exec -it deploy/claude-pod -- claude-tmux`.

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
