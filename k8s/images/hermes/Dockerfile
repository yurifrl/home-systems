# syntax=docker/dockerfile:1
#
# Hermes Agent — container image for home-systems.
#
# Built from the v${HERMES_VERSION} release tarball using upstream's own 0.23+
# layout (NousResearch/hermes-agent Dockerfile): the source tree lives at
# /opt/hermes, its package manager (pm) stages Python/node/npm/ffmpeg/ripgrep
# into /opt/hermes/tools, the app runs from a sealed pm-built venv at
# /opt/hermes/.venv, and docker/build_agent.py assembles the payload. Hermes
# 0.23 assumes this layout: plugin and lazy-extra dependencies are admitted by
# pm into dependency generations on the /opt/data volume, which only works
# when the app runs from pm's own environment (a pip/--system install broke
# WhatsApp, plugin enabling, and lazy provider installs).
#
# What we deliberately do NOT take from upstream:
#   - s6-overlay / entrypoint-dispatch: Kubernetes supervises; the chart runs
#     `hermes gateway run` / `hermes dashboard` directly. The boot work upstream's
#     stage2-hook.sh does (config migration, dependency refresh) runs in the
#     chart's hermes-prepare init container instead.
#   - the [matrix] extra (needs libolm) and the Electron webapp / photon sidecar.
#
# Our additions: kubectl, gh, beads, hctl, fish + shell tools, uid 1000 user.
#
# Runs as a non-root user (uid 1000) with a read-only root filesystem under the
# hermes Helm chart's hardened securityContext. All runtime writes go to mounted
# volumes (/opt/data PVC, /tmp emptyDir, HOME=/opt/data/home).

# renovate: datasource=docker depName=debian versioning=docker
ARG DEBIAN_VERSION=trixie-slim
FROM debian:${DEBIAN_VERSION}

# renovate: datasource=github-releases depName=NousResearch/hermes-agent versioning=semver
ARG HERMES_VERSION=0.23.1

# renovate: datasource=github-releases depName=kubernetes/kubectl versioning=semver
ARG KUBECTL_VERSION=1.33.0

ARG TARGETARCH

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    HERMES_HOME=/opt/data \
    HERMES_RUNTIME_DIR=/opt/hermes/tools \
    PLAYWRIGHT_BROWSERS_PATH=/opt/hermes/tools

# System packages. python3 is only the bootstrap interpreter for pm — the app
# runs on pm's pinned Python. The C toolchain stays (as upstream): plugin and
# lazy-extra dependency admission may compile native wheels at runtime.
RUN apt-get -o Acquire::Retries=3 update \
    && apt-get -o Acquire::Retries=3 install -y --no-install-recommends \
        ca-certificates curl wget gnupg git git-lfs openssh-client xz-utils \
        python3 python3-venv python-is-python3 gcc g++ make python3-dev libffi-dev libatomic1 \
        procps tmux fish less vim htop jq tree iproute2 fzf bat fd-find \
    && ln -sf /usr/bin/batcat /usr/local/bin/bat \
    && ln -sf /usr/bin/fdfind /usr/local/bin/fd \
    && mkdir -p -m 755 /etc/apt/keyrings \
    && wget -nv -O /etc/apt/keyrings/githubcli-archive-keyring.gpg \
        https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    && chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
        > /etc/apt/sources.list.d/github-cli.list \
    && apt-get -o Acquire::Retries=3 update \
    && apt-get install -y --no-install-recommends gh \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /opt/hermes

# Source tree + pm tools + PM runtime (upstream runtime_base stage).
# Tools land in /opt/hermes/tools (outside the /opt/data volume) and are put on
# PATH; hermes resolves node/npm only through pm, never PATH, so these are the
# copies it actually uses. The PM runtime is pm's own isolated venv; the
# install stamp's pmRuntime names it (without it: "packaged PM runtime is
# missing").
RUN set -eu \
    && curl -fsSL --retry 3 --retry-delay 2 \
        "https://github.com/NousResearch/hermes-agent/archive/refs/tags/v${HERMES_VERSION}.tar.gz" \
        | tar -xz --strip-components=1 -C /opt/hermes \
    && rm -rf website tests evals \
    && python3 -c 'from pm import ensure; [ensure(name, explicit=True) for name in ("uv", "python", "npm", "pnpm", "ffmpeg", "ripgrep")]' \
    && python3 -c 'from pathlib import Path; from pm import installed_package; [Path("/usr/local/bin", c).symlink_to(installed_package(p).binary) for c, p in (("python3", "python"), ("node", "node"), ("npm", "npm"), ("pnpm", "pnpm"), ("ffmpeg", "ffmpeg"), ("rg", "ripgrep"))]; Path("/usr/local/bin/ffprobe").symlink_to(installed_package("ffmpeg").binary.with_name("ffprobe"))' \
    && ln -s python3 /usr/local/bin/python \
    && python3 -c 'import shutil; from pathlib import Path; from pm import env_for; Path("/usr/local/bin/npx").symlink_to(shutil.which("npx", path=env_for("npm", base_env={})["PATH"]))' \
    && python3 --version && node --version && npm --version && pnpm --version && ffmpeg -version >/dev/null && rg --version >/dev/null \
    && python3 -c 'from pathlib import Path; from pm import stage_manager_runtime; from scripts.bundles.payload import seal_pm_runtime; root = Path("/opt/hermes"); python = Path("/usr/local/bin/python3").resolve(); stage_manager_runtime(python=python, destination=root / "pm-runtime", project=root / "pm"); seal_pm_runtime(root, python)'

