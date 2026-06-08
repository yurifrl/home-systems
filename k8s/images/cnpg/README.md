# cnpg-postgres — Supabase-on-CloudNativePG operand image

A PostgreSQL container image for running **Supabase on CloudNativePG (CNPG)**.
It extends the upstream CNPG operand image and bakes in the extensions Supabase
expects, so a BYO-Postgres CNPG `Cluster` can serve Supabase workloads.

- **Base image:** `ghcr.io/cloudnative-pg/postgresql:16.10-system-trixie`
  (Debian *trixie*, PGDG packages, runs as the `postgres` user uid **26**).
- **PostgreSQL major:** 16
- **Published as:** `ghcr.io/yurifrl/cnpg-postgres`
  (tags: `latest`, `<pg-version>` e.g. `16.10`, `sha-<commit>`)

The image is built by `.github/workflows/build-cnpg.yaml` on pushes to `main`
that touch `k8s/images/cnpg/**`, multi-arch for `linux/amd64,linux/arm64`.

## Design

Two-stage build. The **builder** stage starts from the *exact same* CNPG base,
adds the toolchain + `postgresql-server-dev-16` and the C dev libs, and compiles
each extension with pgxs so the install paths and ABI match the base PG. The
**final** stage starts from the pristine base again, installs only the runtime
shared libs (`libsodium23`, `libcurl4`), and copies the built `.so` / `.control`
/ `.sql` artifacts. The entrypoint, initdb behaviour, and uid 26 are unchanged,
keeping the image a drop-in CNPG operand.

## Extension status

| Extension        | Status            | Notes |
|------------------|-------------------|-------|
| `wal2json`       | included (source) | logical-decoding JSON output plugin; plain pgxs build (`wal2json_2_6`). |
| `pgjwt`          | included (source) | SQL-only JWT helpers; depends on `pgcrypto` (contrib, present). |
| `pgsodium`       | included (source) | libsodium-backed crypto; built against `libsodium-dev`, runs against `libsodium23` (`v3.1.9`). |
| `pg_net`         | included (source) | async HTTP via libcurl + background worker; built against `libcurl4-openssl-dev`, runs against `libcurl4` (`v0.14.0`). |
| `supabase_vault` | included (source) | secrets storage; pairs with `pgsodium` at runtime (`v0.3.1`). |
| `pg_graphql`     | **TODO**          | Rust/pgrx extension. Needs the full Rust toolchain + `cargo pgrx init` pinned to this PG major; slow/brittle under arm64 QEMU. See the TODO block in the `Dockerfile` for the two ways to add it (dedicated pgrx builder stage, or COPY a prebuilt artifact). |

### Runtime configuration notes (not handled by the image)

Some extensions need cluster-level config the **operator/Cluster spec** must set,
not the Dockerfile:

- `pg_net` and `pgsodium` require entries in `shared_preload_libraries`.
- Each extension still needs `CREATE EXTENSION ...` (typically via the CNPG
  `postBootstrap`/managed roles or a migration) before use.

## How it is consumed by CNPG

Point the CNPG `Cluster` at this image via `spec.imageName`:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: supabase-db
spec:
  imageName: ghcr.io/yurifrl/cnpg-postgres:16.10
  instances: 3
  postgresql:
    shared_preload_libraries:
      - pg_net
      - pgsodium
  # ... storage, bootstrap, etc.
```

(Wiring the Cluster into the GitOps tree lives under `k8s/applications/` and
`k8s/charts/`, which are owned elsewhere — this directory only produces the
image.)
