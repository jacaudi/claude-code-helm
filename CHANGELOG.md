# Changelog

## [0.3.1](https://github.com/jacaudi/claude-code-helm/compare/v0.3.0...v0.3.1) (2026-05-12)

### Bug Fixes

* **chart:** default image.tag to .Chart.AppVersion ([024e59b](https://github.com/jacaudi/claude-code-helm/commit/024e59b95c33fcabde47a5d9741a501dc145635d))
* **ci:** allow workflow_dispatch to trigger semantic-release ([c9cd4e7](https://github.com/jacaudi/claude-code-helm/commit/c9cd4e74df975fdbd5580b55e291189263b7804d))

## [0.3.0](https://github.com/jacaudi/claude-code-helm/compare/v0.2.2...v0.3.0) (2026-05-12)

### Bug Fixes

* **chart:** vendor bjw-s common dependency ([9474bbc](https://github.com/jacaudi/claude-code-helm/commit/9474bbc3f568e5f60d2ab1dcef290cae2bd670bd))


### Features

* **containerfile:** bump Claude Code, enable agent teams, add .local/bin to PATH ([368f511](https://github.com/jacaudi/claude-code-helm/commit/368f511f28fb032fe84e386391b8824081ecffb6))

## [0.2.2](https://github.com/jacaudi/claude-code-helm/compare/v0.2.1...v0.2.2) (2026-05-12)

### Bug Fixes

* **containerfile:** switch base from archlinux to debian for arm64 ([7f2c1b9](https://github.com/jacaudi/claude-code-helm/commit/7f2c1b9e33dcdbfea9c3fa93befd98a24ab56ee9))

## [0.2.1](https://github.com/jacaudi/claude-code-helm/compare/v0.2.0...v0.2.1) (2026-05-12)

### Bug Fixes

* **containerfile:** use github-cli package name for gh ([a206728](https://github.com/jacaudi/claude-code-helm/commit/a2067286ed06e73e279bf7741efb7b6c3f8bc2c4))

## [0.2.0](https://github.com/jacaudi/claude-code-helm/compare/v0.1.1...v0.2.0) (2026-05-12)

* feat!: rebuild image on Arch, add uv/tmux/zsh, ship reusable CI ([b8e649a](https://github.com/jacaudi/claude-code-helm/commit/b8e649a5033439026d568ca76bb303efd93c022c))


### BREAKING CHANGES

* chart values shape changed (controllers.claude ->
controllers.app, serviceAccount.create -> serviceAccount.default.enabled,
pod security consolidated under defaultPodOptions). Image tag now
tracks chart version, not the Claude Code version inside (that became
an internal Renovate-tracked ARG).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
