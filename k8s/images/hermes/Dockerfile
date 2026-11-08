# syntax=docker/dockerfile:1
#
# Hermes Agent — container image for home-systems.
#
# Nous Research publishes NO official container image, and stopped publishing
# hermes-agent wheels to PyPI at 0.19.0 (uploaded 2026-07-20) — releases since
# then are GitHub tags only. We build from the release tag's source tarball
# rather than reproducing their complex multi-stage s6-overlay Dockerfile,
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

# Pinned to 3.13: hermes-agent requires Python >=3.11,<3.15. Upstream supports
# 3.14 but 3.13 is the version this image's dependency set is proven on — bump
# deliberately, not via Renovate's next floating tag.
# renovate: datasource=docker depName=python versioning=docker
ARG PYTHON_VERSION=3.13-slim-trixie
FROM python:${PYTHON_VERSION}

# renovate: datasource=github-releases depName=NousResearch/hermes-agent versioning=semver
ARG HERMES_VERSION=0.23.0

# renovate: datasource=github-releases depName=kubernetes/kubectl versioning=semver
ARG KUBECTL_VERSION=1.33.0

# renovate: datasource=github-releases depName=astral-sh/uv versioning=semver
ARG UV_VERSION=0.13.7

# renovate: datasource=node-version depName=node versioning=node
ARG NODE_VERSION=26

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
    # Upstream setup.py blocks bdist_wheel for non-managed packagers; HERMES_NIX_BUILD=1
    # is their sanctioned escape hatch for owned derivations (gates only the guard —
    # the wheel content is identical).
    && HERMES_NIX_BUILD=1 uv pip install --system "hermes-agent[all,messaging] @ https://github.com/NousResearch/hermes-agent/archive/refs/tags/v${HERMES_VERSION}.tar.gz" \
    && apt-get purge -y gcc python3-dev libffi-dev \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*

# Source products from the v${HERMES_VERSION} release tree (one extraction).
# A pip install of the tag tarball has no .git and no stamp, so:
#   1. install-stamp.json — `hermes --version` reads the stamp; without a real
#      commit it prints "unknown" (the reader rejects all-zero fallback
#      stamps). We resolve the tag's commit with ls-remote — the ^{} peel is
#      authoritative (v0.23.0 is an annotated tag).
#   2. WhatsApp bridge — scripts/whatsapp-bridge ships in the source tree only
#      (not in any wheel); without it `hermes whatsapp` fails with "Bridge
#      script not found". node_modules is baked in so the gateway can launch
#      it immediately; pairing (QR) state persists on /opt/data.
#   3. TUI bundle — _make_tui_argv() guards on the ui-tui/ source workspace
#      (repo-only) and aborts before reaching its bundled-wheel fallback, so
#      `hermes --tui` breaks. We build the self-contained esbuild bundle into
#      HERMES_TUI_DIR, which the launcher prefers and needs no workspace.
# npm cache is a BuildKit cache mount, so no npm cache lands in the image.
RUN --mount=type=cache,target=/root/.npm set -eu \
    && SP="$(python -c 'import site; print(site.getsitepackages()[0])')" \
    && curl -fsSL "https://github.com/NousResearch/hermes-agent/archive/refs/tags/v${HERMES_VERSION}.tar.gz" -o /tmp/hermes-src.tar.gz \
    && tar -xzf /tmp/hermes-src.tar.gz -C /tmp \
    && SRC="/tmp/hermes-agent-${HERMES_VERSION}" \
    && COMMIT="$(git ls-remote https://github.com/NousResearch/hermes-agent.git "refs/tags/v${HERMES_VERSION}^{}" | awk 'NR==1{print $1; exit}')" \
    && if [ -z "$COMMIT" ]; then \
           COMMIT="$(git ls-remote https://github.com/NousResearch/hermes-agent.git "refs/tags/v${HERMES_VERSION}" | awk 'NR==1{print $1; exit}')"; \
       fi \
    && test -n "$COMMIT" \
    && python "$SRC/scripts/write_install_stamp.py" \
        --output "$SP/install-stamp.json" \
        --base-version "${HERMES_VERSION}" --commit "${COMMIT}" \
        --source docker --distribution docker --update-mechanism external \
    # --ignore-scripts: the tree spans all workspaces (incl. apps/desktop's native
    # node-pty); the TUI bundle needs only esbuild's optionalDeps binary, and the
    # build image has no C toolchain by design. The bridge install below keeps scripts.
    && npm ci --ignore-scripts --no-fund --no-audit --progress=false --prefix "$SRC" \
    && node "$SRC/scripts/build/tui.mjs" --source "$SRC" --out /usr/local/lib/hermes-tui \
    && mkdir -p "$SP/scripts" \
    && cp -r "$SRC/scripts/whatsapp-bridge" "$SP/scripts/whatsapp-bridge" \
    && cd "$SP/scripts/whatsapp-bridge" \
    && npm ci --no-fund --no-audit --progress=false \
    && rm -rf /tmp/hermes-src.tar.gz "$SRC"
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
