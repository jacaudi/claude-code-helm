# claude-pod-logger

The pod's default `PID 1`. It streams Claude Code's per-session conversation JSONL files to stdout so the activity is visible in `kubectl logs` (or `docker logs`).

Independent of tmux, `claude`, or anything else: it polls a directory and emits new lines. The pod stays up whether Claude is running or not.

Source: [`cmd/claude-pod-logger/`](../cmd/claude-pod-logger/). Stdlib-only Go.

## Usage

```
claude-pod-logger [flags]
```

| Flag | Default | Notes |
|---|---|---|
| `--dir` | `$HOME/.claude/projects` | Root directory holding session JSONL files. Scanned recursively. |
| `--interval` | `2s` | Polling interval. |
| `--tail` | `true` | Skip the existing backlog at startup; only stream content appended after the logger starts. Set `--tail=false` to replay everything from the beginning. |
| `--format` | `text` | Output format: `text` (compact, emoji-prefixed) or `json` (filtered JSONL passthrough). |
| `--verbose` | `false` | Disable filtering and rendering entirely; emit every JSONL line verbatim. Useful for debugging. |

Operational state goes to stderr via `slog`; emitted content goes to stdout.

## Filtering

Default (filtered) mode keeps only:

- `type: "user"` — user prompts
- `type: "assistant"` — assistant responses + tool calls
- `type: "summary"` — compaction summaries

Everything else is dropped:

- `attachment` (deferred-tools deltas, skill listings, auto-mode reminders, etc.)
- `system` events (turn duration, etc.)
- `file-history-snapshot`
- Any line with `isMeta: true`

Use `--verbose` to disable filtering entirely.

## Text format (default)

```
👤 hello, can you list the files here?
🦀 Sure — let me check.
🔧 LS
🦀 You have three files: README.md, main.go, go.mod.
📝 [session summary] ...
```

| Prefix | Meaning |
|---|---|
| `👤` | User prompt |
| `🦀` | Assistant text (Clawd, the Claude Code mascot) |
| `🔧` | Assistant tool use |
| `📝` | Session summary |

## JSON format

`--format=json` emits the same filtered set as raw JSONL — one JSON object per line, suitable for log aggregators (Loki, Elasticsearch, etc.) that want structure.

## Robustness

- New files (new conversations) are picked up automatically on the next poll.
- File truncation is handled (position reset to zero).
- Partial trailing lines are NOT emitted — only complete `\n`-terminated lines. Partial content is re-read on the next scan when complete.
- `SIGTERM` / `SIGINT` exit cleanly so K8s pod termination is prompt.

## Disabling

If you don't want the log streaming, override the chart's container command:

```bash
--set-json='controllers.app.containers.app.command=["sh","-lc","sleep infinity"]'
```

`claude-tmux` interactive entry via `kubectl exec` is unaffected by either choice.
