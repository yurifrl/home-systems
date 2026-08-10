---
date: 2026-08-10
status: closed
incident_status: resolved
sessions:
  - 019feb56-7bcf-7e78-89c2-f1fbe9684d8c
components:
  - longhorn
  - dell01
  - hermes
symptoms:
  - hermes-0 stuck Init:0/2 for ~8h+, StatefulSet 0/1
  - 'AttachVolume.Attach failed ... CSINode dell01 does not contain driver driver.longhorn.io'
  - 'CSINode dell01 spec.drivers = null'
  - 'attach 500: data engine image longhorn-engine not deployed on dell01'
  - RWO volume stale-attached to a node with no consumer, blocking reattach
failure_mode: longhorn-daemonset-node-affinity-drift
affected_urls:
  - https://hermes.syscd.live
beads:
  - home-systems-5sw.1
  - home-systems-k5b
  - home-systems-ujs
memories:
  - longhorn-daemonset-node-affinity-drift-dell01-2026-08-10
supersedes: []
related:
  - 2026-07-05-hermes-rwx-sharemanager-cross-site
  - 2026-07-13-apiserver-crd-cache-oom
---

# Postmortem: Longhorn DaemonSets excluded from dell01 by stale node-isolation affinity

- **Severity/Impact:** hermes (agent `hermes-0`, `hermes-pages`, `hermes-repository-sync`) could not start for ~8h+ — pods stuck `Init` because their Longhorn volumes would not attach on dell01. Same root cause silently blocked `home-assistant-0` and `presenter` (also scheduled on dell01). User-facing hermes was down; the user was the monitor.
- **Root cause (one line):** `longhorn-daemonset-node-affinity-drift` — three Longhorn DaemonSets (`longhorn-manager`, `longhorn-csi-plugin`, `engine-image-ei-a4d05f02`) carried a manual `kubectl-patch` node-affinity `kubernetes.io/hostname NotIn [dell01]`, leftover from the 2026-07-13 sole-control-plane isolation; dell01 was demoted to a worker on 2026-08-05 but the exclusion was never removed, so dell01 had no CSI node plugin / engine and could not attach volumes.

## What Happened

On 2026-07-13, during the `apiserver-crd-cache-oom` incident (see related), dell01 was the sole control-plane and was isolated from workloads. Part of that work — a manual `kubectl patch` at `14:06`, never committed to git — added a node-affinity to the `longhorn-manager`, `longhorn-csi-plugin`, and `engine-image-ei-a4d05f02` DaemonSets that excluded dell01 (`kubernetes.io/hostname NotIn [dell01]`). Because the Longhorn Helm chart renders **no** affinity, ArgoCD's server-side-apply never owned that field and never reverted the drift.

On 2026-08-05 dell01 was demoted from control-plane to a regular worker (macintel01 is now the sole control-plane). dell01 is a legitimate compute node again, and the scheduler placed hermes, home-assistant, and presenter there. But the stale exclusion meant dell01 ran no `longhorn-manager`, no `longhorn-csi-plugin`, and no engine-image pod. `CSINode dell01` had `spec.drivers = null`, so every `AttachVolume.Attach` for a pod on dell01 failed with `CSINode dell01 does not contain driver driver.longhorn.io`. The four CSI controller sidecars scheduled on dell01 (attacher/provisioner/resizer/snapshotter) crashlooped because the node plugin's `/csi/csi.sock` never existed.

A second, independent problem surfaced once storage was fixed: `hermes-memory-backup` crashlooped cloning `github.com/yurifrl/hermes-memory.git` with *"Repository not found"* — the fine-grained GitHub PAT in 1Password item `hermes-env` (synced to the `hermes-env` Secret via ESO) lacked access to that private repo. Unrelated to the storage fault; documented here because it was part of getting the deploy fully green.

## Detection Gap (how we catch it next time)

