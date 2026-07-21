# syntax=docker/dockerfile:1
#
# Hermes Agent — container image for home-systems.
#
# Nous Research publishes NO official container image, but they DO publish an
# official wheel per release (hermes-agent on PyPI). We install that pinned
# wheel rather than reproducing their complex multi-stage s6-overlay Dockerfile,
# which would drift from upstream.
#
# Both versions below are auto-bumped by Renovate (see renovate.json custom
# manager) and are the single source of truth for the image tag, which CI
# derives as "<hermes-version>-py<python-version>".
#
# Runs as a non-root user (uid 1000) with a read-only root filesystem under the
# hermes Helm chart's hardened securityContext. All runtime writes go to mounted
# volumes (/opt/data PVC, /tmp emptyDir, HOME=/opt/data/home). kubectl is bundled
# so the agent can use its (read-only + pod-delete) RBAC.

# Pinned to 3.13: hermes-agent (below) requires Python >=3.11,<3.14, so 3.14
# breaks `pip install hermes-agent`. Do not bump past 3.13 until upstream
# hermes-agent supports 3.14.
# renovate: datasource=docker depName=python versioning=docker
ARG PYTHON_VERSION=3.13-slim-trixie
FROM python:${PYTHON_VERSION}

# renovate: datasource=pypi depName=hermes-agent versioning=pep440
ARG HERMES_VERSION=0.18.2

# renovate: datasource=github-releases depName=kubernetes/kubectl versioning=semver
ARG KUBECTL_VERSION=1.33.0

# renovate: datasource=github-releases depName=astral-sh/uv versioning=semver
ARG UV_VERSION=0.11.30

# renovate: datasource=node-version depName=node versioning=node
ARG NODE_VERSION=24

ARG TARGETARCH

ENV PYTHONUNBUFFERED=1 \
    UV_NO_CACHE=1 \
    HERMES_HOME=/opt/data

# Runtime deps used by the agent's built-in tools (ripgrep, media, git, gh).
# The GitHub CLI (gh) gives the bundled github-* skills their richest auth path
# and is installed from GitHub's official multi-arch apt repo (amd64 + arm64).
# Node.js is required by the WhatsApp (Baileys) bridge and is installed from
# NodeSource's official multi-arch apt repo (amd64 + arm64). It is kept in the
# final image because the gateway launches the bridge with `node` at runtime.
# Build deps (gcc/dev headers) are installed only to compile native wheels,
# then purged in the same layer to keep the image lean.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl git git-lfs ripgrep ffmpeg wget gnupg \
        gcc python3-dev libffi-dev \
        fish less vim htop jq tree tmux procps iproute2 fzf bat fd-find \
    && ln -sf /usr/bin/batcat /usr/local/bin/bat \
    && ln -sf /usr/bin/fdfind /usr/local/bin/fd \
    && mkdir -p -m 755 /etc/apt/keyrings \
    && wget -nv -O /etc/apt/keyrings/githubcli-archive-keyring.gpg \
        https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    && chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && curl -fsSL https://deb.nodesource.com/setup_${NODE_VERSION}.x | bash - \
    && apt-get install -y --no-install-recommends gh nodejs \
    && curl -LsSf https://astral.sh/uv/${UV_VERSION}/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh \
    && uv pip install --system "hermes-agent[all,messaging]==${HERMES_VERSION}" \
    && apt-get purge -y gcc python3-dev libffi-dev \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*

