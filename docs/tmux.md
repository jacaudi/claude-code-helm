# tmux integration

Claude Code wants a few specific terminal/tmux settings to render correctly (desktop notifications, progress bar, Shift+Enter for newlines). The image ships those baked in and provides a `claude-tmux` wrapper to launch Claude inside a persistent tmux session.

## `claude-tmux`

A two-line wrapper at `/usr/local/bin/claude-tmux`:

```bash
#!/bin/bash
mkdir -p "$HOME/.claude" "$HOME/.local/bin"
ln -sf /usr/local/bin/claude "$HOME/.local/bin/claude"
exec tmux new-session -A -s claude claude "$@"
```

- `tmux new-session -A` attaches if a session named `claude` already exists, otherwise creates one.
- `kubectl exec` disconnects don't kill the session — `claude` keeps running, you reattach by re-running the same command.
- Extra args pass through to `claude` (`claude-tmux --permission-mode auto`, `claude-tmux --resume`, etc.).
- The `mkdir` + `ln -sf` ensures Claude Code's "native install" self-check finds the binary at `~/.local/bin/claude` (the image's actual binary lives at `/usr/local/bin/claude`).

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

If you don't want tmux at all, exec into the pod with `zsh` and run `claude` directly:

```bash
kubectl exec -it deploy/claude-pod -- zsh
claude
```

The image is shaped so this works — Claude Code's native-install path is set up by the `/etc/claude-pod-init.sh` hook sourced by `/etc/zsh/zshenv` and `/etc/bash.bashrc`. Just understand that a `kubectl exec` disconnect ends the session.
