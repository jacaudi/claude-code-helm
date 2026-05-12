# claude-pod

[![Helm 3](https://img.shields.io/badge/Helm-3.8+-0f1689?logo=helm&logoColor=white)](https://helm.sh/)
[![Kubernetes 1.28+](https://img.shields.io/badge/Kubernetes-1.28+-326ce5?logo=kubernetes&logoColor=white)](https://kubernetes.io/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A container image and Helm chart for running the [Claude Code](https://github.com/anthropics/claude-code) native CLI in Kubernetes (or locally in Docker) as a long-lived pod with a persistent `HOME`.

- **Image**: `ghcr.io/jacaudi/claude-pod` — Debian trixie base + Claude Code native binary + Go + uv + tmux + zsh + standard dev tooling. Multi-arch (`linux/amd64`, `linux/arm64`).
- **Chart**: `oci://ghcr.io/jacaudi/charts/claude-pod` — thin pass-through over the [bjw-s common library chart](https://bjw-s-labs.github.io/helm-charts/docs/common-library/).

---

## Quick start (Kubernetes)

```bash
helm install claude-pod oci://ghcr.io/jacaudi/charts/claude-pod --version 0.3.0
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=claude-pod --timeout=120s
kubectl exec -it deploy/claude-pod -- claude-tmux
```

`claude-tmux` is a tiny wrapper around `tmux new-session -A -s claude claude` that runs (or reattaches to) Claude Code inside a persistent tmux session, so the conversation survives `kubectl exec` disconnects. Detach with `Ctrl-b d`; reattach by re-running the same command. Extra args pass through to `claude`.

---

## Local quick start (Docker)

```bash
docker volume create claude-home
docker run -d --name claude-pod -v claude-home:/home/claude \
  ghcr.io/jacaudi/claude-pod:latest sleep infinity

docker exec -it claude-pod claude-tmux
```

The named volume preserves `~/.claude` (auth, settings, history) across container restarts. Wipe with `docker volume rm claude-home`.

---

## What's in the image

| Tool | Notes |
|---|---|
| `claude` | Native binary, fetched + SHA-verified at build time |
| `go` | Pulled `COPY --from` the official `golang:VERSION-alpine` image |
| `uv` / `uvx` | Pulled `COPY --from=ghcr.io/astral-sh/uv` |
| `tmux` | With Claude Code's recommended config baked into `/etc/tmux.conf` |
| `zsh` | Default shell for the `claude` user (UID 1000) |
| `gh`, `git`, `ripgrep`, `fzf`, `jq`, `less`, `tmux`, `gnupg`, `openssh-client`, `build-essential` | |

Env defaults: `HOME=/home/claude`, `GOPATH=/home/claude/.go`, `~/.local/bin` on `PATH`, `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` so Claude can spawn teammates as tmux panes.

All upstream versions (Debian, alpine, golang, uv, claude-code) are pinned via Renovate-tracked `ARG`s.

---

## Chart

The chart wraps bjw-s common 4.6.2 with no values-shim, so everything `common` supports is supported here — see [`values.yaml`](charts/claude-pod/values.yaml) for the defaults claude-pod ships.

### Persistence

A PVC is mounted at `/home/claude` by default:

- `~/.claude` (auth, config, logs) and login state survive pod restarts.
- Disable with `--set persistence.home.enabled=false`.

### Credentials

**Existing secret:**

```bash
kubectl create secret generic claude-credentials \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-xxx

helm install claude-pod oci://ghcr.io/jacaudi/charts/claude-pod \
  --set-json='controllers.app.containers.app.envFrom=[{"secretRef":{"name":"claude-credentials"}}]'
```

**Chart-managed secret:**

```bash
helm install claude-pod oci://ghcr.io/jacaudi/charts/claude-pod \
  --set secrets.credentials.enabled=true \
  --set secrets.credentials.stringData.ANTHROPIC_API_KEY=sk-ant-xxx \
  --set-json='controllers.app.containers.app.envFrom=[{"secretRef":{"name":"claude-pod-credentials"}}]'
```

**Or just log in interactively** — `claude` writes to `~/.claude`, which is PVC-backed.

---

## Releasing

Push to `main` runs the [CI/CD pipeline](.github/workflows/ci-cd.yml):

1. **Lint** — yaml + helm (the bjw-s common dep is vendored at `charts/claude-pod/charts/`, so lint runs offline).
2. **semantic-release** — Conventional Commits drive the next version. In 0.x mode: `breaking`/`feat` → minor, `fix`/`refactor`/`chore(deps|containerfile|claude-code|chart)` → patch. Bumps `version` + `appVersion` in `Chart.yaml` and `image.tag` in `values.yaml`.
3. **Container** — builds multi-arch and pushes `ghcr.io/jacaudi/claude-pod:vX.Y.Z` + `:latest`.
4. **Helm** — publishes the chart to `oci://ghcr.io/jacaudi/charts/claude-pod:X.Y.Z`.

Image tag, chart version, and chart appVersion all move in lockstep. The Claude Code release shipped inside the image is tracked by the `CLAUDE_CODE_VERSION` ARG and bumped by Renovate (which lands as a `chore(claude-code): ...` commit and triggers a patch release).

PRs run a separate [`pr.yml`](.github/workflows/pr.yml) that lints and builds each architecture natively without pushing.

---

## Uninstall

```bash
helm uninstall claude-pod
kubectl delete pvc -l app.kubernetes.io/instance=claude-pod   # PVC is not deleted automatically
```
