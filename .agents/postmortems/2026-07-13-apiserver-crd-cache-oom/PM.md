---
date: 2026-07-13
status: closed
incident_status: mitigated
sessions:
  - 019f5987-1b4e-73c5-b00f-564ff2c98f03
components:
  - kube-apiserver
  - crossplane
  - etcd
  - dell01
symptoms:
  - connection refused to api.k8s.lan:6443
  - cluster down / kubectl unusable
  - kube-apiserver CrashLoopBackOff exit 137
  - Talos OOM controller SIGKILL of kube-apiserver cgroup
failure_mode: apiserver-crd-cache-oom
affected_urls:
  - https://api.k8s.lan:6443
beads: [home-systems-0r5]
memories:
  - apiserver-crd-cache-oom-dell01-2026-07-13
supersedes: []
related:
  - 2026-07-05-argocd-crossplane-webhook-blocks-sync
---

# Postmortem: kube-apiserver CRD-cache OOM on the sole control-plane (dell01)

- **Severity/Impact:** Full cluster-API outage. `kubectl` unusable
  (`connection refused` to `api.k8s.lan:6443`); GitOps, dashboards, and all
  cluster operations down. Required two physical power cycles of dell01.
  Running workloads on other nodes kept serving; no data loss (etcd intact).
- **Root cause (one line):** apiserver-crd-cache-oom — a newly added Crossplane
  provider (`provider-gcp-compute`, **192 CRDs**) inflated kube-apiserver's
  watch caches + etcd on the sole **7.6 GiB** control-plane past capacity;
  Talos's OOM controller repeatedly SIGKILLed the kube-apiserver cgroup.

## What Happened

`provider-gcp-compute` (added in commit `9a09f16d` for the assets Cloud-CDN)
installs the entire `compute.gcp.upbound.io` + `compute.gcp.m.upbound.io` CRD
surface — **192 CRDs**, by far the largest group on the cluster. kube-apiserver
builds an in-memory watch cache per served resource, so those CRDs pushed its
RSS to ~4.4–4.7 GiB on a node with only 7.6 GiB total. An unreachable
conversion webhook (`provider-gcp-storage` reaching the kube-proxy Service VIP
from a worker) then drove kube-apiserver into an endless
`storage.gcp.upbound.io Bucket`/`BucketIAMMember` cache-rebuild loop, which
accelerated allocation. Talos's node-protection OOM controller SIGKILLed the
kube-apiserver cgroup (exit 137) over and over — 16 restarts — so the API never
stayed up. The node had also been shedding low-priority pods and its NFS mounts
were timing out, symptoms of the same node-wide memory pressure.

Two independent contributors stacked here: **workload placement** (ordinary
apps tolerated the control-plane taint and ran on dell01) and, dominating,
**apiserver cache size** (the 192-CRD provider). Draining workloads alone did
not fix it — after a clean apiserver restart RSS climbed straight back to
~4.4 GiB from the CRD caches. Only removing the compute provider (and
defragmenting the bloated etcd) restored durable headroom.

## Detection Gap (how we catch it next time)

- **What the user saw first:** `kubectl` → `The connection to the server
  api.k8s.lan:6443 was refused` — i.e. the human was the monitor.
- **How we detect it before the user next time:**
  1. **External symptom (primary):** a gatus check on kube-apiserver
     `/readyz` over Tailscale (`https://100.82.148.37:6443/readyz`). Nothing
     today catches "sole-CP apiserver down while the node still pings" — the
     `Tailnet dell01` ICMP check stays green (node up), and the `ArgoCD`
     live check hits the argocd-server UI, which runs on macarm01 and stayed
     200 throughout. This is independent of the in-cluster metrics stack,
     which was dark during the incident (see `home-systems-k5b`).
  2. **Predictive (lead time):** the existing `NodeMemoryCritical` VMRule
     (`node_memory_MemAvailable < 8%` for 5m → Discord) WOULD have fired well
     before the OOM (available fell 610 MiB → 277 MiB) — but only once the
     VictoriaMetrics ingestion is fixed. This incident is a second witness for
     `home-systems-k5b`, not a new alert.
