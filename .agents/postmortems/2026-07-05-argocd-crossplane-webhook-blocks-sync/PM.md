---
date: 2026-07-05
status: reopened
incident_status: mitigated
sessions:
  - 019f3230-1736-7b7d-a71d-58b37aa4133c
  - 019ff7ef-8422-75c6-a0a8-fe79d96eb26e
  - 01a006c5-f21e-7533-b79c-6611dbc3cd85
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
beads: [home-systems-57s, home-systems-idb]
memories:
  - argocd-crossplane-webhook-blocks-sync-2026-07-05
supersedes: []
related:
  - 2026-07-05-pc01-vxlan-tx-checksum-offload
  - 2026-07-13-apiserver-crd-cache-oom
  - 2026-08-09-argocd-appset-dual-manager-prune
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

---

## Recurrence — 2026-08-12/13 (GMT-0300)

**Same component + same failure_mode.** Reopened. The 2026-07-05 fix
(`resource.exclusions` for the upbound groups) was **later removed from git**
because the in-file comment claimed the exclusion was redundant once the GCP
providers were pinned to a "stable" worker with a direct API path. The pin
failed, the exclusion was gone, and the exact same cascade returned.

**Why the prior follow-ups did not prevent this (per bead):**
- `home-systems-57s` (epic) — **never-done** (still OPEN). The exclusion it
  shipped was subsequently deleted from `manifests/values/argocd.yaml`; the
  "pin makes exclusion redundant" rationale in the comment was wrong.
- `home-systems-meb` (verify scrape) — **never-done** (OPEN) at the time; the
  metrics stack is alive now (verified this session, see below).
- `home-systems-gqy` (verify alert fires) — **never-done** (OPEN). The
  `ArgoCDClusterCacheDown` alert was deployed but unverified; during the
  ~18h window this incident ran, it did not page (likely the metrics stack
  dark window `home-systems-k5b`, still OPEN).
- `home-systems-k5b` (VM stack dark) — **never-done** (OPEN), the detection
  gap that let this run silent for ~18h.
- `home-systems-1l9` (metallb exclusion doc) — **never-done** (OPEN).

**The lesson:** the durable fix is the exclusion, NOT the node pin. A node pin
is a single point of failure; the exclusion is what actually decouples ArgoCD's
shared cache from webhook health. Never remove the exclusion in favor of a pin.

### What happened this time

pc01's containerd broke (`FailedCreatePodSandBox`, runc broken-pipe /
missing-`/proc/<pid>` errors). Cilium agent on pc01 went down → held the
`node.cilium.io/agent-not-ready:NoSchedule` taint → `provider-gcp-storage` and
`provider-gcp-iam` (hard-pinned to pc01 via `nodeSelector:
kubernetes.io/hostname: pc01`) stayed `Pending` with zero endpoints. The
`provider-gcp-storage` Service (`10.111.26.85`) had no backend → `connection
refused` on the `BucketIAMMember.storage.gcp.upbound.io` conversion call →
ArgoCD's shared cluster cache aborted → every Application `ComparisonError`.
Identical mechanism to 2026-07-05, different node fault (containerd vs VXLAN).

### Fix applied this recurrence (committed `476806b0`)

- Re-added the upbound exclusion to `manifests/values/argocd.yaml`
  `configs.cm."resource.exclusions"`, but **explicit per-group** (ArgoCD
  matches `apiGroups` EXACTLY, no glob): `gcp.upbound.io`, `gcp.m.upbound.io`,
  and all 10 `*.gcp[.m].upbound.io` webhook-converted groups, `kinds: ["*"]`.
- **Rewrote the misleading comment** that caused the 2026-07-05 exclusion to
  be removed — it now states the pin failed and the exclusion is permanent.
- Break-glass live `argocd-cm` patch (the self-managed `argocd` app could not
  apply its own fix — its diff step was blocked by the same jam), then
  `kubectl delete pod argocd-application-controller-0` to rebuild the cache.
