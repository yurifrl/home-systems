---
date: 2026-07-05
status: implemented
incident_status: resolved
sessions:
  - 019f3230-1736-7b7d-a71d-58b37aa4133c
components:
  - argocd
  - crossplane
  - pc01
symptoms:
  - all ArgoCD Applications ComparisonError
  - conversion webhook Post https://provider-gcp-iam.crossplane-system.svc:9443/convert timeout
  - cluster-wide GitOps sync halted
failure_mode: conversion-webhook-blocks-argocd-sync
affected_urls:
  - https://argocd.syscd.live
beads: [home-systems-57s]
memories:
  - argocd-crossplane-webhook-blocks-sync-2026-07-05
supersedes: []
related:
  - 2026-07-05-pc01-vxlan-tx-checksum-offload/PM.md
---

# Postmortem: ArgoCD cluster sync fully blocked by an unreachable crossplane conversion webhook

- **Severity/Impact:** All ArgoCD Applications stuck in `ComparisonError` /
  `Unknown` — cluster-wide GitOps reconciliation halted for at least
  ~09:15–09:26 GMT-0300 (likely longer before noticed). No data loss; running
  workloads unaffected.
- **Root cause (one line):** conversion-webhook-blocks-argocd-sync — one
  unreachable `*.upbound.io` conversion webhook (provider pod on broken pc01)
  aborts ArgoCD's shared cluster cache sync.

## What Happened

Crossplane upbound GCP provider CRDs (`*.upbound.io`) use
`spec.conversion.strategy: Webhook`. The backing provider pods
(`provider-gcp-iam`, `-storage`, `-cloudplatform`) run on **pc01**, whose
cross-node datapath is broken (VXLAN TX checksum offload — separate incident).
When the apiserver LISTs one of those CRDs it calls the conversion webhook on
pc01, which times out. ArgoCD's application-controller builds **one shared
cluster-wide resource cache** by LISTing every resource type; a single failing
LIST aborts the entire cache sync, so **no** Application could compute state —
one dead webhook took down all of GitOps. A conversion webhook has no
`failurePolicy` escape hatch.

## Detection Gap (how we catch it next time)

- **What the user saw first:** every app in the ArgoCD UI showing
  `ComparisonError`; nothing syncing.
- **How we detect it before the user next time:**
  `argocd_cluster_connection_status == 0` — the application-controller sets
  this when it cannot sync the cluster cache, the exact symptom, regardless of
  cause (webhook, apiserver, RBAC). The existing gatus check on
  `argocd.syscd.live` does NOT catch this: the UI stays up (200) while sync is
  dead.
- **Fix path once detected:** find the failing LIST in
  application-controller logs, then the runbook below (exclusion + break-glass).

## Mitigation (runbook — how to detect & fix this again)

**Durable fix (applied, committed `ea69d394`):** in
`manifests/values/argocd.yaml` under `configs.cm`, set `resource.exclusions`
to include `apiGroups: ["*.upbound.io"] kinds: ["*"] clusters: ["*"]` PLUS the
argo-cd chart's default exclusion blocks (Endpoints/EndpointSlice, Lease,
authn/authz reviews, CSR, cert-manager CertificateRequest, cilium
Identity/Endpoint/EndpointSlice, kyverno reports) — setting the key replaces
the chart default wholesale. ArgoCD stops watching upbound CRDs; crossplane
still reconciles the managed resources itself.

**Break-glass when ArgoCD is too broken to sync itself:**
1. Prepend the `*.upbound.io` block to the live `argocd-cm`
   `resource.exclusions` key (kubectl patch — the ONE sanctioned direct patch;
   git is still the source of truth).
2. `kubectl delete pod -n argocd argocd-application-controller-0` to force
   cache re-init (controller reads `argocd-cm` at startup).
3. Commit + push FIRST, then hard-refresh the `argocd` app — otherwise
   auto-sync (automated, selfHeal:false) re-renders `argocd-cm` from the stale
   repo-server cache and wipes the patch. If you hard-refresh before the
   repo-server has fetched your commit, re-patch and refresh again.

**Verify:** `argocd` app shows `sync=Synced` with empty `.status.conditions`;
`kubectl get applications -n argocd` shows the fleet reconciling instead of
all `ComparisonError`.

**Latent same trap:** `metallb.io` CRDs also use webhook conversion and the
metallb controller also runs on pc01 — the only other webhook-conversion CRD
group on this cluster besides `*.upbound.io`. **Now also excluded** (committed
with the follow-ups). CAVEAT: metallb `IPAddressPool`/`L2Advertisement` are
argo-managed (`support-cluster/templates/metallb.yaml`), so ArgoCD no longer
applies git changes to them — to edit the pool, temporarily remove the
`metallb.io` block from `resource.exclusions`, sync, then re-add
(tracked in `home-systems-1l9`).