# Sealed application venv (upstream python_deps stage). Extras match upstream's
# production image minus [matrix]: provider SDKs (anthropic, bedrock, azure)
# must be baked — lazy installs can't write the sealed venv.
RUN python3 -m pm.build_env --source /opt/hermes --python /usr/local/bin/python3 \
        --cache /tmp/uv-cache \
        --out /opt/hermes/.venv --no-install-project --sealed --record-selection \
        --extra all --extra messaging --extra otlp --extra anthropic --extra bedrock \
        --extra azure-identity --extra google-chat --extra fal --extra documents \
    && rm -rf /tmp/uv-cache

# Frontends (upstream icons + frontend_build stages): pnpm workspace install,
# TUI bundle, dashboard SPA. Workspace node_modules never ship; only the
# TypeScript copy upstream keeps for standalone linting.
# WhatsApp bridge: scripts/whatsapp-bridge ships in the source tree only;
# node_modules is baked. At runtime the read-only tree is mirrored to
# HERMES_HOME/scripts/whatsapp-bridge; pairing state persists on /opt/data.
ENV HERMES_PYTHON=/usr/local/bin/python3 CC=gcc CXX=g++
RUN --mount=type=cache,target=/root/.npm set -eu \
    && node scripts/build/node-deps.mjs --source /opt/hermes --workspace ui-tui --workspace web \
    && /opt/hermes/.venv/bin/python -I scripts/generate_icons.py --source /opt/hermes --out /tmp/hermes-icons \
    && node scripts/build/tui.mjs --source /opt/hermes --out /tmp/products/tui \
    && node scripts/build/web.mjs --source /opt/hermes --icons /tmp/hermes-icons --out /tmp/products/web \
    && cp -RL node_modules/typescript /tmp/products/typescript \
    && find /opt/hermes -path /opt/hermes/tools -prune -o -path /opt/hermes/.venv -prune -o -name node_modules -type d -prune -exec rm -rf {} + \
    && mkdir -p node_modules/.bin \
    && mv /tmp/products/typescript node_modules/typescript \
    && ln -s ../typescript/bin/tsc node_modules/.bin/tsc \
    && rm -rf ui-tui && mv /tmp/products/tui ui-tui \
    && rm -rf hermes_cli/web_dist && mv /tmp/products/web hermes_cli/web_dist \
    && test -f hermes_cli/web_dist/index.html \
    && (cd scripts/whatsapp-bridge && npm ci --no-fund --no-audit --progress=false) \
    && rm -rf /tmp/products /tmp/hermes-icons /root/.local/share/pnpm /root/.cache

# Install stamp + payload assembly (upstream runtime stage). The stamp carries
# the tag's real commit (ls-remote ^{} peel: tags are annotated) so
# `hermes --version` is accurate, and pmRuntime names the staged PM runtime.
RUN set -eu \
    && COMMIT="$(git ls-remote https://github.com/NousResearch/hermes-agent.git "refs/tags/v${HERMES_VERSION}^{}" | awk 'NR==1{print $1; exit}')" \
    && if [ -z "$COMMIT" ]; then \
           COMMIT="$(git ls-remote https://github.com/NousResearch/hermes-agent.git "refs/tags/v${HERMES_VERSION}" | awk 'NR==1{print $1; exit}')"; \
       fi \
    && test -n "$COMMIT" \
    && python3 scripts/write_install_stamp.py --output /opt/hermes/install-stamp.json \
        --base-version "${HERMES_VERSION}" --commit "${COMMIT}" \
        --source docker --distribution docker --update-mechanism external \
    && /opt/hermes/.venv/bin/python -m docker.build_agent \
    && python3 -c 'import json; from pathlib import Path; path = Path("/opt/hermes/install-stamp.json"); stamp = json.loads(path.read_text()); stamp["pmRuntime"] = "/opt/hermes/pm-runtime"; path.write_text(json.dumps(stamp) + "\n")' \
    && printf 'docker\n' > /opt/hermes/.install_method \
    && chmod 0644 /opt/hermes/tools/facts.json \
    && rm -f /opt/hermes/.venv/.lock /opt/hermes/pm-runtime/.lock \
    && chmod -R a+rX,go-w /opt/hermes \
    && for cmd in /opt/hermes/.venv/bin/hermes*; do ln -sf "$cmd" /usr/local/bin/; done \
    && /opt/hermes/.venv/bin/python -c 'from pm.runtime import _resident_runtime; print("pm runtime:", _resident_runtime())' \
    && /opt/hermes/.venv/bin/python -c 'import anthropic, hermes_cli; print("anthropic", anthropic.__version__)'

ENV HERMES_PYTHON=/opt/hermes/.venv/bin/python \
    HERMES_WEB_DIST=/opt/hermes/hermes_cli/web_dist \
    HERMES_TUI_DIR=/opt/hermes/ui-tui \
    PATH="/opt/hermes/.venv/bin:${PATH}"

# State (config.yaml, .env, memory, skills, plugins, dependency generations)
# lives here and is backed by a persistent volume in Kubernetes.
RUN mkdir -p /opt/data

# kubectl for the agent's read-only + pod-delete RBAC (multi-arch).
RUN curl -fsSLo /usr/local/bin/kubectl \
        "https://dl.k8s.io/release/v${KUBECTL_VERSION}/bin/linux/${TARGETARCH:-amd64}/kubectl" \
    && chmod 0755 /usr/local/bin/kubectl

# beads (bd) — AI-native issue tracker used by pi agents for task coordination.
RUN curl -fsSL https://raw.githubusercontent.com/steveyegge/beads/main/scripts/install.sh | bash

# hctl — in-container helper CLI used by the Helm chart's init/sidecar
# containers (auth login, op files sync, repos clone, repos sync). One Python
# entrypoint, stdlib-only.
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
