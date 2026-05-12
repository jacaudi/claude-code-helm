# claude-pod-logger

The image's literal `CMD` and the program `claude-pod-init` execs into after launching the tmux session — so this is effectively the pod's `PID 1` once init has handed off. Streams Claude Code's per-session conversation JSONL files to stdout so the activity is visible in `kubectl logs` (or `docker logs`).

Independent of tmux, `claude`, or anything else: it polls a directory and emits new lines. The pod stays up whether Claude is running or not.

To run the logger without auto-launching tmux/claude, set the chart's container command to `[claude-pod-logger]` instead of the default `[claude-pod-init]`. See [tmux.md](tmux.md).

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

Each emitted turn is prefixed with `HH:MM:SS` (UTC) parsed from the JSONL's `timestamp` field. A blank line is inserted on role changes so turns are visually separated. Multi-line assistant text is hanging-indented to align with the prefix.

```
18:11:56 👤 list the files here

18:11:57 🔧 LS: {"path":"/home/claude/projects"}
18:11:58 ↩ README.md  main.go  go.mod

18:11:59 🦀 You have three files: README.md, main.go, go.mod. They look like a
         small Go binary plus a README. Want me to read main.go?
```

| Prefix | Meaning |
|---|---|
| `👤` | User prompt |
| `🦀` | Assistant text (Clawd, the Claude Code mascot) |
| `🔧` | Assistant tool use, with the JSON-serialized input truncated to 200 chars |
| `↩` | Tool result (first non-empty line, truncated to 200 chars). Errors render as `↩ ERR: ...`. |
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
