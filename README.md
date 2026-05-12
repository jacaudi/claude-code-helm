# claude-pod

[![Helm 3](https://img.shields.io/badge/Helm-3.8+-0f1689?logo=helm&logoColor=white)](https://helm.sh/)
[![Kubernetes 1.28+](https://img.shields.io/badge/Kubernetes-1.28+-326ce5?logo=kubernetes&logoColor=white)](https://kubernetes.io/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A container image and Helm chart for running the [Claude Code](https://github.com/anthropics/claude-code) native CLI in Kubernetes (or locally in Docker) as a long-lived pod with a persistent `HOME`.

- **Image**: `ghcr.io/jacaudi/claude-pod` — Debian trixie base + Claude Code native binary + Go + uv + bun + tmux + zsh + standard dev tooling. Multi-arch (`linux/amd64`, `linux/arm64`).
- **Chart**: `oci://ghcr.io/jacaudi/charts/claude-pod` — thin pass-through over the [bjw-s common library chart](https://bjw-s-labs.github.io/helm-charts/docs/common-library/).

---

## Quick start (Kubernetes)

```bash
helm install claude-pod oci://ghcr.io/jacaudi/charts/claude-pod --version 0.6.0
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=claude-pod --timeout=120s
kubectl exec -it deploy/claude-pod -- claude-tmux
```

`claude-tmux` is a wrapper that runs (or reattaches to) Claude Code inside a persistent `tmux` session, so the conversation survives `kubectl exec` disconnects. Detach with `Ctrl-b d`; reattach by re-running the same command.

## Quick start (Local Docker)

```bash
docker volume create claude-home
docker run -d --name claude-pod -v claude-home:/home/claude \
  ghcr.io/jacaudi/claude-pod:latest sleep infinity

docker exec -it claude-pod claude-tmux
```

The named volume preserves `~/.claude` (auth, settings, history) across container restarts. Wipe with `docker volume rm claude-home`.

---

## Prerequisites

### Deploying to Kubernetes

- **Kubernetes 1.28+** — chart's `kubeVersion` floor; older clusters may template OK but aren't tested.
- **Helm 3.8+** — needs OCI registry support (`helm install oci://...`).
- **A `ReadWriteOnce` storage class** for the home PVC. Default size is `5Gi`; override with `--set persistence.home.size=20Gi` (also `--set persistence.home.storageClass=<class>` if your default isn't right).
- **Cluster nodes on `linux/amd64` or `linux/arm64`**. No 32-bit / non-Linux.
- **Outbound network egress** from the pod to:
  - `ghcr.io` (image pull, chart pull)
  - `api.anthropic.com` (Claude Code traffic)
  - Anywhere else you ask Claude to reach (git remotes, MCP servers, etc.).
- **An Anthropic credential** — either an `ANTHROPIC_API_KEY` wired in via secret (see [chart docs](docs/chart.md#credentials)) or just log in interactively on first `kubectl exec`. The PVC keeps the login state.

### Running locally with Docker

- **Docker 24+** with `buildx` (most current installs). Multi-arch base, so Mac M-series works natively on arm64; amd64 hosts work too.
- **Outbound network egress** to `ghcr.io` (image pull) and `api.anthropic.com` (Claude Code traffic).
- **A persistent volume** (`docker volume create claude-home` above) if you want auth/settings/history to survive container removal.

### Building from source

If you want to build the image and chart yourself rather than pull the published artifacts:

- Docker `buildx` (for multi-arch container builds)
- `helm` CLI 3.8+
- `go` 1.24+ (to compile `claude-pod-logger`, embedded into the image — but the multi-stage build does this for you; you don't need Go installed unless you run the binary outside the image)

See [docs/image.md](docs/image.md) and [docs/release.md](docs/release.md) for the build/release plumbing.

---

## Documentation

In-depth docs live under [`docs/`](docs/):

- [**Image internals**](docs/image.md) — base layers, tools, env vars, multi-stage Containerfile, Renovate-tracked versions, building locally.
- [**Chart configuration**](docs/chart.md) — values reference, persistence, credentials (existing secret / chart-managed / interactive), common overrides.
- [**`claude-pod-logger`**](docs/logger.md) — what the binary does as `PID 1`, flags (`--format`, `--tail`, `--verbose`, `--dir`, `--interval`), filter rules, emoji prefixes, alternative shapes if you don't want log streaming.
- [**tmux integration**](docs/tmux.md) — the `claude-tmux` entrypoint, the `/etc/tmux.conf` settings Claude Code wants, agent-teams / split-pane mode.
- [**Release flow**](docs/release.md) — CI/CD pipeline (lint → semantic-release → container → helm-publish), Conventional Commits rules, Renovate-driven dependency bumps, manual workflow dispatch.

---

## Uninstall

```bash
helm uninstall claude-pod
kubectl delete pvc -l app.kubernetes.io/instance=claude-pod   # PVC is not deleted automatically
```

For local Docker: `docker rm -f claude-pod && docker volume rm claude-home`.

---

## Acknowledgements

- **[Chrisbattarbee/claude-code-helm](https://github.com/Chrisbattarbee/claude-code-helm)** — this repo was forked from theirs; the "run Claude Code in a long-lived Kubernetes pod" idea is theirs.
- **[bjw-s-labs/helm-charts](https://github.com/bjw-s-labs/helm-charts)** — the [`common`](https://bjw-s-labs.github.io/helm-charts/docs/common-library/) library chart this one wraps. Saved a few hundred lines of YAML and gave us a sensible, well-maintained values schema for free.
- **[Anthropic](https://www.anthropic.com/)** — for Claude Code itself, the native binary, and the published per-release manifests that make a verified, no-piped-curl install possible.

---

## License

[MIT](LICENSE).