# WhatsApp bridge (Baileys, Node.js). The bridge lives ONLY in the upstream git
# repo under scripts/whatsapp-bridge/ — it is shipped in neither the PyPI wheel
# nor the sdist, so a wheel-only install (as above) lacks it and `hermes whatsapp`
# fails with "Bridge script not found". The official s6 image bakes it in from
# source; we do the same. `hermes --version` prints the calendar git tag in
# parentheses (e.g. "v0.15.2 (2026.5.29.2)" -> tag v2026.5.29.2), so we derive
# the ref from the installed wheel and stay locked to it across Renovate bumps.
# node_modules is baked in so the gateway can launch the bridge immediately and
# `hermes whatsapp` only needs to pair (QR scan); the WhatsApp session itself
# persists on the /opt/data volume.
RUN set -eu \
    && SP="$(python -c 'import site; print(site.getsitepackages()[0])')" \
    && CAL="$(hermes --version | grep -oE '\([0-9.]+\)' | tr -d '()')" \
    && test -n "$CAL" \
    && curl -fsSL "https://github.com/NousResearch/hermes-agent/archive/refs/tags/v${CAL}.tar.gz" -o /tmp/hermes-src.tar.gz \
    && mkdir -p "$SP/scripts" \
    && tar -xzf /tmp/hermes-src.tar.gz -C /tmp "hermes-agent-${CAL}/scripts/whatsapp-bridge" \
    && cp -r "/tmp/hermes-agent-${CAL}/scripts/whatsapp-bridge" "$SP/scripts/whatsapp-bridge" \
    && cd "$SP/scripts/whatsapp-bridge" \
    && (npm ci --no-fund --no-audit --progress=false \
        || npm install --no-fund --no-audit --progress=false) \
    && npm cache clean --force \
    && rm -rf /tmp/hermes-src.tar.gz "/tmp/hermes-agent-${CAL}"

# TUI bundle (dashboard Chat tab). The wheel ships a prebuilt, self-contained
# TUI at hermes_cli/tui_dist/entry.js, but hermes_cli/main.py:_make_tui_argv()
# guards on the ui-tui/ *source* workspace — which lives only in the git repo,
# not the wheel — and aborts ("TUI workspace is missing") BEFORE reaching its
# bundled-wheel fallback. Setting HERMES_TUI_DIR to a dir whose dist/entry.js
# resolves to the shipped bundle makes the launcher run the prebuilt TUI and
# skip the source-workspace guard entirely, so `hermes --tui` (the Chat tab)
# works without shipping/building the ui-tui Node workspace.
RUN set -eu \
    && SP="$(python -c 'import site; print(site.getsitepackages()[0])')" \
    && test -f "$SP/hermes_cli/tui_dist/entry.js" \
    && mkdir -p /usr/local/lib/hermes-tui \
    && ln -s "$SP/hermes_cli/tui_dist" /usr/local/lib/hermes-tui/dist
ENV HERMES_TUI_DIR=/usr/local/lib/hermes-tui

# State (config.yaml, .env, memory, skills) lives here and is backed by a
# persistent volume in Kubernetes.
RUN mkdir -p /opt/data

# kubectl for the agent's read-only + pod-delete RBAC (multi-arch).
RUN curl -fsSLo /usr/local/bin/kubectl \
        "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH:-amd64}/kubectl" \
    && chmod 0755 /usr/local/bin/kubectl

# beads (bd) — AI-native issue tracker used by pi agents for task coordination.
RUN curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash

# hctl — in-container helper CLI used by the Helm chart's init/sidecar
# containers (auth login, op files sync, repos clone, repos sync). One Python
# entrypoint, stdlib-only, replaces the legacy op-files-sync script and the
# inline shell blobs that used to live in k8s/applications/hermes.yaml.
COPY hctl /usr/local/bin/hctl
RUN chmod 0755 /usr/local/bin/hctl

# Non-root runtime user matching the chart's runAsUser/fsGroup 1000.
# Default shell is fish for a comfortable interactive `kubectl exec` experience
# (the app itself runs via the `hermes` entrypoint, not a login shell).
RUN useradd -u 1000 -m -d /home/hermes -s /usr/bin/fish hermes

WORKDIR /opt/data
VOLUME ["/opt/data"]
USER 1000

ENTRYPOINT ["hermes"]
CMD ["gateway", "run"]
