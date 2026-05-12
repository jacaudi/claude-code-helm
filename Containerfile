# syntax=docker/dockerfile:1.7

# renovate: datasource=docker depName=archlinux
ARG ARCH_VERSION=base-20260510.0.525573

# renovate: datasource=docker depName=alpine
ARG ALPINE_VERSION=3.21

# renovate: datasource=npm depName=@anthropic-ai/claude-code
ARG CLAUDE_CODE_VERSION=2.1.37

# renovate: datasource=docker depName=golang
ARG GO_VERSION=1.26.3

# renovate: datasource=docker depName=ghcr.io/astral-sh/uv
ARG UV_VERSION=0.11.13

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

############################################
# Stage 3: final runtime image
############################################
FROM public.ecr.aws/docker/library/archlinux:${ARCH_VERSION}

ARG CLAUDE_CODE_VERSION
ARG GO_VERSION
ARG UV_VERSION
ARG BUILD_DATE
ARG VCS_REF

ENV LANG=C.UTF-8 \
    LC_ALL=C.UTF-8

RUN pacman -Syu --noconfirm --needed \
      base-devel \
      bash \
      ca-certificates \
      curl \
      fzf \
      git \
      github-cli \
      gnupg \
      jq \
      less \
      openssh \
      procps-ng \
      ripgrep \
      tmux \
      unzip \
      which \
      zsh \
    && pacman -Scc --noconfirm \
    && rm -rf /var/cache/pacman/pkg/*

# Go toolchain — copied from the official golang image.
COPY --from=go-source /usr/local/go /usr/local/go

# uv / uvx — copied from Astral's distroless image.
COPY --from=uv-source /uv /uvx /usr/local/bin/

# Claude Code native binary — verified above.
COPY --from=claude-fetcher /out/claude /usr/local/bin/claude

# System-wide tmux config — recommended settings for Claude Code per
# https://code.claude.com/docs/en/terminal-config.md#configure-tmux
# Lives at /etc to survive the PVC mount over /home/claude.
RUN printf '%s\n' \
      'set -g allow-passthrough on' \
      'set -s extended-keys on' \
      'set -as terminal-features "xterm*:extkeys"' \
      > /etc/tmux.conf

# Helper entrypoint: run (or reattach to) Claude Code in a persistent tmux
# session. Detach with Ctrl-b d; reattach by re-running this command.
# Extra args pass through to claude.
RUN printf '%s\n' \
      '#!/bin/bash' \
      'exec tmux new-session -A -s claude claude "$@"' \
      > /usr/local/bin/claude-tmux \
 && chmod 0755 /usr/local/bin/claude-tmux

ENV GOROOT=/usr/local/go \
    GOPATH=/home/claude/go \
    PATH=/usr/local/go/bin:/home/claude/go/bin:/usr/local/bin:/usr/bin:/bin

RUN groupadd -g 1000 claude \
 && useradd -m -u 1000 -g 1000 -s /bin/zsh claude

ENV HOME=/home/claude

WORKDIR /home/claude

LABEL org.opencontainers.image.title="claude-pod" \
      org.opencontainers.image.description="Claude Code native binary on Arch Linux with developer tooling, Go ${GO_VERSION}, and uv ${UV_VERSION}" \
      org.opencontainers.image.source="https://github.com/jacaudi/claude-code-helm" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.version="${CLAUDE_CODE_VERSION}"

USER claude

CMD ["claude-tmux"]