- **What the user saw first:** "hermes deploy has an error — dell01 doesn't have `driver.longhorn.io`" — the human was the monitor, ~8h into the outage.
- **Why nothing paged (the real gap):** a `HermesDown` VMRule (StatefulSet 0 ready replicas >10m) AND a `KubePodNotReady` VMRule (pod Pending/Init >20m) **both already exist** and both should have fired within 10–20m. They are **inert**: `kube_statefulset_status_replicas_ready{namespace="hermes"}` returns **zero series** in VictoriaMetrics. kube-state-metrics is chronically broken — 3 of 4 `vmks-kube-state-metrics` pods are in `Error`/CrashLoopBackOff (100–1174 restarts), the survivor logs `Failed to write metrics ... connection reset by peer`. With no `kube_*` metrics, every kube-state-derived alert is dark. This is the **third** witness of the same dark-metrics failure class after `2026-07-05-hermes-rwx` and `2026-07-13-apiserver-crd-cache-oom`; it is already tracked by open beads `home-systems-5sw.1` (fix kube-state-metrics / re-arm KubePodNotReady) and `home-systems-k5b` (VM stack dark). This incident does not need a new in-cluster alert — it needs those to land.
  - **KSM darkness fixed 2026-08-10 (close-out):** root cause of the dark `kube_*` metrics was **not** KSM being unhealthy — KSM's `/metrics` served in 0.68s. It was vmagent (CPU-limited, on the rpi01 Pi) timing out KSM's ~3.4MB scrape at the default 10s (`context deadline exceeded` → `up=0` → every `kube_*` series absent → all KSM-derived alerts inert). Fix: rescheduled KSM off the flaky macintel01 node onto pc01, and committed a durable `VMServiceScrape` override (`interval: 60s / scrapeTimeout: 30s`, KSM state changes slowly) in `k8s/applications/victoria-metrics-k8s-stack.yaml`. Verified end-to-end: vmagent config now `scrape_interval: 1m0s / scrape_timeout: 30s`, `up{job=kube-state-metrics}=1`, `kube_statefulset_status_replicas_ready{namespace="hermes"}=1`, 197 `kube_*` names flowing — so `HermesDown`/`KubePodNotReady` are **re-armed**. Remaining verification (that they actually fire + route to Discord) is tracked in `home-systems-ujs`.
  - **External check still wanted:** `home-systems-ujs` (P3) also carries the gatus check for `hermes.syscd.live` — an internet→service signal independent of the VM/KSM pipeline that has now gone dark three times.
- **KSM-independent symptom (the concrete new delta):** there is **no** gatus check for `hermes.syscd.live`. Every in-cluster alert that would catch "hermes down" depends on the same VM/KSM pipeline that has been dark repeatedly. An external gatus check on `hermes.syscd.live` tests the internet→service path from outside and fires via Discord independent of the cluster metrics stack — the one signal that would have paged here regardless of KSM. hermes is Cloudflare-Access protected (`*.syscd.live`), so the check needs the `CF-Access-Client-Id/Secret` headers (same pattern as the ArgoCD entry).
- **Fix path once detected:** runbook below (check `CSINode <node>` for the driver; if `drivers: null`, look for a stray `NotIn <node>` affinity on the Longhorn DaemonSets).

## Mitigation (runbook — how to detect & fix this again)

**Symptom:** a pod stuck `Init`/`Pending` on a specific node; `kubectl -n <ns> describe pod` shows `AttachVolume.Attach failed ... CSINode <node> does not contain driver driver.longhorn.io`.

**Diagnose:**
```
kubectl get csinode <node> -o jsonpath='{.spec.drivers[*].name}{"\n"}'   # empty = no Longhorn plugin on node
kubectl get pods -n longhorn-system -o wide | grep <node>                 # no longhorn-manager / -csi-plugin / engine-image pod?
# find the stray exclusion (drift, not in git):
kubectl get ds longhorn-manager   -n longhorn-system -o jsonpath='{.spec.template.spec.affinity}{"\n"}'
kubectl get ds longhorn-csi-plugin -n longhorn-system -o jsonpath='{.spec.template.spec.affinity}{"\n"}'
kubectl get ds engine-image-ei-<hash> -n longhorn-system -o jsonpath='{.spec.template.spec.affinity}{"\n"}'
```
A `nodeAffinity ... kubernetes.io/hostname NotIn [<node>]` on any of the three is the smoking gun. Confirm it is manual drift (not git):
`kubectl get ds longhorn-manager -n longhorn-system --show-managed-fields -o json | jq '..|.manager? // empty' | sort -u` → `kubectl-patch` owning the affinity.

