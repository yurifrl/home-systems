# syscd Temporal rollout checklist (PR description draft)

Design: `syscd-app/docs/jobs-temporal-design.md` (Q12-Q14, Q20). Merging to `main`
deploys to PROD via ArgoCD auto-sync. Do the manual steps below BEFORE merge.

## What this branch adds
- `postgres.yaml`: CNPG role `syscd_temporal`; databases `syscd_temporal` and
  `syscd_temporal_visibility` (retain).
- `syscd-temporal.yaml` (new Application, ns `syscd-app`): temporalio/helm-charts
  `temporal` 1.8.0 (server 1.32.2), 1 replica each frontend/history/matching/worker,
  postgres12 SQL persistence + visibility, namespace `syscd`, TenantId (Keyword)
  search attribute (PostSync Job), UI at `temporal.syscd.tech` (tailnet only),
  ExternalSecrets for the DB password.
- `syscd-app.yaml`: Deployments `syscd-worker` (`bun server/worker.ts`) and
  `syscd-admin`; TEMPORAL_* / JOBS_INSTANCE_ID / TEMPORAL_UI_URL / ADMIN_ORIGIN env on
  `syscd-api`; Service + VirtualService for `admin.syscd.live` / `admin.syscd.tech`.
- `support` chart: Deployments now accept optional `command` / `args`.
- values submodule (branch `syscd-temporal-admin`, NOT pushed): Cloudflare Access app
  for `admin.syscd.live` using the Admin policy. The gitlink in this branch points at
  commit `dea9134`, which exists only in the local submodule clone.

## Manual steps (in order)
1. 1Password (Kubernetes vault): add field `TEMPORAL_DB_PASSWORD` to item `syscd-app`
   (strong random). Nothing else is needed for Temporal itself.
2. Push the values-submodule branch `syscd-temporal-admin` and merge it to its `main`
   first, then bump the gitlink in home-systems (otherwise Argo cannot fetch `dea9134`).
   Cloudflare Access app `admin.syscd.live` is applied by the cloudflare-access
   Application; the DNS record for admin.syscd.live comes from external-dns via the
   VirtualService.
3. syscd-app repo (not touched here) must ship, before the new Deployments can start:
   - `server/worker.ts`, Debian-based `Dockerfile.api` (Temporal core-bridge fails on
     Alpine), `@temporalio/*` deps;
   - image `ghcr.io/yurifrl/syscd-app-admin` (Vite static build, busybox httpd :8080);
   - `k8s/values.yaml` `imageTags`: add `syscd-worker` (same tag as `syscd-api`) and
     `syscd-admin` (release workflow). Without pins both fall back to `latest`
     (admin image missing => ImagePullBackOff until first release).
4. Merge order / sync waves (no explicit waves needed; ordering is by retry):
   a. `postgres` app: creates role + 2 DBs (needs ExternalSecret
      `syscd-temporal-db-roles` in ns `databases`, from `syscd-temporal` app, so sync
      `syscd-temporal` once right after, or sync both together; role reconcile retries).
   b. `syscd-temporal` app: schema Jobs (plain Jobs, `useHelmHooks: false`) retry
      until DB + secret exist; then server pods, namespace Job (`syscd`), then PostSync
      Job creates `TenantId`.
   c. `syscd-app` app (worker/admin/api env). The worker will crash-loop until the
      frontend Service answers; harmless.
5. Verify: `kubectl -n syscd-app exec deploy/syscd-temporal-admintools -- temporal
   operator namespace describe syscd` and `... operator search-attribute list
   --namespace syscd` shows TenantId; UI reachable on the tailnet at
   https://temporal.syscd.tech.

## Risks
- CNPG backups are DISABLED (`postgres.yaml` `backups.enabled: false`, TODO
  home-systems-jfr.4). Losing the `pg` cluster loses the syscd ledger AND Temporal
  history. Enable backups, or accept this explicitly, before relying on it.
- Resources are guesses (history 768Mi limit); watch with goldilocks.
- `numHistoryShards: 512` is immutable once the DB is initialised.
- Temporal frontend has no authN/TLS; it is cluster-internal only (ambient mesh) and
  the UI is tailnet-only. Do not expose either on cloudflare domains.
- Postgres 16 / `postgres12` plugin: supported; chart schema version tracked by
  the chart's Jobs (`temporal-sql-tool update-schema`) on every chart upgrade.
