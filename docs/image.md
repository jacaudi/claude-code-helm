# Image internals

## Tags

- `ghcr.io/jacaudi/claude-pod:latest` — built from `main`, every CI run.
- `ghcr.io/jacaudi/claude-pod:vX.Y.Z` — the same image, semver-pinned. Tag and chart version always move in lockstep (see [release.md](release.md)).

Multi-arch (`linux/amd64`, `linux/arm64`) with SBOM + provenance attestations from `docker/build-push-action`.

## What's in the image

| Tool | Path | Notes |
|---|---|---|
| `claude` | `/usr/local/bin/claude` | Native binary, SHA-verified at build time against Anthropic's per-release `manifest.json`. Symlinked at runtime into `~/.local/bin/claude` so Claude Code's "native install" self-check is happy. |
| `claude-pod-init` | `/usr/local/bin/claude-pod-init` | Pod entrypoint. Starts `claude` in a detached tmux session at `$CLAUDE_WORK_DIR` (default `~/projects`), then execs `claude-pod-logger`. Chart's default `command`. See [tmux.md](tmux.md). |
| `claude-tmux` | `/usr/local/bin/claude-tmux` | Interactive wrapper. Attaches to the `claude-pod-init`-started session, or creates one. Used by `kubectl exec`. See [tmux.md](tmux.md). |
| `claude-pod-logger` | `/usr/local/bin/claude-pod-logger` | Streams Claude's per-session JSONL files to stdout. The image's literal `CMD`, and what `claude-pod-init` execs into. See [logger.md](logger.md). |
| `go` / `gofmt` | `/usr/local/go/bin/` | Pulled `COPY --from=golang:VERSION-alpine`. `GOROOT=/usr/local/go`, `GOPATH=/home/claude/.go`. |
| `uv` / `uvx` | `/usr/local/bin/` | Pulled `COPY --from=ghcr.io/astral-sh/uv`. Use `uv python install` to get a Python (none is pre-installed). |
| `bun` / `bunx` | `/usr/local/bin/` | Pulled `COPY --from=docker.io/oven/bun`. `bunx` is a symlink to `bun`. |
| `tmux` | apt | System-wide `/etc/tmux.conf` ships Claude Code's recommended settings. |
| `zsh` | apt | Default login shell for the `claude` user. |
| `gh`, `git`, `ripgrep`, `fzf`, `jq`, `less`, `openssh-client`, `procps`, `build-essential`, `passwd` | apt | Standard dev tooling. |

## Env defaults

| Variable | Value |
|---|---|
| `HOME` | `/home/claude` |
| `LANG`, `LC_ALL` | `C.UTF-8` |
| `GOROOT` | `/usr/local/go` |
| `GOPATH` | `/home/claude/.go` |
| `PATH` | `/home/claude/.local/bin:/usr/local/go/bin:/home/claude/.go/bin:/usr/local/bin:/usr/bin:/bin` |
| `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` | `1` |
| `DEBIAN_FRONTEND` | `noninteractive` |

User: `claude` (UID/GID 1000). Workdir: `/home/claude`.

## Versioning

Every upstream is pinned via a Renovate-tracked `ARG` in the [Containerfile](../Containerfile):

| ARG | Datasource | What it controls |
|---|---|---|
| `DEBIAN_VERSION` | `docker depName=debian` | Final-stage base (`debian:trixie-XXX-slim`). |
| `ALPINE_VERSION` | `docker depName=alpine` | `claude-fetcher` stage base. |
| `CLAUDE_CODE_VERSION` | `npm depName=@anthropic-ai/claude-code` | Which Claude Code release to download + verify. |
| `GO_VERSION` | `docker depName=golang` | Source image for the Go toolchain (`golang:VERSION-alpine`). |
| `UV_VERSION` | `docker depName=ghcr.io/astral-sh/uv` | Source image for `uv` / `uvx`. |
| `BUN_VERSION` | `docker depName=oven/bun` | Source image for `bun` / `bunx`. |

Renovate config: [`.github/renovate.json`](../.github/renovate.json). Each bump lands as a `chore(containerfile|claude-code|chart): ...` commit and triggers a patch release.

## Building locally

```bash
docker build --platform linux/arm64 -f Containerfile \
  --build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --build-arg VCS_REF="$(git rev-parse HEAD)" \
  -t claude-pod:local .
```

Multi-arch (`linux/amd64,linux/arm64`) goes through `docker buildx`; CI does this automatically — see [release.md](release.md).

## Multi-stage layout

1. **`claude-fetcher`** (`alpine`) — downloads `manifest.json`, verifies the `claude` binary's SHA256 against the per-platform checksum.
2. **`go-source`** (`golang:VERSION-alpine`) — just a `FROM` for `COPY --from=`; no build inside.
3. **`uv-source`** (`ghcr.io/astral-sh/uv:VERSION`) — same; distroless source.
4. **`bun-source`** (`oven/bun:VERSION`) — same.
5. **`logger-build`** (`golang:VERSION-alpine`) — compiles `claude-pod-logger` (`CGO_ENABLED=0 -trimpath -ldflags='-s -w'`).
6. **Final** (`debian:trixie-XXX-slim`) — apt install, `COPY --from=` each prior stage, drop `claude` user, set env.
