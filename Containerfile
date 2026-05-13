# syntax=docker/dockerfile:1.7

# renovate: datasource=docker depName=debian
ARG DEBIAN_VERSION=trixie-20260505-slim

# renovate: datasource=docker depName=alpine
ARG ALPINE_VERSION=3.21

# renovate: datasource=npm depName=@anthropic-ai/claude-code
ARG CLAUDE_CODE_VERSION=2.1.139

# renovate: datasource=docker depName=golang
ARG GO_VERSION=1.26.3

# renovate: datasource=docker depName=ghcr.io/astral-sh/uv
ARG UV_VERSION=0.11.13

# renovate: datasource=docker depName=oven/bun
ARG BUN_VERSION=1.3.13

############################################
# Stage 1: fetch and verify Claude Code
############################################
FROM public.ecr.aws/docker/library/alpine:${ALPINE_VERSION} AS claude-fetcher

ARG CLAUDE_CODE_VERSION
ARG TARGETARCH

RUN apk add --no-cache curl jq ca-certificates

RUN set -eu; \
    case "${TARGETARCH:-amd64}" in \
      amd64) CC_PLATFORM=linux-x64 ;; \
      arm64) CC_PLATFORM=linux-arm64 ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    base="https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}"; \
    mkdir -p /out; \
    curl -fsSL -o /tmp/manifest.json "${base}/manifest.json"; \
    expected="$(jq -r --arg p "${CC_PLATFORM}" '.platforms[$p].checksum' /tmp/manifest.json)"; \
    if [ -z "${expected}" ] || [ "${expected}" = "null" ]; then \
      echo "no checksum for platform ${CC_PLATFORM} in manifest" >&2; exit 1; \
    fi; \
    curl -fsSL -o /out/claude "${base}/${CC_PLATFORM}/claude"; \
    echo "${expected}  /out/claude" | sha256sum -c -; \
    chmod 0755 /out/claude

############################################
# Stage 2: pin upstream toolchain images
############################################
FROM public.ecr.aws/docker/library/golang:${GO_VERSION}-alpine AS go-source
FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv-source
FROM docker.io/oven/bun:${BUN_VERSION} AS bun-source

############################################
# Stage 3: build claude-pod-logger
############################################
FROM public.ecr.aws/docker/library/golang:${GO_VERSION}-alpine AS logger-build
WORKDIR /src
COPY cmd/claude-pod-logger/go.mod cmd/claude-pod-logger/main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/claude-pod-logger .

############################################
# Stage 3b: build claude-pod-config
############################################
FROM public.ecr.aws/docker/library/golang:${GO_VERSION}-alpine AS config-build
WORKDIR /src
COPY cmd/claude-pod-config/go.mod cmd/claude-pod-config/main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/claude-pod-config .

############################################
# Stage 3c: build claude-pod-init
############################################
FROM public.ecr.aws/docker/library/golang:${GO_VERSION}-alpine AS init-build
WORKDIR /src
COPY cmd/claude-pod-init/go.mod cmd/claude-pod-init/main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/claude-pod-init .

############################################
# Stage 4: final runtime image
############################################
FROM public.ecr.aws/docker/library/debian:${DEBIAN_VERSION}

ARG CLAUDE_CODE_VERSION
ARG GO_VERSION
ARG UV_VERSION
ARG BUN_VERSION
ARG BUILD_DATE
ARG VCS_REF

ENV LANG=C.UTF-8 \
    LC_ALL=C.UTF-8 \
    DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
 && apt-get install --no-install-recommends -y \
      bash \
      build-essential \
      ca-certificates \
      curl \
      fzf \
      gh \
      git \
      jq \
      less \
      openssh-client \
      passwd \
      procps \
      ripgrep \
      tmux \
      zsh \
 && apt-get clean \
 && rm -rf /var/lib/apt/lists/*

# Go toolchain — copied from the official golang image.
COPY --from=go-source /usr/local/go /usr/local/go

# uv / uvx — copied from Astral's distroless image.
COPY --from=uv-source /uv /uvx /usr/local/bin/

# bun — copied from the official oven/bun image. `bunx` ships as a
# symlink to /usr/local/bin/bun, which we recreate after the copy.
COPY --from=bun-source /usr/local/bin/bun /usr/local/bin/bun
RUN ln -sf /usr/local/bin/bun /usr/local/bin/bunx

# Claude Code native binary — verified above.
COPY --from=claude-fetcher /out/claude /usr/local/bin/claude

# claude-pod-logger — streams ~/.claude/projects/**/*.jsonl to stdout so
# Claude activity is visible in `kubectl logs`. tmux-independent.
COPY --from=logger-build /out/claude-pod-logger /usr/local/bin/claude-pod-logger