**Fix (remove the drift — the chart renders no affinity, so removal is git-aligned and ArgoCD will not re-add it):**
```
kubectl patch ds longhorn-manager       -n longhorn-system --type=json -p '[{"op":"remove","path":"/spec/template/spec/affinity"}]'
kubectl patch ds longhorn-csi-plugin     -n longhorn-system --type=json -p '[{"op":"remove","path":"/spec/template/spec/affinity"}]'
kubectl patch ds engine-image-ei-<hash>  -n longhorn-system --type=json -p '[{"op":"remove","path":"/spec/template/spec/affinity"}]'
```
Order matters and it is slow to converge:
1. Removing the manager/csi-plugin affinity gets `driver.longhorn.io` onto `CSINode <node>`. **Caveat:** the DaemonSet uses `updateStrategy.maxUnavailable: 100%`, so the patch restarts **every** longhorn-manager at once. They crashloop for ~10–15m during convergence (`fatal: Error starting webhooks: ... the object has been modified`, webhook leader-election churn against the single Tailscale apiserver backend, slow initial datastore cache-sync on the 8 GB dell01). This is expected — they were all 2/2 before the patch and reconverge to 2/2. Wait it out; do not keep restarting.
2. The engine-image DaemonSet must ALSO be un-excluded or attach fails with `500 ... data engine image longhorn-engine not deployed`. Verify: `kubectl get engineimages.longhorn.io -n longhorn-system -o jsonpath='{.items[0].status.nodeDeploymentMap}'` shows `<node>:true`.
3. Verify the Longhorn node is usable: `kubectl get nodes.longhorn.io <node> -n longhorn-system -o jsonpath='{range .status.conditions[?(@.type=="Ready")]}{.status}{end}'` → `True`, and an `instance-manager` pod runs on `<node>`.

**Stale RWO VolumeAttachment (blocks reattach):** an RWO volume can stay bound to a node whose consumer is long gone. Symptom: `kubectl get volumes.longhorn.io <pvc> -n longhorn-system -o jsonpath='{.status.currentNodeID}'` shows the wrong node while the pod is elsewhere.
```
kubectl get volumeattachment | grep <pvc>     # find the ATTACHED=true one on the wrong node, with no pod there
kubectl delete volumeattachment <stale-va>     # controller recreates VAs as needed; releases the RWO for the correct node
```
Then delete the stuck pod so it re-issues a clean attach/mount (`SuccessfulAttachVolume` → kubelet `Pulling` = past the mount).

**dell01 is storage-capable, keep it so.** dell01 has a single NVMe and `node.longhorn.io/create-default-disk: "false"` (it hosts no replicas by design). That is fine and NOT the bug — the CSI node plugin only needs to *run* on dell01 to *attach* volumes whose replicas live elsewhere. Do not re-exclude Longhorn from dell01 when isolating a node in future; use taints/tolerations or Longhorn's own scheduling settings, not a hand-patched DaemonSet affinity.

**memory-backup git clone 404:** `hctl auth login` (the `git-login` init container) reads `GH_TOKEN`/`GITHUB_TOKEN` from Secret `hermes-env` (ns `hermes`), synced by ESO (ClusterSecretStore `onepassword`, ExternalSecret `hermes`) from the 1Password `hermes-env` item. A private-repo clone returning *"Repository not found"* means the fine-grained PAT lacks **Contents: Read and write** on that repo (fine-grained PATs 404 rather than 403). Fix in GitHub token settings (repo access + Contents perm), or recreate the token and paste the new value into the 1Password `hermes-env` item, then `kubectl annotate externalsecret hermes -n hermes force-sync=$(date +%s) --overwrite` and restart the pod.

## Dead Ends

