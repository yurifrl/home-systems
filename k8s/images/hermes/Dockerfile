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

# 3.14: hermes-agent requires Python >=3.11,<3.15, but since 0.20+ upstream
# gates its runtime dependency set on python_version >= '3.14' (47 lines in
# pyproject.toml — openai, pydantic, fastapi, ruamel.yaml, ...) — on 3.13 the
# package installs without its dependencies and crashes at import. Keep in
# lockstep with upstream's matrix; bump deliberately, not via Renovate's next
# floating tag.
# renovate: datasource=docker depName=python versioning=docker
ARG PYTHON_VERSION=3.14-slim-trixie
FROM python:${PYTHON_VERSION}

# renovate: datasource=github-releases depName=NousResearch/hermes-agent versioning=semver
ARG HERMES_VERSION=0.23.1

# renovate: datasource=github-releases depName=kubernetes/kubectl versioning=semver
ARG KUBECTL_VERSION=1.33.0

# renovate: datasource=github-releases depName=astral-sh/uv versioning=semver
ARG UV_VERSION=0.13.7

ARG TARGETARCH

ENV PYTHONUNBUFFERED=1 \
    UV_NO_CACHE=1 \
    HERMES_HOME=/opt/data \
    HERMES_RUNTIME_DIR=/opt/hermes/tools \
    HERMES_PYTHON=/usr/local/bin/python3 \
    HERMES_WEB_DIST=/opt/hermes/web_dist \
    HERMES_TUI_DIR=/usr/local/lib/hermes-tui

