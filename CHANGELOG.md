# Changelog

## [0.2.0](https://github.com/jacaudi/claude-code-helm/compare/v0.1.1...v0.2.0) (2026-05-12)

* feat!: rebuild image on Arch, add uv/tmux/zsh, ship reusable CI ([b8e649a](https://github.com/jacaudi/claude-code-helm/commit/b8e649a5033439026d568ca76bb303efd93c022c))


### BREAKING CHANGES

* chart values shape changed (controllers.claude ->
controllers.app, serviceAccount.create -> serviceAccount.default.enabled,
pod security consolidated under defaultPodOptions). Image tag now
tracks chart version, not the Claude Code version inside (that became
an internal Renovate-tracked ARG).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
