# Chart configuration

The chart at `oci://ghcr.io/jacaudi/charts/claude-pod` is a thin pass-through over the [bjw-s common library chart](https://bjw-s-labs.github.io/helm-charts/docs/common-library/) — every key supported by `common` is supported here. See [`charts/claude-pod/values.yaml`](../charts/claude-pod/values.yaml) for the defaults claude-pod ships.

## Layout

```yaml
controllers:
  app:
    type: deployment
    replicas: 1
    serviceAccount:
      identifier: default
    containers:
      app:
        image:
          repository: ghcr.io/jacaudi/claude-pod
          tag: ""         # defaults to .Chart.AppVersion via the chart helper
        command: [claude-pod-init]   # starts tmux+claude at ~/projects, then claude-pod-logger
        resources: { ... }
        securityContext: { drop ALL caps, no priv escalation }
        tty: true
        stdin: true

defaultPodOptions:
  securityContext:
    runAsUser: 1000
    runAsGroup: 1000
    fsGroup: 1000
    runAsNonRoot: true
    seccompProfile: { type: RuntimeDefault }

serviceAccount:
  default:
    enabled: true

persistence:
  home:
    enabled: true
    type: persistentVolumeClaim
    accessMode: ReadWriteOnce
    size: 5Gi
    globalMounts: [{ path: /home/claude }]

secrets:
  credentials:
    enabled: false   # opt-in chart-managed secret; see Credentials below
```

## Persistence

By default a `ReadWriteOnce` PVC is mounted at `/home/claude`. This preserves `~/.claude` (auth, settings, history), `~/.ssh`, anything the user clones into `~`, and Claude's per-session JSONL files (which `claude-pod-logger` streams).

Common overrides:

```bash
--set persistence.home.size=20Gi
--set persistence.home.storageClass=ceph-block
--set persistence.home.enabled=false              # ephemeral home (re-auth every restart)
```

Mount additional ConfigMaps/secrets via `persistence.<name>` — e.g., the home-ops deployment mounts a rules ConfigMap at `~/.claude/rules/`.

### MCP servers and settings.json overlays

For `settings.json` and MCP server configuration, mount the ConfigMap at `/etc/claude-pod/` instead of overwriting Claude's writable files directly. On boot, `claude-pod-init` runs `claude-pod-config merge` against each fragment, overlaying its top-level keys onto Claude Code's state (so anything Claude itself writes is preserved):

| Source (ConfigMap mount) | Destination | Typical contents |
|---|---|---|
| `/etc/claude-pod/mcp.json` | `~/.claude.json` | `{"mcpServers": {...}}` |
| `/etc/claude-pod/settings.json` | `~/.claude/settings.json` | `{"permissions": {...}, "env": {...}, "model": "...", ...}` |

The merge is a top-level shallow assignment: each key in the source replaces the same key in the destination wholesale (it is not a deep merge — supply the full value you want for each top-level key). Both sources are optional; absent files are skipped.

## Credentials

Three options, pick one:

**1. Existing secret in the cluster**

```bash
kubectl create secret generic claude-credentials \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-xxx

helm install claude-pod oci://ghcr.io/jacaudi/charts/claude-pod \
  --set-json='controllers.app.containers.app.envFrom=[{"secretRef":{"name":"claude-credentials"}}]'
```

**2. Chart-managed secret (values include the key)**

```bash
helm install claude-pod oci://ghcr.io/jacaudi/charts/claude-pod \
  --set secrets.credentials.enabled=true \
  --set secrets.credentials.stringData.ANTHROPIC_API_KEY=sk-ant-xxx \
  --set-json='controllers.app.containers.app.envFrom=[{"secretRef":{"name":"claude-pod-credentials"}}]'
```

**3. Interactive login** — leave `envFrom` empty, `kubectl exec` in, run `claude`, log in normally. Credentials land in `~/.claude/.credentials.json` which is PVC-backed and survives pod restarts.

For a GitOps deployment, the cleanest pattern is to pull `ANTHROPIC_API_KEY` from your secret store via an `ExternalSecret` (1Password / Vault / AWS Secrets Manager / etc.) and wire it through `envFrom`.

## Useful overrides

```bash
# Bigger resources for heavier workloads
--set-json='controllers.app.containers.app.resources={"requests":{"cpu":"2","memory":"4Gi"},"limits":{"cpu":"4","memory":"8Gi"}}'

# Pin a specific timezone
--set-json='controllers.app.containers.app.env={"TZ":"America/New_York"}'

# Switch the pod's PID 1 back to `sleep infinity` (disable log streaming)
--set-json='controllers.app.containers.app.command=["sh","-lc","sleep infinity"]'
```

## Image tag handling

The chart's `image.tag` defaults to `""`. A tiny template helper at [`templates/common.yaml`](../charts/claude-pod/templates/common.yaml) substitutes `.Chart.AppVersion` when the tag is empty, so the deployed image always matches the chart's appVersion — no drift even when CI checks out an older commit. Set `image.tag` explicitly to override.
