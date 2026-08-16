---
date: 2026-08-16
status: closed
incident_status: mitigated
sessions:
  - 01a008a2-112d-79d3-91bb-1fcc216e16a4
components:
  - kube-apiserver
  - crossplane
  - argocd
  - macintel01
symptoms:
  - macintel01 control-plane CPU sustained at 87–99%
  - kube-apiserver consumed 2.2–3.2 CPU cores and returned Handler timeout errors
  - API reads for Pods, CNPG Cluster, metrics, and leader-election leases timed out
failure_mode: crossplane-finalizer-retry-storm
affected_urls:
  - https://api.k8s.lan:6443
beads: [home-systems-cnm]
memories:
  - crossplane-finalizer-retry-storm-macintel01-2026-08-16
supersedes: []
related:
  - 2026-07-05-argocd-crossplane-webhook-blocks-sync
  - 2026-07-13-apiserver-crd-cache-oom
---

# Postmortem: Crossplane finalizer retry storm saturates kube-apiserver

- **Severity/Impact:** The active control plane, `macintel01`, remained Ready but
  sustained 87–99% CPU. `kube-apiserver` used 2.2–3.2 cores and timed out
  ordinary Kubernetes API requests, including Pod lists, CNPG Cluster reads,
  metrics scrapes, leases, and ArgoCD event writes. GitOps applications remained
  mostly reconcilable; no cloud bucket objects or Proxmox VMs were deleted.
- **Root cause (one line):** crossplane-finalizer-retry-storm — Crossplane
  Provider/ProviderRevision deletion left Proxmox CRDs and managed resources
  terminating for six days, while a non-empty billing-function source bucket
  was incorrectly configured for destructive deletion; kube-apiserver retried
  every blocked CRD finalizer continuously.

## What Happened

The control-plane host was `macintel01`, not `dell01`. Initial observation
showed it at 94% CPU, with `kube-apiserver` consuming 2.3 cores. Its logs
showed repeated CRD cleanup failures for four Proxmox resource families and
one GCP Storage Bucket, interleaved with request `Handler timeout` errors.

The Proxmox Provider and its sole ProviderRevision had both entered foreground
deletion on 2026-08-10, while their managed resources were still terminating.
Their CRDs retained cleanup finalizers, so kube-apiserver retried finalization
for `ProviderConfig`, `EnvironmentFile`, `EnvironmentVM`, and
`EnvironmentDownloadFile`. The Provider remained Git-declared but could not
recreate its controller until stale ownership metadata from the dead revision
was released.

Separately, `functions-src` was a non-empty GCS bucket containing
content-addressed billing-function source archives. Private values explicitly
overrode the chart default with `deletionPolicy: Delete`; when the managed
resource was pruned, the GCP provider correctly refused to delete the non-empty
bucket without `forceDestroy`, leaving it finalizing since 2026-08-09.

## Detection Gap

- **What the user saw first:** high control-plane CPU.
- **What existed:** Gatus has an external kube-apiserver liveness check, but it
  still targets the former control-plane node `dell01`; it checks availability,
  not elevated API latency. The in-cluster control-plane VMRule also detects
  only total probe failure.
- **What did not exist:** an actionable alert for a live but persistently slow
  Kubernetes API, and routing labels on the current control-plane VMRule that
  match the Discord receiver.

## Mitigation / Runbook

1. Identify a retry storm with:
   `kubectl logs -n kube-system kube-apiserver-<cp> --since=5m | grep
   crd_finalizer.go:302`. Enumerate terminating instances for every referenced
   CRD before changing any lifecycle metadata.
2. Restore the Git-declared Crossplane provider with a durable runtime path.
   Commit `09cabfdc` made `provider-proxmox-bpg` use the existing direct
   apiserver, amd64-only runtime configuration. ArgoCD recreated it on `dell01`.
3. When a foreground-deleting package revision prevents its declared
   replacement from starting, release only the stale package finalizers and
   ownership references that name the dead revision UID. Preserve managed
   resource specs and their Crossplane finalizers so the recreated controller
   performs normal cleanup.
4. For non-empty imported or operational buckets, preserve the real resource
   declaratively. Commit `704be76` changes `functions-src` to
   `deletionPolicy: Orphan`; release the old failed managed-resource finalizer
   only after the Git declaration is pushed and ArgoCD has refreshed.
5. Verify no terminating resources remain for the implicated CRDs and that
   `crd_finalizer.go:302` no longer appears in recent API-server logs.

## Mitigation Applied

- `09cabfdc` in `home-systems`: Proxmox provider uses the direct-apiserver
  `gcp-storage-pinned` runtime configuration.
- ArgoCD recreated a healthy Proxmox ProviderRevision on `dell01`.
- All stranded Proxmox `ProviderConfig`, `EnvironmentFile`, `EnvironmentVM`,
  and `EnvironmentDownloadFile` resources cleared.
- `704be76` in `home-systems-values`: `functions-src` now uses
  `deletionPolicy: Orphan`, preserving billing-function deployment artifacts.
- The failed GCP bucket managed-resource record was removed without deleting
  the actual GCS bucket.

At the last measurement, all five implicated resource families had zero
terminating objects and API-server CRD-finalizer errors were absent. The
control plane remained CPU-saturated with unrelated Pod-list, CNPG-read, and
metrics-request timeouts, so this incident is **mitigated**, not resolved.

## Approved Follow-ups

The approved implementation ledger is
`.agents/postmortems/2026-08-16-crossplane-finalizer-retry-storm/FP.md`.
It tracks Gatus endpoint correction, production-routed API-latency alerting,
and the remaining root-cause investigation.

## Dead Ends

- Initially treated `dell01` as the control plane. Cluster node roles showed
  `macintel01` was the active control plane; `dell01` was a worker.
- Attempted to solve the problem solely by changing the Proxmox provider runtime.
  The existing Provider and ProviderRevision were already in foreground deletion,
  so desired state alone could not cancel their deletion timestamps.
- Considered `forceDestroy` for `functions-src`. Rejected: the bucket contains
  billing-function source artifacts and the correct durable policy is `Orphan`.
- The local ArgoCD CLI had no configured server address. Existing Application
  objects were reconciled through their GitOps sync operations instead.

## Timeline

### 2026-08-16 (GMT-0300)

- `~00:30` User reports high control-plane CPU.
- `~00:35` Observed `macintel01` at 94% CPU; kube-apiserver used 2.3 cores.
- `~00:40` API-server logs show repeated Proxmox and GCP Storage CRD finalizer
  timeouts; 40 handler timeouts in ten minutes.
- `~00:45` Found Proxmox Provider and ProviderRevision foreground-deleting
  since 2026-08-10, with eight managed resources still terminating.
- `~00:50` Committed and pushed `09cabfdc`; ArgoCD recreated the Proxmox
  provider using the direct API runtime on `dell01`.
- `~01:00` Released stale package lifecycle ownership from the dead revision;
  the new Proxmox revision became healthy and cleared all eight resources.
- `~01:10` Identified `functions-src` as a non-empty bucket stuck because its
  private values set `deletionPolicy: Delete`.
- `~01:15` Committed and pushed `704be76` in `home-systems-values`, changing
  the bucket to `deletionPolicy: Orphan`; released only the failed managed
  resource finalizer after GitOps refresh.
- `~01:20` Verified zero terminating Proxmox and GCP Bucket resources and no
  remaining CRD-finalizer errors. API latency and high control-plane CPU persist
  from a separate cause.