# Runtime deps used by the agent's built-in tools (ripgrep, media, git, gh).
# The GitHub CLI (gh) gives the bundled github-* skills their richest auth path
# and is installed from GitHub's official multi-arch apt repo (amd64 + arm64).
# Node.js is NOT installed from apt: since 0.23 hermes resolves node/npm only
# through its own package manager (pm) and never falls back to PATH, so node,
# npm and pnpm are staged by pm into HERMES_RUNTIME_DIR in the next layer.
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
    && apt-get update \
    && apt-get install -y --no-install-recommends gh \
    && curl -LsSf https://astral.sh/uv/${UV_VERSION}/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh \
    # Upstream setup.py blocks bdist_wheel for non-managed packagers; HERMES_NIX_BUILD=1
    # is their sanctioned escape hatch for owned derivations (gates only the guard —
    # the wheel content is identical).
    && printf '%s\n' \
        'pynacl>=1.6,<1.7' \
        'cryptography==50.0.2' \
        'Pillow==13.0.0' \
        "spacy-curated-transformers; sys_platform == 'never'" \
        > /tmp/hermes-uv-overrides.txt \
    # Overrides mirror upstream [tool.uv] override-dependencies (pyproject.toml):
    # browser-harness pins pillow==12.3.0, discord.py pins pynacl<1.6, and
    # alibabacloud-tea-openapi caps cryptography<49 — upstream force-resolves
    # these exact versions and we must too, or [all] resolution fails.
    && HERMES_NIX_BUILD=1 uv pip install --system \
        --overrides /tmp/hermes-uv-overrides.txt \
        "hermes-agent[all,messaging] @ https://github.com/NousResearch/hermes-agent/archive/refs/tags/v${HERMES_VERSION}.tar.gz" \
    && rm -f /tmp/hermes-uv-overrides.txt \
    && apt-get purge -y gcc python3-dev libffi-dev \
    && apt-get autoremove -y \
    && rm -rf /var/lib/apt/lists/*

# Source products from the v${HERMES_VERSION} release tree (one extraction),
# mirroring the pm/frontend steps of upstream's own Dockerfile:
#   1. pm tools — since 0.23 hermes resolves node/npm only through its package
#      manager (find_node_executable never falls back to PATH). uv/node/npm/pnpm
#      are staged into HERMES_RUNTIME_DIR=/opt/hermes/tools, outside the
#      /opt/data volume, and node/npm/pnpm are symlinked onto PATH.
#   2. PM runtime — PM's own isolated venv. A docker-distribution install
#      stamp MUST name it (pmRuntime) with a pm-runtime.json marker, or every
#      pm call fails "packaged PM runtime is missing" (this took WhatsApp down).
#      Upstream's seal_pm_runtime() requires a payload-owned interpreter, so
#      the marker is written directly against the base image's python.
#   3. install-stamp.json — `hermes --version` reads the stamp; we resolve the
#      tag's commit with ls-remote (the ^{} peel — tags are annotated).
#   4. Frontends — JS workspaces install with pm's pinned pnpm (upstream
#      dropped package-lock.json at 0.23.1). The dashboard SPA is built into
#      HERMES_WEB_DIST (without it: "Frontend not built"); the TUI bundle into
#      HERMES_TUI_DIR (the launcher's ui-tui/ workspace guard is repo-only).
#   5. WhatsApp bridge — scripts/whatsapp-bridge ships in the source tree only;
#      node_modules is baked in. At runtime the read-only tree is mirrored to
#      HERMES_HOME/scripts/whatsapp-bridge; pairing state persists on /opt/data.
# npm cache is a BuildKit cache mount, so no npm cache lands in the image.
RUN --mount=type=cache,target=/root/.npm set -eu \
    && SP="$(python -c 'import site; print(site.getsitepackages()[0])')" \
    && curl -fsSL --retry 3 --retry-delay 2 "https://github.com/NousResearch/hermes-agent/archive/refs/tags/v${HERMES_VERSION}.tar.gz" -o /tmp/hermes-src.tar.gz \
    && tar -xzf /tmp/hermes-src.tar.gz -C /tmp \
    && SRC="/tmp/hermes-agent-${HERMES_VERSION}" \
    && python -c 'from pm import ensure; [ensure(name, explicit=True) for name in ("uv", "npm", "pnpm")]' \
    && python -c 'from pathlib import Path; from pm import installed_package; [Path("/usr/local/bin", c).symlink_to(installed_package(p).binary) for c, p in (("node", "node"), ("npm", "npm"), ("pnpm", "pnpm"))]' \
    && python -c 'import shutil; from pathlib import Path; from pm import env_for; Path("/usr/local/bin/npx").symlink_to(shutil.which("npx", path=env_for("npm", base_env={})["PATH"]))' \
    && node --version && npm --version && pnpm --version \
    && python -c "from pathlib import Path; from pm import stage_manager_runtime; stage_manager_runtime(python=Path('/usr/local/bin/python3').resolve(), destination=Path('/opt/hermes/pm-runtime'), project=Path('$SRC/pm'))" \
    && python -c 'import json, os, sys; from pathlib import Path; r = Path("/opt/hermes/pm-runtime"); s = list(r.glob("lib/python*/site-packages")); assert len(s) == 1, s; (r / "pm-runtime.json").write_text(json.dumps({"python": os.path.relpath(Path(sys.executable).resolve(), r), "sitePackages": s[0].relative_to(r).as_posix()}))' \
    && COMMIT="$(git ls-remote https://github.com/NousResearch/hermes-agent.git "refs/tags/v${HERMES_VERSION}^{}" | awk 'NR==1{print $1; exit}')" \
    && if [ -z "$COMMIT" ]; then \
           COMMIT="$(git ls-remote https://github.com/NousResearch/hermes-agent.git "refs/tags/v${HERMES_VERSION}" | awk 'NR==1{print $1; exit}')"; \
       fi \
    && test -n "$COMMIT" \
    && python "$SRC/scripts/write_install_stamp.py" \
        --output "$SP/install-stamp.json" \
        --base-version "${HERMES_VERSION}" --commit "${COMMIT}" \
        --source docker --distribution docker --update-mechanism external \
    && python -c "import json; from pathlib import Path; p = Path('$SP/install-stamp.json'); d = json.loads(p.read_text()); d['pmRuntime'] = '/opt/hermes/pm-runtime'; p.write_text(json.dumps(d) + '\\n')" \
    && python -c 'from pm.runtime import _resident_runtime; print("pm runtime:", _resident_runtime())' \
    && node "$SRC/scripts/build/node-deps.mjs" --source "$SRC" --workspace ui-tui --workspace web \
    && python "$SRC/scripts/generate_icons.py" --source "$SRC" --out /tmp/hermes-icons \
    && node "$SRC/scripts/build/tui.mjs" --source "$SRC" --out "$HERMES_TUI_DIR" \
    && node "$SRC/scripts/build/web.mjs" --source "$SRC" --icons /tmp/hermes-icons --out "$HERMES_WEB_DIST" \
    && test -f "$HERMES_WEB_DIST/index.html" \
    && mkdir -p "$SP/scripts" \
    && cp -r "$SRC/scripts/whatsapp-bridge" "$SP/scripts/whatsapp-bridge" \
    && cd "$SP/scripts/whatsapp-bridge" \
    && npm ci --no-fund --no-audit --progress=false \
    && rm -rf /tmp/hermes-src.tar.gz "$SRC" /tmp/hermes-icons \
    && chmod -R a+rX /opt/hermes "$HERMES_TUI_DIR"

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
