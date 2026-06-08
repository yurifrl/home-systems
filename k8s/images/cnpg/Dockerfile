# syntax=docker/dockerfile:1
#
# Supabase-on-CloudNativePG PostgreSQL image for home-systems.
#
# CloudNativePG (CNPG) runs a PostgreSQL operand image as the database server.
# To run Supabase on a BYO-Postgres CNPG Cluster, the operand image must carry
# the extensions Supabase expects. The stock CNPG image
# (ghcr.io/cloudnative-pg/postgresql) does not ship them, so this image extends
# that base and bakes the buildable Supabase extensions in, staying
# CNPG-compatible: same Debian/PGDG layout, same postgres uid (26), unchanged
# entrypoint/initdb behaviour.
#
# Extension status (see README.md for the full table):
#   included (built from source here): wal2json, pgjwt, pgsodium, pg_net,
#                                       supabase_vault
#   TODO (not built — see block at the bottom): pg_graphql
#
# All version ARGs below are the single source of truth for the image tag,
# which CI derives from BASE_PG_VERSION (e.g. "16.10-rN"). Keep them near the
# top so .github/workflows/build-cnpg.yaml can grep them with grep -oP.

# ---- shared build args (declared before the first FROM so both stages see them)
ARG PG_MAJOR=16
# renovate: datasource=docker depName=ghcr.io/cloudnative-pg/postgresql
# Full base tag. Auto-detection in CNPG requires the tag to start with the PG
# major; this is the FROM tag, our own image tag is derived from it in CI.
ARG BASE_PG_VERSION=16.10-system-trixie
ARG BASE_IMAGE=ghcr.io/cloudnative-pg/postgresql:${BASE_PG_VERSION}

# Pinned extension versions (git tags). Bump deliberately — these are compiled
# against the base PG major above.
# renovate: datasource=github-releases depName=eulerto/wal2json
ARG WAL2JSON_VERSION=wal2json_2_6
# pgjwt has no tagged releases; pin to a known-good commit on master.
ARG PGJWT_REF=f3d82fd1f37b493abe5f989088f1c660f9e8b3c8
# renovate: datasource=github-releases depName=michelp/pgsodium
ARG PGSODIUM_VERSION=v3.1.9
# renovate: datasource=github-releases depName=supabase/pg_net
ARG PG_NET_VERSION=v0.14.0
# renovate: datasource=github-releases depName=supabase/vault
ARG VAULT_VERSION=v0.3.1

# =============================================================================
# Build stage — compile every extension against the EXACT base image so the
# pg_config paths (/usr/lib/postgresql/$PG_MAJOR/lib,
# /usr/share/postgresql/$PG_MAJOR/extension) and ABI match the final image.
# Runs as root (the base sets USER 26 at the end; we override here).
# =============================================================================
FROM ${BASE_IMAGE} AS builder

ARG PG_MAJOR
ARG WAL2JSON_VERSION
ARG PGJWT_REF
ARG PGSODIUM_VERSION
ARG PG_NET_VERSION
ARG VAULT_VERSION

USER root

# Toolchain + dev headers for the matching PG major and the C deps the
# extensions link against (libsodium for pgsodium, libcurl for pg_net).
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential git ca-certificates \
        postgresql-server-dev-${PG_MAJOR} \
        libsodium-dev libcurl4-openssl-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# wal2json — logical decoding output plugin (JSON). Plain pgxs build.
RUN git clone --depth 1 --branch "${WAL2JSON_VERSION}" \
        https://github.com/eulerto/wal2json.git wal2json \
    && cd wal2json \
    && make USE_PGXS=1 \
    && make USE_PGXS=1 install

# pgjwt — SQL-only JWT helpers (depends on pgcrypto, shipped in contrib).
RUN git clone https://github.com/michelp/pgjwt.git pgjwt \
    && cd pgjwt \
    && git checkout "${PGJWT_REF}" \
    && make install

# pgsodium — libsodium-backed crypto. Needs libsodium-dev at build time.
RUN git clone --depth 1 --branch "${PGSODIUM_VERSION}" \
        https://github.com/michelp/pgsodium.git pgsodium \
    && cd pgsodium \
    && make \
    && make install

# pg_net — async HTTP via libcurl + a background worker. C extension (pgxs).
RUN git clone --depth 1 --branch "${PG_NET_VERSION}" \
        https://github.com/supabase/pg_net.git pg_net \
    && cd pg_net \
    && make \
    && make install

# supabase_vault — secrets storage; pairs with pgsodium at runtime. pgxs build.
RUN git clone --depth 1 --branch "${VAULT_VERSION}" \
        https://github.com/supabase/vault.git vault \
    && cd vault \
    && make \
    && make install

# =============================================================================
# Final stage — start from the pristine CNPG base again and copy only the built
# artifacts. Install runtime shared libs (libsodium, libcurl) as root, then
# return to uid 26 so the image stays a drop-in CNPG operand.
# =============================================================================
FROM ${BASE_IMAGE}

ARG PG_MAJOR

USER root

# Runtime shared libraries the copied extensions dlopen at load time.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        libsodium23 libcurl4 \
    && rm -rf /var/lib/apt/lists/*

# Extension shared objects (.so) and their control/SQL files. These two dirs
# are where pgxs `make install` placed everything in the builder.
COPY --from=builder /usr/lib/postgresql/${PG_MAJOR}/lib/ \
                    /usr/lib/postgresql/${PG_MAJOR}/lib/
COPY --from=builder /usr/share/postgresql/${PG_MAJOR}/extension/ \
                    /usr/share/postgresql/${PG_MAJOR}/extension/

# -----------------------------------------------------------------------------
# TODO: pg_graphql (https://github.com/supabase/pg_graphql)
#
# Not built here. pg_graphql is a Rust extension built with pgrx, which requires
# the full Rust toolchain + a `cargo pgrx init` against this exact PG major.
# Under buildx QEMU emulation (arm64) that compile is very slow and brittle, and
# pgrx pins a specific Rust/PG combo that must be kept in lockstep with the base.
#
# To add it, either:
#   (a) add a dedicated Rust builder stage:
#         cargo install --locked cargo-pgrx@<ver matching pg_graphql>
#         cargo pgrx init --pg${PG_MAJOR} $(which pg_config)
#         cd pg_graphql && cargo pgrx install --release
#       then COPY pg_graphql.so + pg_graphql--*.sql + pg_graphql.control across,
#       and probably restrict that path to linux/amd64 (see workflow note); or
#   (b) pull a prebuilt artifact matching this PG major + arch from a trusted
#       release and COPY it into the extension dirs above.
# Until then, GraphQL-over-Postgres is unavailable on this image.
# -----------------------------------------------------------------------------

# Back to the CNPG runtime user. Do NOT change the entrypoint/cmd — the operator
# drives initdb/postgres through the base image's own ENTRYPOINT.
USER 26