- **Fix path once detected:** runbook below — identify the CRD/webhook load,
  cordon + drain, remove the offending provider, defrag etcd, restart
  kube-apiserver.

## Mitigation (runbook — how to detect & fix this again)

**Symptoms & triage**
- `kubectl ... 6443: connection refused`, but `talosctl -n <cp>` still works.
- `talosctl -n <cp> dmesg | grep -E 'OOM controller triggered|Sending SIGKILL'`
  shows repeated kills of the kube-apiserver cgroup
  (`/sys/fs/cgroup/kubepods/burstable/pod<uid>`).
- `talosctl -n <cp> get memorystats -o yaml | grep -E 'used:|available:'`
  shows <300 MiB available.
- `talosctl -n <cp> processes | grep kube-apiserver` shows RSS ~4+ GiB.

**Find the load**
- `kubectl get crd -o json | jq -r '.items[].spec.group' | sort | uniq -c | sort -nr | head`
  — a single group with ~100+ CRDs (here `compute.gcp.upbound.io` = 96 ×2
  singular/`.m.` = 192) is the smoking gun.
- kube-apiserver log
  (`talosctl -n <cp> logs -k kube-system/kube-apiserver-<node>:kube-apiserver`)
  full of `conversion webhook ... connect: connection timed out` +
  `cacher ... unexpected ListAndWatch error ... reinitializing` = a webhook
  cache-rebuild loop accelerating the leak.

**Fix (GitOps + minimal live action)**
1. **Cordon + drain** the CP of everything non-essential:
   `kubectl cordon <cp>` then
   `kubectl drain <cp> --ignore-daemonsets --delete-emptydir-data --force`.
   (A single Longhorn CSI provisioner may block on its PDB — delete that one
   pod; peers exist elsewhere.)
2. **Remove the heavy provider** in git (drop `provider-gcp-compute` from
   `k8s/charts/crossplane-providers/values.yaml`; disable its consumer with
   `assetsCdn.enabled: false` in `k8s/applications/crossplane-gcp.yaml`).
   Cloud resources survive because `crossplane-gcp` uses
   `deletionPolicy: Orphan`. Commit + push; if the API is up, delete the
   Provider live to reclaim memory immediately:
   `kubectl delete --raw=/apis/pkg.crossplane.io/v1/providers/provider-gcp-compute`.
   Crossplane garbage-collects the 192 CRDs.
3. **Defrag etcd** — after deleting many CRDs the etcd DB is bloated
   (`talosctl -n <cp> etcd status` showed 1.6 GB DB / 62 MB in use):
   `talosctl -n <cp> etcd defrag` → DB back to ~60 MB.
4. **Restart kube-apiserver to drop the stale cache** — deleting the mirror
   Pod does NOT restart the static container; use the runtime:
   `talosctl -n <cp> restart -k kube-system/kube-apiserver-<node>:kube-apiserver:<id>`
   (`id` from `talosctl -n <cp> containers -k | grep kube-apiserver`).
5. **Verify:** `kubectl get --raw=/readyz` passes; `talosctl get memorystats`
   shows >2 GiB available; `talosctl processes` shows apiserver RSS ~1 GiB and
   holding; `dmesg` shows no new OOM kills.

**Rebooting a wedged CP:** if you must reboot and the graceful sequence hangs
(`talosctl reboot` stalls in `unmountPodMounts` on dead NFS mounts),
`talosctl reboot --mode force` CANNOT take over — the takeover waits on the
stuck sequence's lock, and an uninterruptible NFS unmount never releases it.
`talosctl debug` cannot write `/proc/sysrq-trigger` (SELinux ro procfs). The
only recovery is an **out-of-band / physical power cycle**. dell01 has no
BMC/smart-plug configured, so this needs hands on the box.

## Dead Ends

- **"Another etcd quorum loss"** (like 2026-07-03/04) — wrong. etcd was a
  healthy single member the whole time; the fault was memory, not raft.
