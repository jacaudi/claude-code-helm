# Release flow

The repo uses `jacaudi/github-actions` reusable workflows ([`ci-cd.yml`](../.github/workflows/ci-cd.yml), [`pr.yml`](../.github/workflows/pr.yml)) plus `semantic-release` driven by Conventional Commits.

## On every push to `main`

Pipeline: **lint → semantic-release → container → helm**.

1. **Lint** — yaml + helm. Helm lint runs offline because the bjw-s common dep is vendored at [`charts/claude-pod/charts/common-X.Y.Z.tgz`](../charts/claude-pod/charts/).
2. **Semantic-release** — analyzes commit messages since the last `vX.Y.Z` tag. If a release is warranted: bumps `version` and `appVersion` in `Chart.yaml`, regenerates `CHANGELOG.md`, commits as `release: vX.Y.Z [skip ci]`, tags, and creates a GitHub Release.
3. **Container** — `docker buildx build --platform linux/amd64,linux/arm64`, pushes `ghcr.io/jacaudi/claude-pod:vX.Y.Z` and `:latest`. Both jobs run only when semantic-release published a new version.
4. **Helm** — packages `charts/claude-pod` with `--version` and `--app-version` set to the new release, pushes to `oci://ghcr.io/jacaudi/charts/claude-pod:vX.Y.Z`. Waits on the container job so the image tag the chart references is guaranteed to exist when the chart lands.

Image tag, chart version, and chart appVersion always move in lockstep. The Claude Code release shipped inside the image is tracked separately by the `CLAUDE_CODE_VERSION` ARG — when Renovate bumps it, the resulting `chore(claude-code): ...` commit triggers a patch release of the whole stack.

## Release rules

[`.releaserc.json`](../.releaserc.json) extends semantic-release's defaults with 0.x-friendly rules:

| Commit | Bump |
|---|---|
| `feat: ...`, `feat!: ...`, any commit with `BREAKING CHANGE:` | **minor** (0.x → next 0.x+1.0) |
| `fix: ...`, `refactor: ...` | **patch** |
| `chore(deps): ...`, `chore(containerfile): ...`, `chore(claude-code): ...`, `chore(chart): ...` | **patch** |
| Everything else (`docs:`, `chore: ...` without a known scope, `ci: ...`, etc.) | no release |

In 0.x mode `breaking` triggers a minor bump (not major) — we stay in 0.x until the API is stable.

## PR validation

[`pr.yml`](../.github/workflows/pr.yml) runs on pull requests:

- **Lint** — same as main.
- **Container builds** — multi-arch (`linux/amd64` on `ubuntu-latest`, `linux/arm64` on `ubuntu-24.04-arm`) with `push: false`. Catches Containerfile breakage before merge.

No release happens from PR branches.

## Manual triggers

`workflow_dispatch` is allowed on `ci-cd.yml` — useful if a push event doesn't register or you want to re-run after fixing an out-of-band issue:

```bash
gh workflow run "CI/CD" --ref main
```

Semantic-release respects its safety guard: if the runner's local checkout of `main` is behind the remote, it skips release to avoid double-versioning.

## Renovate

[`.github/renovate.json`](../.github/renovate.json) tracks:

- **Containerfile ARGs** (Debian, Alpine, Go, uv, bun, Claude Code) via custom regex managers — depName + datasource per-tool. Each bump lands with the `chore(containerfile|claude-code): ...` scope.
- **Chart helm dependency** (bjw-s `common`) via the `helmv3` manager. A `postUpgradeTask` runs `helm dependency update charts/claude-pod` automatically so `Chart.lock` and the vendored `.tgz` get refreshed in the same PR — keeping offline lint green.

Branch protection lets the wall-e-one[bot] GitHub App bypass required checks so semantic-release's `[skip ci]` release commit can push to `main`.

## Required secrets

- `APP_ID` — GitHub App ID for wall-e-one (used by semantic-release to authenticate the `[skip ci]` release commit + tag push).
- `APP_PRIVATE_KEY` — the App's PEM private key.

Set these via your preferred path (`gh secret set` or any tool that reads your secret store). The repo expects them at the *repo* level, not org-wide.
