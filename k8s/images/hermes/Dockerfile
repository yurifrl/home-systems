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

# renovate: datasource=docker depName=python versioning=docker
ARG PYTHON_VERSION=3.14-slim-trixie
FROM python:${PYTHON_VERSION}

# renovate: datasource=pypi depName=hermes-agent versioning=pep440
ARG HERMES_VERSION=0.15.2

# renovate: datasource=github-releases depName=kubernetes/kubectl
ARG KUBECTL_VERSION=1.33.0
ARG TARGETARCH

ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
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
        ca-certificates curl git ripgrep ffmpeg wget gnupg \
        gcc python3-dev libffi-dev \
    && mkdir -p -m 755 /etc/apt/keyrings \
    && wget -nv -O /etc/apt/keyrings/githubcli-archive-keyring.gpg \
        https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    && chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends gh nodejs \
    && pip install "hermes-agent[all,messaging]==${HERMES_VERSION}" \
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

# State (config.yaml, .env, memory, skills) lives here and is backed by a
# persistent volume in Kubernetes.
RUN mkdir -p /opt/data

# kubectl for the agent's read-only + pod-delete RBAC (multi-arch).
RUN curl -fsSLo /usr/local/bin/kubectl \
        "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH:-amd64}/kubectl" \
    && chmod 0755 /usr/local/bin/kubectl

# op-files-sync: downloads all file attachments of a 1Password item via the
# Connect REST API (dependency-free, dynamic — no filenames configured).
COPY op-files-sync /usr/local/bin/op-files-sync
RUN chmod 0755 /usr/local/bin/op-files-sync

# Non-root runtime user matching the chart's runAsUser/fsGroup 1000.
RUN useradd -u 1000 -m -d /home/hermes -s /bin/bash hermes

WORKDIR /opt/data
VOLUME ["/opt/data"]
USER 1000

ENTRYPOINT ["hermes"]
CMD ["gateway", "run"]