- **`kubectl delete pod kube-apiserver-<node>`** — the mirror pod came back
  with the SAME PID and 4.7 GiB RSS; deleting the mirror doesn't restart the
  static container. Had to use `talosctl restart -k`.
- **`talosctl reboot --mode force`** to break the NFS-wedged graceful reboot —
  takeover is cooperative and bounded by lock release; the uninterruptible
  unmount never yielded, so force timed out (confirmed against Talos v1.13
  source).
- **`talosctl debug` → `/proc/sysrq-trigger`** as a software force-reboot —
  denied by SELinux (read-only procfs in the debug domain). No software path;
  physical cycle required.
- **Draining workloads alone** — freed ~600 MiB but a clean apiserver restart
  re-grew RSS to 4.4 GiB from the CRD caches within a minute. The apiserver
  cache, not the co-scheduled pods, was the dominant hog.
- **argocd CLI `--core`** kept failing `configmap "argocd-cm" not found` (core
  mode reads the kube-context namespace, ignores `--app-namespace`); after a
  repo-server restart it still served a stale git revision (known repo-server
  cache race). Fell back to `Application.operation.sync` patches and,
  where GitOps hadn't yet converged, narrow live patches mirroring the commit.
- **WOL packet** during the outage — dell01 was mid-boot/off after the
  physical cycle; the magic packet was inert, the box came back on its own.

## Timeline

### 2026-07-13 (UTC)
- `02:20` dell01 kernel logs NFS servers `10.102.60.110` / `10.104.12.25` not
  responding — first sign of node-wide memory/IO pressure.
- `02:33` Talos OOM controller begins SIGKILLing the kube-apiserver cgroup on
  dell01.
- `03:29` kube-apiserver exits 137 (16th restart), stays CrashLoopBackOff; API
  `connection refused`. User reports "cluster down".
- `03:29` Triage: etcd healthy single-member (not quorum loss); dmesg shows
  repeated OOM kills; apiserver log in a `storage.gcp.upbound.io` conversion-
  webhook cache-rebuild loop; apiserver RSS ~4.7 GiB on 7.6 GiB node.
- `~03:34` `talosctl reboot` issued; stalls in `unmountPodMounts` on the dead
  NFS mounts. `--mode force` cannot take over the stuck sequence.
- `--` Physical power cycle #1. Node returns but with only 277 MiB free and the
  same workloads + provider CRDs restored.
- `--` Cordon dell01; drain non-DaemonSet workloads → ~894 MiB free.
- `--` Commit `edd88ddc`: remove control-plane tolerations from apps
  (camofox, goldilocks, longhorn, metrics-server, vertical-pod-autoscaler,
  obs cloudflared/restreamer, firecrawl, argocd values), move `syscd-app` and
  the pinned GCP storage/compute provider runtimes to pc01. Pushed.
- `--` Live-patch `gcp-storage-pinned` DeploymentRuntimeConfig to pc01 →
  GCP providers recover on pc01 (conversion webhooks healthy).
- `--` Clean kube-apiserver restart still re-grows to ~4.4 GiB (avail
  3.6 GiB → 409 MiB in ~1 min) → proves the CRD cache itself is the hog.
- `--` Identify 192 `compute.gcp.upbound.io` CRDs from `provider-gcp-compute`
  (added `9a09f16d`).
- `--` Commit `b861180d`: remove `provider-gcp-compute`; disable `assetsCdn`
  (`deletionPolicy: Orphan` keeps the live GCP CDN). Pushed.
- `--` API wedges again before cleanup runs → physical power cycle #2.
- `--` On recovery GitOps had already removed the provider; **0 compute CRDs**;
  no compute runtime pod.
- `--` `etcd defrag`: DB 1.6 GB → 60 MB. Restart kube-apiserver → RSS ~1.1 GiB,
  available ~2.2–2.6 GiB, stable, no further OOM kills. All 7 nodes Ready;
  dell01 left cordoned running only static + DaemonSet pods.