- **Disk-split theory (user's first instinct):** "split dell01's disk, one partition for system, one for Longhorn storage." Wrong layer — dell01 has one NVMe and is intentionally a non-storage node (`create-default-disk: false`). The fault was the CSI node plugin being absent, not disk capacity; attaching a volume whose replica lives on tp1/tp4 needs only the node plugin running on dell01.
- **`node dell01 not found` (attach error #2)** looked like a fresh problem — it was just the dell01 `longhorn-manager` not yet `Ready` during the maxUnavailable-100% roll. Resolved itself as the manager finished cache-sync.
- **Manager crashloop looked like a permanent dell01→apiserver connectivity fault** (`context deadline exceeded` to `10.96.0.1:443`, leader lease acquired-then-lost). It was the simultaneous fleet restart hammering the single Tailscale apiserver backend + slow cache-sync; it converged without intervention.
- **memory-backup "Repository not found" read as a repo-scope problem** — the repo exists and the token was already "All repositories"; the real gap was the token value/Contents permission. Recreating the token fixed it.

## Timeline

### 2026-08-10 (UTC)
- `~03:xx` hermes-0 (and hermes-pages, hermes-repository-sync) sitting `Init:0/2` on dell01; `FailedAttachVolume: CSINode dell01 does not contain driver driver.longhorn.io` (x253 over ~8h). ArgoCD `hermes` app Degraded. No alert fired.
- `~11:00` User reports the dell01 CSI-driver error and asks to fix. Triage: `CSINode dell01 spec.drivers = null`; no `longhorn-manager`/`longhorn-csi-plugin`/`engine-image` pod on dell01; the four CSI sidecars on dell01 crashloop on missing `/csi/csi.sock`.
- `~11:02` Found `longhorn-manager` + `longhorn-csi-plugin` DaemonSets carry `nodeAffinity NotIn [dell01]`; `managedFields` attributes it to `kubectl-patch` at `2026-07-13T14:06Z` (the sole-CP isolation, commit `edd88ddc` same day). Not present in git.
- `~11:03` Removed the affinity from both DaemonSets. `driver.longhorn.io` registers on `CSINode dell01`; `longhorn-csi-plugin` runs 3/3 on dell01.
- `~11:04–11:20` `maxUnavailable: 100%` rolled all longhorn-managers at once → ~15m fleet crashloop (webhook `object has been modified` fatal, leader-election churn, slow cache-sync on 8 GB dell01). Converged to 2/2; dell01 Longhorn node `Ready=True`, instance-manager running.
- `~11:20` Next attach failed `500 ... data engine image longhorn-engine:v1.12.0 not deployed on dell01`. `engine-image-ei-a4d05f02` DaemonSet had the SAME `NotIn [dell01]` affinity. Removed it → engine-image pod runs on dell01, `nodeDeploymentMap dell01:true`.
- `~11:2x` workdir volume (`pvc-93765d6b`, longhorn-single) attached to dell01, but state volume (`pvc-7f563eff`, longhorn-ha, RWO) was stale-attached to **tp1** via an orphaned 13h-old k8s VolumeAttachment (no consumer on tp1; only hermes-dashboard runs there and it doesn't mount it). Deleted the stale VolumeAttachment → volume reattached to dell01.
- `~11:3x` Recreated hermes-0 → both volumes attached (`SuccessfulAttachVolume`), kubelet pulled the image, init containers ran, **hermes-0 1/1 Running**. hermes-pages and hermes-repository-sync (stuck 13h) recovered. `home-assistant-0` (2/2) and `presenter` (1/1) — same engine-image gap — also recovered.
- `~11:4x` `hermes-memory-backup` now runs but crashloops cloning `yurifrl/hermes-memory` (*Repository not found*) — fine-grained PAT in 1Password `hermes-env` lacks access. Traced the token chain (1Password `hermes-env` → ESO ExternalSecret `hermes` → Secret `hermes-env` → `envFrom` → `hctl auth login`).
- `~13:4x` User recreated the token (already "All repositories" + Contents R/W). Forced ExternalSecret resync + restarted memory-backup → clone + push succeeded (`f27892a..dca9f46 main -> main`). **All 5 hermes pods 1/1 Running.**
