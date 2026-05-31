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
# Runs as root (no USER switch) on purpose: the upstream image runs s6 as PID 1
# (root) and chowns the data volume itself. We keep root so the Longhorn PVC
# mounted at /opt/data is writable without needing a pod-level fsGroup (the
# shared `support` chart does not expose securityContext).

# renovate: datasource=docker depName=python versioning=docker
ARG PYTHON_VERSION=3.14-slim-trixie
FROM python:${PYTHON_VERSION}

# renovate: datasource=pypi depName=hermes-agent versioning=pep440
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