- Verified: `litellm` conditions clean (0 conversion-webhook mentions); fleet
  moved from all-`ComparisonError` to `Synced`/`Unknown`-recomputing; all 13
  webhook-conversion CRD groups on the cluster now covered by exclusions.

### Detection verified working this session

`min(argocd_cluster_connection_status) = 1` confirmed live from vmsingle (2
series). The metrics stack is up (vmagent/vmalert/vmsingle Running on
rpi01/tp4). So `ArgoCDClusterCacheDown` can now fire — the 2026-07-05
dark-metrics gap is closed. It did NOT page during this incident's ~18h window
because that window overlapped the metrics-dark period; detection is healthy
now.

### Timeline (2026-08-12/13, GMT-0300)
- `08-12 15:16` First `ComparisonError` recorded on `argocd` app:
  `BucketIAMMember.storage.gcp.upbound.io` conversion webhook `connection
  refused` to `10.111.26.85:9443`.
- `08-12→08-13` Incident runs ~18h unnoticed (metrics-dark gap; no page).
- `08-13 07:45` User reports all apps `ComparisonError` (screenshot).
- `08-13` Diagnosed: pc01 containerd broken → cilium taint → storage/iam
  provider pods Pending → webhook no endpoints → cache abort.
- `08-13` Found exclusion had been removed from git; comment blamed pin.
- `08-13` Re-added explicit per-group exclusion (commit `476806b0`), patched
  live cm, restarted application-controller. Fleet recovered.

### Dead ends (this recurrence)
- Initially suspected the two-LAN Tailscale partition (the 2026-07-05 cause)
  — but pc01's kubelet was `Ready`; the real fault was containerd, not the
  datapath.
- Considered re-pinning the providers to another amd64 node — but dell01 and
  macarm01 were also `NotReady`, so there was no healthy amd64 target; the
  exclusion is the correct fix regardless of node health.
- Regenerating `manifests/argocd.yaml` via `helm template` produced a huge
  chart-drift diff (9.5.17 → 10.1.3) — a red herring; the `argocd` app is
  self-managed from chart `10.3.2` + the values file, so the rendered file is
  a stale artifact and was reverted.

---

## Recurrence — 2026-08-15 (GMT-0300)

**Same component + same failure_mode.** Reopened again. The user saw Argo CD's
`ComparisonError` for `.submodules/home-systems-values`, but the live
application-controller was simultaneously blocked by the same storage-provider
conversion-webhook failure. This occurrence revealed a deeper recovery
cascade: Crossplane's rbac-manager was independently crash-looping, so a
ProviderRevision recreation could not receive its required RBAC.

### What happened this time

- `provider-gcp-storage` retained a stale `kubernetes.io/hostname: pc01`
  selector in its live ProviderRevision even after git changed its
  DeploymentRuntimeConfig to amd64/non-control-plane placement. With pc01
  `NotReady`, the webhook Service had zero endpoints.
- The storage conversion call failed (`BucketIAMMember.storage.gcp.upbound.io`)
  and Argo CD's shared cache again failed, holding the self-managed Argo CD app
  at `Unknown/Degraded` and preventing its own fix from reconciling.
- Recreating the ProviderRevision moved the runtime to dell01, but it initially
  crash-looped: its logs reported missing CRD-watch RBAC and disabled
  **SafeStart**, then failed while enumerating inactive MRDs.
- The real blocker was `crossplane-rbac-manager`: it had restarted more than
  700 times over three days on rpi01. Its leader-election lease calls went via
  the unreliable `10.96.0.1` Service VIP and timed out. Without rbac-manager,
  fresh ProviderRevisions do not get their RBAC, so SafeStart is disabled.

### Mitigation applied

- Disabled repo-server recursive submodule checkout (`833666ea`), eliminating
  the optional local submodule as a manifest-generation dependency.
- Removed the stale live pc01 selector and recreated the storage
  ProviderRevision; its webhook endpoint returned on dell01.