# claude-pod-config — overlays JSON config fragments mounted from
# Kubernetes ConfigMaps onto Claude Code's writable home-dir files
# (~/.claude.json, ~/.claude/settings.json). Invoked by claude-pod-init.
COPY --from=config-build /out/claude-pod-config /usr/local/bin/claude-pod-config

# claude-pod-init — the container's startup binary. Sets up the
# writable home tree, overlays /etc/claude-pod/*.json fragments onto
# Claude's writable state, launches `claude` inside a persistent tmux
# session named "claude", and execs claude-pod-logger so the pod's
# PID 1 becomes the JSONL log streamer.
#
# Two modes selected by argv[0]:
#   - claude-pod-init: boot path (detached tmux + exec logger).
#   - claude-tmux:     interactive entry (tmux new-session -A) used by
#                      `kubectl exec` / `docker exec`.
#
# Both auto mode (`--permission-mode auto`) and remote control
# (`--remote-control`) are enabled by default; override per-deployment
# with CLAUDE_POD_AUTO=0 / CLAUDE_POD_REMOTE_CONTROL=0 or per-invocation
# with `claude-tmux --no-auto --no-rc`. Extra args pass through to
# claude.
COPY --from=init-build /out/claude-pod-init /usr/local/bin/claude-pod-init
RUN ln -sf /usr/local/bin/claude-pod-init /usr/local/bin/claude-tmux

# System-wide tmux config — recommended settings for Claude Code per
# https://code.claude.com/docs/en/terminal-config.md#configure-tmux
# Lives at /etc to survive the PVC mount over /home/claude.
RUN printf '%s\n' \
      'set -g allow-passthrough on' \
      'set -s extended-keys on' \
      'set -as terminal-features "xterm*:extkeys"' \
      > /etc/tmux.conf

# Same setup for interactive shell entry (`docker exec -it ... zsh`,
# `... bash`), so users who skip claude-tmux still get the symlink.
RUN mkdir -p /etc/zsh \
 && printf '%s\n' \
      '# Claude Code native-install symlink (idempotent)' \
      'if [ -w "$HOME" ] && [ ! -L "$HOME/.local/bin/claude" ]; then' \
      '  mkdir -p "$HOME/.claude" "$HOME/.local/bin" 2>/dev/null' \
      '  ln -sf /usr/local/bin/claude "$HOME/.local/bin/claude" 2>/dev/null' \
      'fi' \
      > /etc/claude-pod-init.sh \
 && chmod 0644 /etc/claude-pod-init.sh \
 && printf '\n%s\n' '[ -r /etc/claude-pod-init.sh ] && . /etc/claude-pod-init.sh' >> /etc/zsh/zshenv \
 && printf '\n%s\n' '[ -r /etc/claude-pod-init.sh ] && . /etc/claude-pod-init.sh' >> /etc/bash.bashrc

RUN groupadd -g 1000 claude \
 && useradd -m -u 1000 -g 1000 -s /bin/zsh claude

ENV GOROOT=/usr/local/go \
    GOPATH=/home/claude/.go \
    PATH=/home/claude/.local/bin:/usr/local/go/bin:/home/claude/.go/bin:/usr/local/bin:/usr/bin:/bin \
    HOME=/home/claude \
    CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1 \
    CLAUDE_CODE_NO_FLICKER=1

WORKDIR /home/claude

LABEL org.opencontainers.image.title="claude-pod" \
      org.opencontainers.image.description="Claude Code native binary on Debian with developer tooling, Go ${GO_VERSION}, uv ${UV_VERSION}, and bun ${BUN_VERSION}" \
      org.opencontainers.image.source="https://github.com/jacaudi/claude-pod" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.version="${CLAUDE_CODE_VERSION}"

USER claude

CMD ["claude-tmux"]
