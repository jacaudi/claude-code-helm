# claude-pod

[![Helm 3](https://img.shields.io/badge/Helm-3.8+-0f1689?logo=helm&logoColor=white)](https://helm.sh/)
[![Kubernetes 1.25+](https://img.shields.io/badge/Kubernetes-1.25+-326ce5?logo=kubernetes&logoColor=white)](https://kubernetes.io/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A container image and Helm chart for running the [Claude Code](https://github.com/anthropics/claude-code) native CLI in Kubernetes as a long-lived pod with a persistent `HOME` directory.

- **Image**: `ghcr.io/jacaudi/claude-pod` — Arch Linux base + Claude Code native binary + Go + uv + standard developer tooling
- **Chart**: `oci://ghcr.io/jacaudi/charts/claude-pod` — wraps the [bjw-s common library chart](https://bjw-s-labs.github.io/helm-charts/docs/common-library/)

---

## Quick Start

```bash
helm install claude-pod oci://ghcr.io/jacaudi/charts/claude-pod --version 0.2.0
```

Wait for the pod and open a shell:

```bash
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=claude-pod --timeout=120s
kubectl exec -it deploy/claude-pod -- claude-tmux
```

`claude-tmux` is a small wrapper that runs (or reattaches to) Claude Code
inside a persistent tmux session, so the conversation survives if the
`kubectl exec` connection drops. Detach with `Ctrl-b d` and reattach by
re-running the same command. Extra args pass through to `claude`.

---

## Prerequisites

- Kubernetes 1.25+
- Helm 3.8+ (OCI support)

---

## Image

The image is built from [`Containerfile`](Containerfile) and pinned via Renovate-tracked `ARG`s for:

- the Arch Linux base tag (`datasource=docker`)
- the Claude Code release (`datasource=npm depName=@anthropic-ai/claude-code`)
- the Go toolchain (`datasource=golang-version`)

### Install path

The Claude Code binary is fetched directly from the official release bucket:

```
https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/manifest.json
https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/${PLATFORM}/claude
```

The build downloads `manifest.json`, reads the SHA256 for the target platform, downloads the binary, verifies the checksum, and installs to `/usr/local/bin/claude`. No piped `curl | bash`.

### Tags

- `ghcr.io/jacaudi/claude-pod:latest` — built from `main`
- `ghcr.io/jacaudi/claude-pod:<X.Y.Z>` — built from a `claude-X.Y.Z` git tag
- `ghcr.io/jacaudi/claude-pod:sha-<shortsha>` — built from a `main` commit

Images are multi-arch (`linux/amd64`, `linux/arm64`) and include SBOM + provenance attestations.

---

## Chart

The chart is a thin wrapper over the [bjw-s common](https://bjw-s-labs.github.io/helm-charts/docs/common-library/) library chart. Everything supported by `common` is available — see [`values.yaml`](charts/claude-pod/values.yaml) for the defaults claude-pod ships.

### Persistence

By default a PVC is mounted at `/home/claude`:

- `~/.claude` (auth, config, logs) persists across restarts
- interactive login state survives restarts

Disable with `--set persistence.home.enabled=false`.

### Credentials

#### 1) Use an existing secret

```bash
kubectl create secret generic claude-credentials \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-xxx

helm install claude-pod oci://ghcr.io/jacaudi/charts/claude-pod \
  --set-json='controllers.app.containers.app.envFrom=[{"secretRef":{"name":"claude-credentials"}}]'
```

#### 2) Let the chart create the secret

```bash
helm install claude-pod oci://ghcr.io/jacaudi/charts/claude-pod \
  --set secrets.credentials.enabled=true \
  --set secrets.credentials.stringData.ANTHROPIC_API_KEY=sk-ant-xxx \
  --set-json='controllers.app.containers.app.envFrom=[{"secretRef":{"name":"claude-credentials"}}]'
```

#### 3) Log in interactively in the pod

`claude` writes login artifacts to `~/.claude`, which persists because `/home/claude` is PVC-backed by default.

---

## Releasing

Push to `main` runs the [CI/CD pipeline](.github/workflows/ci-cd.yml):

1. **Lint** — yaml + helm
2. **semantic-release** — Conventional Commits drive the next version; `breaking` triggers a minor bump (0.x semantics), `feat` minor, `fix`/`refactor`/`chore(deps|containerfile|claude-code|chart)` patch. Bumps `version` and `appVersion` in `charts/claude-pod/Chart.yaml` and the image `tag` in `charts/claude-pod/values.yaml`.
3. **Container** — builds multi-arch image and pushes `ghcr.io/jacaudi/claude-pod:vX.Y.Z` + `:latest`
4. **Helm** — publishes the chart to `oci://ghcr.io/jacaudi/charts/claude-pod:X.Y.Z`

Image tag, chart version, and chart appVersion all move in lockstep. The Claude Code version inside the image is tracked by the `CLAUDE_CODE_VERSION` ARG and bumped by Renovate.

---

## Uninstall

```bash
helm uninstall claude-pod
```

The PVC is not deleted automatically:

```bash
kubectl delete pvc -l app.kubernetes.io/instance=claude-pod
```