## Follow-ups Implemented (epic home-systems-57s)

While implementing, found ArgoCD had **zero** metrics in VictoriaMetrics: the
argo-cd chart's `serviceMonitor.enabled` renders a
`monitoring.coreos.com/ServiceMonitor`, and this cluster has no such CRD (it
uses `VMServiceScrape`). So the `argocd-operational-overview` dashboard was
dataless and the detection metric above didn't exist. Fixed first.

- **Scrape:** `support-cluster/templates/monitoring/argocd-vmservicescrape.yaml`
  — one `VMServiceScrape` (`app.kubernetes.io/part-of: argocd`, port
  `http-metrics`) covering controller/server/repo/appset/notifications/redis/dex.
- **Alert:** `support-cluster/templates/monitoring/argocd.yaml` — VMRule
  `ArgoCDClusterCacheDown` (`min(argocd_cluster_connection_status) < 1` for
  `10m`, `severity: critical` + `environment: production` → Discord). Symptom
  alert; description links this file.
- **Dashboard:** no change — `argocd-operational-overview.json` already has the
  connection-status panels; it was dataless only from the missing scrape.
- **Rejected:** per-pod/per-webhook cause alerts (noisy, superseded); `cilium
  monitor`/VXLAN drop detail (diagnostic logs, never paged on).
- **Open verification:** `home-systems-meb` (metrics actually flow +
  dashboard lights up), `home-systems-gqy` (alert fires + routes to Discord).

### Caveat: the alert cannot fire yet (monitoring is dark)

While verifying, discovered the whole VictoriaMetrics stack (vmagent, vmalert,
vmalertmanager) is scheduled on **pc01**, whose VXLAN TX-checksum-offload fault
corrupts encapsulated TCP — so vmagent cannot remote-write to vmsingle and
cluster-wide metrics have been **dark for ~19h** (`count(up) == 0`). The new
`ArgoCDClusterCacheDown` VMRule is deployed and correct (the controller does
expose `argocd_cluster_connection_status`, the scrape target is `up`) but
**nothing can fire until metrics ingest again**. A `nodeAffinity` to pin the
stack off the broken nodes was tried and **reverted** (`c1e6cf8d`) — it masks
the node fault instead of fixing it. Tracked as **`home-systems-k5b`** (P1):
fix the pc01 datapath, then metrics + alerting recover on their own.

## Dead Ends

- Deleting the `provider-gcp-iam` pod to reschedule it — amd64 pin + no
  healthy amd64 node meant it just went `Pending` (and left the webhook with
  zero endpoints, turning "timeout" into "connection refused").
- Considered rebooting pc01 — correct for the node, but doesn't address the
  ArgoCD fragility, and the datapath fault reverts on reboot anyway (offload
  defaults back on).
- First git-push + hard-refresh reverted the live patch (stale repo-server
  cache) — looked like the fix "didn't hold"; actually an ordering race.
- The transient `127.0.0.1:26443 connection refused` during patching was a
  brief dell01 sole-control-plane apiserver blip, unrelated to the fix.

## Timeline

### 2026-07-05 (GMT-0300)
- `09:15` `argocd` app records `ComparisonError`: conversion webhook for
  `iam.gcp.upbound.io/v1beta1 WorkloadIdentityPoolProvider` failed, `Post
  https://provider-gcp-iam.crossplane-system.svc:9443/convert` deadline exceeded.
- `09:17` Same error recurs (live + target state); `UnknownError` on cache
  sync. User reports all sync blocked.
- `09:18` Confirmed `provider-gcp-iam` pod is on pc01; pc01 is
  `Ready,SchedulingDisabled` (cordoned from prior incident).
- `09:19` Deleted provider pod → `Pending` (amd64 pin, no healthy amd64 node).
- `09:21` Fresh netshoot on tp4: ICMP to pc01 pod 0% loss, TCP to pc01
  coredns:53 timeout — fault is inbound-TCP-to-pc01, not stale cilium state.
- `09:22` User redirected: fix ArgoCD's fragility, leave the node for another
  session.
- `09:24` Added `resource.exclusions` for `*.upbound.io` to
  `manifests/values/argocd.yaml`, preserving chart default exclusions.
- `09:25` Break-glass live `argocd-cm` patch; restarted repo-server; deleted
  `argocd-application-controller-0`. Transient apiserver blip mid-patch.
- `09:26` Committed `ea69d394`, pushed. Auto-sync briefly wiped the patch from
  stale repo-server cache (different upbound group tripped the same trap);
  re-patched + hard refresh after repo-server fetched the new commit.
- `09:28` Stable: `argocd` app `Synced`, no error conditions; 40 apps
  reconciling (30 `Synced/Healthy`).