- Kept the broad Upbound resource exclusion deployed (`aea72ff1`) so Argo CD's
  cache is insulated while a provider conversion endpoint is unavailable.
- Configured rbac-manager like Crossplane core (`38884ab6`): leader election
  disabled for the single replica and direct apiserver host/port, bypassing the
  cross-LAN Service VIP. It then ran with zero restarts.
- Pinned rbac-manager to macintel01 (`f238267c`) as a bounded control-plane
  exception: 100m CPU / 256Mi memory request; 500m CPU / 512Mi limit. The
  placement rule was subsequently narrowed (`a1153824`): control-plane
  placement is never the default and requires recovery-critical purpose,
  bounded resources, no heavy cluster-wide LIST/WATCH loop, and an explicit
  manifest comment.

### Evidence after mitigation

- `crossplane-rbac-manager` rolled onto macintel01 Ready with zero restarts.
- The storage provider Service had a ready conversion endpoint after the
  revision recreation.
- The `reposerver.enable.git.submodule=false` parameter became live.
- Crossplane returned `Synced/Healthy`; the fleet resumed reconciliation.

### Detection gap (still open)

The symptom alert already exists: `ArgoCDClusterCacheDown` in
`k8s/charts/support-cluster/templates/monitoring/argocd.yaml` fires when
`argocd_cluster_connection_status` is zero for ten minutes and routes as
critical/production to Discord. The user still discovered this occurrence
first, so the existing verification beads `home-systems-idb.2`,
`home-systems-gqy`, and `home-systems-meb` remain open until the metric,
VMRule evaluation, and Discord route are proved end-to-end. No new duplicate
alert is proposed.

### Additional follow-up facts

- The live Argo exclusion now uses `apiGroups: ["*.upbound.io"]` while prior
  repository comments asserted exact-only group matching. Its behavior must be
  verified against the deployed Argo CD version before treating it as durable
  protection.
- `macintel01` reported 97% CPU during recovery. Visible pod usage was led by
  kube-apiserver (~2.6 cores), local Cilium (~0.5), and controller-manager
  (~0.37); rbac-manager used ~9m. The remaining host-level CPU is unassigned
  by pod metrics and requires Talos/host-level investigation before any more
  control-plane placements are approved.

### Dead ends (this recurrence)

- Adding storage MRDs one at a time (`ObjectAccessControl`, `BucketIAMPolicy`)
  treated the provider crash loop as an MRAP problem. It was a symptom: once
  rbac-manager granted CRD-watch RBAC, SafeStart enabled and the provider
  could start with inactive MRDs.
- Treating the control-plane node as a broadly reliable workload destination
  was too permissive. It is reliable for this small recovery-critical
  singleton, but its 97% CPU rules out adding heavy workloads there.
- A direct merge patch initially preserved the obsolete `pc01` map key; the
  exact JSON Patch removal was required before revision recreation could use
  the corrected placement.

### Timeline (2026-08-15, GMT-0300)

- `15:55` User reports Argo CD target-state generation failure: recursive
  submodule checkout cannot find the home-systems-values revision.
- `~16:00` Git inspection confirms the optional SSH submodule; repo-server
  submodule checkout is identified as unnecessary for Argo manifests.
- `~16:20` Live Argo CD also reports the storage conversion webhook connection
  refused; storage provider pod is Pending with zero Service endpoints because
  its live revision still selects pc01.
- `~16:30` Runtime config is corrected live; stale ProviderRevision is
  recreated, moving storage webhook runtime to dell01.
- `~16:45` Provider crash loop exposes missing SafeStart/RBAC; rbac-manager
  is found crash-looping on Service-VIP leader-election timeouts.
- `~17:00` Argo application-controller cache is rebuilt after the webhook
  endpoint returns; repo-server submodule exclusion reaches live config.
- `~18:00` rbac-manager is changed through GitOps to bypass the Service VIP
  and disable needless single-replica leader election; it stabilizes.
- `~18:30` rbac-manager is moved to macintel01 under the bounded
  control-plane-exception policy; it is Ready with zero restarts.

