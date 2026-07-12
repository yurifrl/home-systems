---
date: 2026-07-11
status: draft
incident_status: active
sessions:
  - 019f538f-6c29-7410-9dfb-b151ecb16eb1
components:
  - victoria-metrics
  - vmsingle
  - longhorn
  - monitoring
symptoms:
  - vmsingle rejecting all /api/v1/write with "storage is in read-only mode"
  - no metrics ingestion; vmalert/vmagent remote_write failing
  - disk-full alert (KubePersistentVolumeCriticalFull) never fired
failure_mode: storage-exhaustion-readonly-self-blinded-monitoring
affected_urls:
  - https://prometheus.syscd.live (query API; historical data only, no fresh)
beads: [home-systems-el5]
memories: []
supersedes: []
related: []
---

# Postmortem: vmsingle storage exhausted → read-only, ingestion dead, and the disk alert was blind to it

- **Severity/Impact:** cluster-wide metrics **ingestion fully stopped**.
  `vmsingle-vmks` filled its 3Gi PVC (97%, ~83M free) under 30d retention and
  flipped VictoriaMetrics into read-only mode, rejecting every remote_write
  from vmagent/vmalert. 30d of historical data is still queryable but no new
  samples land. Detected by the **user**, not by monitoring.
- **Root cause (one line):** storage-exhaustion-readonly-self-blinded-monitoring
  — the metrics store ran out of disk and went read-only; the alert that should
  have caught it (`KubePersistentVolumeCriticalFull`) is evaluated by vmalert
  against vmsingle, which stopped ingesting its own PVC metric, so the alert had
  no fresh data to fire on. No external dead-man's-switch existed.

## What Happened

`vmsingle-vmks` is the single-node VictoriaMetrics TSDB, backed by a **3Gi**
single-replica Longhorn PVC with **30d** retention. Over ~35 days the data grew
to fill the volume. When free space fell below
`-storage.minFreeDiskSpaceBytes`, VictoriaMetrics entered **read-only mode** and
began rejecting all writes:

```
cannot flush metric bufs: cannot store metrics: the storage is in read-only
mode; check -storage.minFreeDiskSpaceBytes command-line flag value
```

Ingestion stopped completely. Confirmed live: `up` and every other metric
return empty as an instant query at `now` (zero fresh samples), while 30d of
history is still present.

## Detection Gap (why the user saw it first)

- **What the user saw first:** they noticed VictoriaMetrics was broken; no
  alert had fired.
- **Why the existing alert was blind:** `KubePersistentVolumeCriticalFull`
  (`k8s/charts/support-cluster/templates/monitoring/disk.yaml`) fires at <4%
  free with `severity: critical` + `environment: production` → Discord — the
  vmsingle PVC is at ~2.7% free, well past threshold, and it is **not firing**.
  The rule is evaluated by **vmalert querying vmsingle** for a *fresh*
  `kubelet_volume_stats_available_bytes` sample. Once vmsingle went read-only it
  stopped ingesting **all** metrics, including its own PVC stats, so the
  instant-query expression returns no data and cannot evaluate. **The metrics
  stack is its own alert data source; when its storage broke it went blind to
  the exact condition that broke it.** vmalert never told Alertmanager
  anything, so nothing routed.
- **Second gap — nothing watches the watcher:** no Watchdog/dead-man's-switch
  VMRule and (before this incident) no external gatus check on the monitoring
  stack. A self-hosted stack cannot page on its own storage exhaustion from the
  inside; the silence itself was undetectable.
- **How we detect it before the user next time:** an **external** gatus check
  (runs off-cluster on the tailnet) that queries the VictoriaMetrics API for
  data freshness — `count(up)` returns a non-empty result only while fresh
  samples land; a read-only / dead vmsingle returns `"result":[]`. This catches
  both "stack totally down" (non-200) and "up but not ingesting" (empty result),
  neither of which the internal alert or the existing `/-/healthy` check can
  see. See Follow-ups.
- **Fix path once detected:** expand the vmsingle PVC (durable) or drop
  retention (frees space, loses history). See Mitigation runbook.

## Mitigation (runbook — how to detect & fix this again)

1. **Confirm read-only:** `kubectl -n monitoring logs deploy/vmsingle-vmks
   --tail=20` → look for `the storage is in read-only mode`. Check free space:
   `kubectl -n monitoring exec deploy/vmsingle-vmks -- df -h
   /victoria-metrics-data` (near 100% used).
2. **Durable fix — expand the PVC (GitOps):** bump
   `vmsingle.spec.storage.resources.requests.storage` in
   `k8s/applications/victoria-metrics-k8s-stack.yaml`, commit + push, let ArgoCD
   sync (parent app-of-apps `applications` → child `victoria-metrics-k8s-stack`
   → operator patches the VMSingle CR). Longhorn `allowVolumeExpansion=true`.
