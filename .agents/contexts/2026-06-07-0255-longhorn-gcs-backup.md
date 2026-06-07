---
created: 2026-06-07T02:55:00Z
project: home-systems
description: VictoriaMetrics GCS backup attempt; ended up clarifying VM has no S3-tiered storage and restoring old Prometheus/Grafana apps
context: monitoring backup strategy, crossplane-gcp buckets, VM vs Thanos/Mimir
tags: [victoriametrics, backup, gcs, crossplane, longhorn, thanos, mimir, monitoring]
session_name: 2026-06-07-0255-longhorn-gcs-backup
purpose: Set up off-cluster backup for VictoriaMetrics after a cluster wipe, then course-correct once the user clarified the real goal was S3-backed historical querying (which VM cannot do)
session_id: 019e9f20-dabb-7712-8b92-e7145be43ada
provider: pi
resume_with: cly agent-session resume --provider pi 2026-06-07-0255-longhorn-gcs-backup
context_name: 2026-06-07-0255-longhorn-gcs-backup
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/2026-06-07-0255-longhorn-gcs-backup.md
---

# Session: 2026-06-07-0255-longhorn-gcs-backup

- **Name:** 2026-06-07-0255-longhorn-gcs-backup
- **Purpose:** Back up VictoriaMetrics to GCS after a cluster wipe wiped Longhorn volumes; the session pivoted hard once the user clarified they actually wanted S3-backed on-demand historical querying (a Thanos/Mimir capability VM lacks).
- **Resume:** `cly agent-session resume --provider pi 2026-06-07-0255-longhorn-gcs-backup`

## Context
Home-lab Talos/k8s cluster managed by ArgoCD (GitOps). A cluster reprovision wiped all Longhorn volumes; VictoriaMetrics (`vmsingle-vmks`, monitoring ns, VM operator via `victoria-metrics-k8s-stack` app) lost its data. The repo is a public **VictoriaMetrics showcase**; real bucket names/secrets must live ONLY in the private repo `home-systems-values` (merged via ArgoCD `$values`), never in the public `home-systems`.

## Problem
1. VM had no off-cluster backup; wanted backups to GCS.
2. Crossplane GCP (providers healthy: storage/iam/cloudplatform v2.5.4) existed but `crossplane-gcp` ArgoCD app was excluded from reconciliation by a `resource.exclusions` workaround in `manifests/values/argocd.yaml` (added during a past dead-webhook outage).
3. Repeated agent failures: hand-rolled `while true; sleep 6h` vmbackup sidecar (a hack), a CronJob blocked by RWO PVC multi-attach, a leaked real bucket name into the public repo, and an accidental `git commit` that swept in another session's staged WIP.
4. **Root realization (late):** the user's actual goal was "query far-past data, slow first then cached" = object-storage-backed querying. **VictoriaMetrics does NOT support this** (local-disk only; S3 = backup/restore only). That behavior is Thanos / Grafana Mimir / Cortex.

## Decisions
- Created GCS buckets via Crossplane GitOps (`victoriametrics`, `longhorn`) with SA + key + IAM, declared in private `gcp/values.yaml`.
- Removed the `resource.exclusions` block from `manifests/values/argocd.yaml` (GCP providers verified healthy) so ArgoCD manages crossplane GCP resources — fixes wipe-survival instead of manual `kubectl apply`.
- Added `writeConnectionSecretToRef` to the ServiceAccountKey chart template so backup creds are produced declaratively.
- **Reverted the Longhorn-based VM backup** — Longhorn volume backup doesn't showcase VM. VM full-feature backup = `vmbackupmanager` = Enterprise (free trial license, user-only step). Not implemented.
- Anonymized `hack/lib/velero.yaml` (real bucket/project → placeholders).
- Saved a hard rule to `~/.agents/MEMORY.md`: avoid local `kubectl apply`; commit→push→ArgoCD sync.
- Restored the pre-migration `kube-prometheus-stack.yaml` + `grafana.yaml` apps, then moved both to `k8s/lib/` (archived).

## Current State
- Pushed (public `home-systems` + private `home-systems-values`): crossplane GCS buckets/SA/key/IAM, exclusion removal, velero anonymization, Longhorn-backup revert.
- `k8s/lib/grafana.yaml` and `k8s/lib/kube-prometheus-stack.yaml` restored and parked (uncommitted working-tree changes).
- `victoria-metrics-k8s-stack.yaml` reverted to single VM source (no backup sidecar). VM has NO working backup currently.
- vmbackupmanager (Enterprise) wiring NOT built — blocked on user obtaining a free trial license, and superseded by the realization that VM is the wrong tool for S3-tiered querying.
- A concurrent session is independently working the `support` chart `longhorn-backup-target` (`cdfa0c95`) — left untouched.

## Next Steps
1. Decide backend direction: keep **VM** (long retention = bigger PVC, no S3 query) vs adopt **Thanos** (recommended: more popular, CNCF, Apache-2.0) or **Mimir** (Grafana, AGPLv3) for S3-backed historical querying.
2. If staying on VM and only wanting DR backup: either OSS `vmbackup` (manual/CronJob, RWO-constrained) or Enterprise `vmbackupmanager` (needs free trial license in 1Password item `vm-enterprise-license`).
3. Decide fate of restored `k8s/lib/*` apps and whether to commit them.
4. `k8s/charts/support-grafana` chart is still missing (grafana.yaml depends on it) — restore if Grafana operator is revived.
