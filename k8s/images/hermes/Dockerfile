# syntax=docker/dockerfile:1
#
# Hermes Agent — container image for home-systems.
#
# Nous Research publishes NO official container image, but they DO publish an
# official wheel per release (hermes-agent on PyPI). We install that pinned
# wheel rather than reproducing their complex multi-stage s6-overlay Dockerfile,
# which would drift from upstream. Bump HERMES_VERSION to upgrade.
#
# Runs as root (no USER switch) on purpose: the upstream image runs s6 as PID 1
# (root) and chowns the data volume itself. We keep root so the Longhorn PVC
# mounted at /opt/data is writable without needing a pod-level fsGroup (the
# shared `support` chart does not expose securityContext).
FROM python:3.13-slim-trixie

ARG HERMES_VERSION=0.15.2

ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    HERMES_HOME=/opt/data

# Runtime deps used by the agent's built-in tools (ripgrep, media, git).
# Build deps (gcc/dev headers) are installed only to compile native wheels,
# then purged in the same layer to keep the image lean.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl git ripgrep ffmpeg \
        gcc python3-dev libffi-dev \
    && pip install "hermes-agent[all,messaging]==${HERMES_VERSION}" \
    && apt-get purge -y gcc python3-dev libffi-dev \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*

# State (config.yaml, .env, memory, skills) lives here and is backed by a
# persistent volume in Kubernetes.
RUN mkdir -p /opt/data
WORKDIR /opt/data
VOLUME ["/opt/data"]

ENTRYPOINT ["hermes"]
CMD ["gateway", "run"]