3. **Watch the expansion, do NOT restart the pod prematurely (see Dead Ends):**
   Longhorn expands the block device; the filesystem grows on a clean mount.
   `kubectl -n monitoring get pvc vmsingle-vmks -w` until
   `status.capacity == 10Gi`. VictoriaMetrics exits read-only automatically once
   free space exceeds the threshold — no manual restart needed.
4. **Keep vmsingle on a core LAN node:** the RWO Longhorn volume's engine +
   replica live on core nodes (tp1/tp4). If the pod reschedules to a cross-site
   node (macarm01/rpi01) it cannot attach/resize the volume. vmsingle is now
   pinned to tp4 (`nodeSelector`), matching its stable 35-day placement.
5. **Quick relief if expansion is blocked:** lower `retentionPeriod` so VM
   deletes old data and frees space (sacrifices history, no Longhorn expansion
   needed).

## Follow-ups (epic home-systems-el5)

See `FP.md` for the live ledger.

- **DONE — PVC 3Gi→10Gi** (`victoria-metrics-k8s-stack.yaml`, commit `21dfb3b1`):
  durable capacity fix, synced by ArgoCD.
- **DONE — pin vmsingle to tp4** (commit `fd09602d`): keeps the RWO PVC off
  cross-site nodes.
- **DONE (deploy held) — gatus dead-man's-switch** (`nixos` commit `a15f7cc`,
  `home-systems-el5.3`): external `count(up)` freshness check. Deploy held until
  ingestion is restored so it doesn't page for the in-progress outage.
- **BLOCKED — restore ingestion** (`home-systems-el5.1`, P1): the Longhorn
  expansion is stuck on a stale replica artifact (see Dead Ends); needs a
  user decision (recreate volume vs instance-manager restart).
- **OPEN — verify the alert fires + routes** (`home-systems-el5.2`, P3).

## Dead Ends

- **Restarting the vmsingle pod to "trigger" the filesystem resize backfired.**
  The pod rescheduled to **macarm01** (a cross-site node — vmsingle had no node
  pinning) and hit a Multi-Attach / cross-site attach timeout on the RWO
  Longhorn volume (engine + replica on core nodes). This created the pin-to-tp4
  follow-up. Lesson: expand in place and let Longhorn/kubelet grow the FS on the
  existing mount; only restart once, and only onto the node where the volume is
  attached.
- **The real blocker is a stuck Longhorn expansion, not VictoriaMetrics.** The
  Longhorn volume shows `size=10Gi` but the engine block device is stuck at 3Gi
  with `expansionRequired=true`. longhorn-manager logs the true cause on every
  ~5min retry: `FailedExpansion ... failed to expand replica <tp1-replica>: ...
  failed to create new disk expand-10737418240: volume-head-001.img already
  exists` — a leftover expansion artifact on the single replica (tp1) from the
  first failed attempt. Scaling to 0 does **not** detach (Longhorn keeps it
  attached to retry online expansion), so it never gets a clean offline cycle,
  and the leftover file blocks every retry. This is what keeps vmsingle
  read-only despite the config being correct.
- **`prometheus.syscd.tech/-/healthy` is a false comfort.** It returns
  `Prometheus Server is Healthy.` (200) even in read-only mode — it proves the
  process is up, not that ingestion works. That's why the freshness check
  (`count(up)`) is needed alongside it.

## Timeline

### 2026-07-11 / 2026-07-12 (UTC)
- (prior ~35d) vmsingle 3Gi PVC gradually fills under 30d retention.
- `~2026-07-11` vmsingle free space crosses `minFreeDiskSpaceBytes` → read-only
  mode; ingestion stops. The internal disk alert cannot fire (no fresh
  `kubelet_volume_stats` samples to evaluate).
- `2026-07-12 ~16:40` User reports "victoria metrics is broken."
- `16:44` Root cause found: `df` shows `/victoria-metrics-data` 2.9G, 97% used,
  83.6M free; logs show read-only mode.
- `16:47` PVC `3Gi→10Gi` committed + pushed (`21dfb3b1`); ArgoCD app-of-apps
  chain syncs the VMSingle CR to 10Gi.
- `~16:54` Longhorn volume expands to 10Gi at the volume level, but the pod
  restart reschedules vmsingle to macarm01 → Multi-Attach / cross-site attach
  failure; filesystem never resizes.
- `~17:05` vmsingle pinned to tp4 (`fd09602d`); pod returns to tp4, attaches,
  runs — but still read-only (block device still 3Gi).
- `17:15–17:31` longhorn-manager logs repeated `FailedExpansion` on the tp1
  replica: `volume-head-001.img already exists`. Scale-to-0 does not detach;
  offline expansion never runs. Stuck.
- `~17:40` gatus external freshness dead-man's-switch added
  (`nixos` `a15f7cc`), deploy held. Postmortem written; ingestion still down
  pending the Longhorn-recovery decision.
