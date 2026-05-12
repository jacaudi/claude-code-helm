# Changelog

## [0.9.0](https://github.com/jacaudi/claude-pod/compare/v0.8.0...v0.9.0) (2026-05-12)

### Features

* **containerfile:** merge /etc/claude-pod/mcp.json into ~/.claude.json on boot ([b047186](https://github.com/jacaudi/claude-pod/commit/b0471867e1c468d6238ce8817558730763933926))

## [0.8.0](https://github.com/jacaudi/claude-pod/compare/v0.7.1...v0.8.0) (2026-05-12)

### Features

* **chart:** readOnlyRootFilesystem: true + tmpfs at /tmp ([a1b695f](https://github.com/jacaudi/claude-pod/commit/a1b695fff55c0cc7a65ec253a51e61ae941f7e8b))

## [0.7.1](https://github.com/jacaudi/claude-pod/compare/v0.7.0...v0.7.1) (2026-05-12)

### Bug Fixes

* **containerfile:** check dir/symlink existence before creating ([43437e3](https://github.com/jacaudi/claude-pod/commit/43437e3aee26bedd38795ce798cdb90fccbd318c))

## [0.7.0](https://github.com/jacaudi/claude-pod/compare/v0.6.0...v0.7.0) (2026-05-12)

### Features

* **containerfile:** add claude-pod-init entrypoint, launch claude at ~/projects on boot ([62a34cb](https://github.com/jacaudi/claude-pod/commit/62a34cbca3ba75d00515ee6f27e4fade2a51f41b))

## [0.6.0](https://github.com/jacaudi/claude-pod/compare/v0.5.0...v0.6.0) (2026-05-12)

### Features

* **claude-pod-logger:** use 🦀 Clawd for assistant + track helm dep ([677560f](https://github.com/jacaudi/claude-pod/commit/677560f7d588961cdd0aea559c2a70452a00ee91))
* **containerfile:** add bun and bunx via multi-stage copy ([388b7e9](https://github.com/jacaudi/claude-pod/commit/388b7e95060a7e06d9c75e1af9dd0b8f10d3a472))

## [0.5.0](https://github.com/jacaudi/claude-pod/compare/v0.4.0...v0.5.0) (2026-05-12)

### Features

* **claude-pod-logger:** add 👤 user / 🔧 tool / 📝 summary prefixes ([d859254](https://github.com/jacaudi/claude-pod/commit/d859254a5ec1a144637cbb628702b4673270e371))
* **claude-pod-logger:** filter noise and render as compact text ([2a362e6](https://github.com/jacaudi/claude-pod/commit/2a362e61660942f53412e369921111971832f771))

## [0.4.0](https://github.com/jacaudi/claude-pod/compare/v0.3.2...v0.4.0) (2026-05-12)

### Features

* **claude-pod:** add claude-pod-logger and use it as default command ([1966839](https://github.com/jacaudi/claude-pod/commit/1966839093d216c7ce94380789a4f120890a6c96))

## [0.3.2](https://github.com/jacaudi/claude-pod/compare/v0.3.1...v0.3.2) (2026-05-12)

### Bug Fixes

* **containerfile:** ensure ~/.local/bin/claude symlink exists ([f93b33b](https://github.com/jacaudi/claude-pod/commit/f93b33be1f00367d4b56e094f746564a1ddd0cec))

## [0.3.1](https://github.com/jacaudi/claude-pod/compare/v0.3.0...v0.3.1) (2026-05-12)

### Bug Fixes

* **chart:** default image.tag to .Chart.AppVersion ([024e59b](https://github.com/jacaudi/claude-pod/commit/024e59b95c33fcabde47a5d9741a501dc145635d))
* **ci:** allow workflow_dispatch to trigger semantic-release ([c9cd4e7](https://github.com/jacaudi/claude-pod/commit/c9cd4e74df975fdbd5580b55e291189263b7804d))

## [0.3.0](https://github.com/jacaudi/claude-pod/compare/v0.2.2...v0.3.0) (2026-05-12)

### Bug Fixes

* **chart:** vendor bjw-s common dependency ([9474bbc](https://github.com/jacaudi/claude-pod/commit/9474bbc3f568e5f60d2ab1dcef290cae2bd670bd))


### Features

* **containerfile:** bump Claude Code, enable agent teams, add .local/bin to PATH ([368f511](https://github.com/jacaudi/claude-pod/commit/368f511f28fb032fe84e386391b8824081ecffb6))

## [0.2.2](https://github.com/jacaudi/claude-pod/compare/v0.2.1...v0.2.2) (2026-05-12)

### Bug Fixes

* **containerfile:** switch base from archlinux to debian for arm64 ([7f2c1b9](https://github.com/jacaudi/claude-pod/commit/7f2c1b9e33dcdbfea9c3fa93befd98a24ab56ee9))

## [0.2.1](https://github.com/jacaudi/claude-pod/compare/v0.2.0...v0.2.1) (2026-05-12)

### Bug Fixes

* **containerfile:** use github-cli package name for gh ([a206728](https://github.com/jacaudi/claude-pod/commit/a2067286ed06e73e279bf7741efb7b6c3f8bc2c4))

## [0.2.0](https://github.com/jacaudi/claude-pod/compare/v0.1.1...v0.2.0) (2026-05-12)

* feat!: rebuild image on Arch, add uv/tmux/zsh, ship reusable CI ([b8e649a](https://github.com/jacaudi/claude-pod/commit/b8e649a5033439026d568ca76bb303efd93c022c))


### BREAKING CHANGES

* chart values shape changed (controllers.claude ->
controllers.app, serviceAccount.create -> serviceAccount.default.enabled,
pod security consolidated under defaultPodOptions). Image tag now
tracks chart version, not the Claude Code version inside (that became
an internal Renovate-tracked ARG).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
