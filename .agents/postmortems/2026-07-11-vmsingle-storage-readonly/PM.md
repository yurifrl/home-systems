---
date: 2026-07-11
status: reopened
incident_status: mitigated
sessions:
  - 019f538f-6c29-7410-9dfb-b151ecb16eb1
  - 019f573d-9209-70e1-b670-a8c7df841f14
  - 01a006c5-f21e-7533-b79c-6611dbc3cd85
components:
  - victoria-metrics
  - vmsingle
  - longhorn
  - monitoring
symptoms:
  - vmsingle rejecting all /api/v1/write with "storage is in read-only mode"
  - no metrics ingestion; vmalert/vmagent remote_write failing
  - disk-full alert (KubePersistentVolumeCriticalFull) never fired
  - vmsingle 10Gi PVC at 99% used / 94.6M free, read-only again ~5 weeks after the 3Gi->10Gi expansion
  - victoriametrics-watchdog CronJob failing every run (DNS resolution flake + "awk: cmd. line:1: Unexpected token")
failure_mode: storage-exhaustion-readonly-self-blinded-monitoring
affected_urls:
  - https://prometheus.syscd.live (query API; historical data only, no fresh)
beads: [home-systems-el5]
memories: [vmsingle-storage-readonly-self-blinded-2026-07-11]
supersedes: []
related:
  - 2026-07-05-cloudflare-tunnel-quic-edge-unreachable
  - 2026-08-16-hermes-dashboard-affinity-and-gh-multi-account
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
- **Second gap — nothing watched the watcher at incident time:** there was no
  external gatus check and no alert path independent of vmalert querying
  vmsingle. The external gatus freshness check now covers total monitoring-path
  failure. A separate in-cluster CronJob was later added that reads vmsingle and
  vmagent process metrics directly and posts to Alertmanager, bypassing the
  failed TSDB query path; it is supplementary because it still depends on the
  cluster and Alertmanager.
- **How we detect it before the user next time:** the primary signal is an
  **external** gatus check (runs off-cluster on the tailnet) that queries the
  VictoriaMetrics API for data freshness — `count(up)` returns a non-empty
  result only while fresh samples land; a read-only / dead vmsingle returns
  `"result":[]`. The supplementary direct-to-Alertmanager CronJob checks
  read-only state, free space, endpoint availability, and vmagent backlog
  without querying stored metrics. See Follow-ups.
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
- **DONE — gatus dead-man's-switch** (`nixos` commit `a15f7cc`,
  `home-systems-el5.3`): external `count(up)` freshness check deployed and
  green after ingestion was restored.
- **DONE — restore ingestion** (`home-systems-el5.1`): the stuck Longhorn
  expansion could not be unstuck in place, so the volume was recreated fresh at
  10Gi (single-replica metrics data is expendable). Ingestion resumed; vmagent
  drained its ~740MB outage backlog.
- **DONE — size-based retention guard** — REVERTED. `-retention.maxDiskSpaceUsageBytes`
  does **not** exist in single-node VictoriaMetrics (v1.147); it is a
  cluster-only `vmstorage` flag. Adding it crashlooped vmsingle
  (`flag provided but not defined`); reverted in `e6a77375`. Single-node VM has
  **no size-based retention** — only `-retentionPeriod` (time). Recurrence
  protection is therefore: the 10Gi PVC (~3.3× the 30d footprint — the old 3Gi
  held ~30d, so 30d now uses ~3Gi of 10Gi) **plus** the external gatus
  dead-man's-switch, which catches read-only regardless.
- **DONE — deploy gatus check** (`home-systems-el5.3`): pushed nixos `a15f7cc`
  + ran the `deploy.yml` GitHub Action (`workflow_dispatch target=gatus`) —
  deployed successfully; check is live and green.
- **OPEN — verify gatus fires on a real ingestion loss** (`home-systems-el5.2`, P3).
- **DONE — stop false Cloudflare outage pages on missing monitoring data:**
  removed `absent(cloudflared_tunnel_ha_connections)` from
  `CloudflareTunnelDown` (`fd79c7a6`). Missing telemetry no longer claims the
  tunnel is down.
- **IMPLEMENTED, NOT STABLE — direct-to-Alertmanager watchdog:** CronJob added
  in `victoriametrics-watchdog.yaml` (`fd79c7a6`, parser fixes `901b8e5f` and
  `c263b635`). It succeeded once, then later runs failed on another BusyBox awk
  expression. Stabilization and routing verification are tracked by
  `home-systems-el5.4`.

### Recurrence (2026-08-16) — per-bead effectiveness verdict

vmsingle filled again and went read-only a second time, ~5 weeks after the
3Gi→10Gi expansion, discovered as a side-investigation of an unrelated hermes
incident (`2026-08-16-hermes-dashboard-affinity-and-gh-multi-account`) while
checking why `KubePodNotReady`/`KubePodCrashLooping` hadn't fired. Per prior
follow-up:

- **`home-systems-el5.1` (restore ingestion, DONE) — closed-but-ineffective
  long-term, effective short-term.** The fresh 10Gi volume did restore
  ingestion in July and stayed healthy for ~5 weeks; it was never meant to be
  a permanent size fix — the headroom simply ran out again under real growth
  (retention, cardinality, or both). Not a wasted fix, just not sized for
  long-run growth.
- **`home-systems-el5.3` (gatus dead-man's-switch, DONE) — status unverified
  this incident.** Could not confirm live status from this session (the gatus
  host was not reachable from the exec context used); this is exactly what
  `el5.2` was supposed to close out and never did.
- **`home-systems-el5.2` (verify gatus fires on real ingestion loss, OPEN,
  P3) — never-done, and this recurrence is precisely the real ingestion loss
  it should have been verified against.** This is the sharpest finding: the
  one open verification task from the prior incident is exactly the test this
  recurrence provides, and nobody was watching to see if gatus paged.
- **`home-systems-el5.4` (stabilize + verify watchdog, OPEN, P2) —
  never-done.** Checked live: `victoriametrics-watchdog` CronJob (runs every
  1m) is still failing most runs — one clean `Completed` job followed by a
  string of `Failed` jobs, with two distinct causes observed live: (a)
  `curl: (6) Could not resolve host: vmsingle-vmks` (a DNS resolution flake,
  transient — a manual `nslookup vmsingle-vmks` from a throwaway pod resolved
  fine on retry) and (b) the same unresolved `awk: cmd. line:1: Unexpected
  token` from the original incident. The watchdog has never been stabilized in
  the 5 weeks since it was flagged, so it provided **zero** detection value for
  this recurrence.

**This recurrence needs something stronger than "verify the two follow-ups
that were never verified."** Both `el5.2` and `el5.4` are now proven,
live-observed gaps, not theoretical ones — see `FP.md` for the escalated,
more assertive follow-up plan (larger PVC + retention-based headroom margin
instead of a fixed multiplier, and dropping the BusyBox-incompatible watchdog
for a runtime that actually supports the script instead of re-patching awk a
third time).

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
- **`-retention.maxDiskSpaceUsageBytes` does not exist in single-node VM.** I
  added it expecting size-based auto-rotation; vmsingle crashlooped with
  `flag provided but not defined`. That flag is cluster-only (`vmstorage`).
  Single-node VictoriaMetrics only has time-based retention. Lesson: there is no
  in-VM "rotate before disk-full" knob for single-node — size headroom + an
  external freshness alert is the protection.
- **vmagent's remote-write worker got stuck across the vmsingle pod churn.**
  After vmsingle cycled (volume recreate + the bad-flag crashloop), vmagent
  stopped pushing (2XX counter frozen, on-disk queue growing) even though
  vmsingle was healthy and its Service endpoint was correct. A vmagent pod
  restart cleared it and live ingestion resumed immediately (`count(up)` > 0).
- **The repeating `CloudflareTunnelDown` notification was not tunnel flapping.**
  cloudflared exposed 4 healthy connections and had only one 14-second
  single-connection reconnect. The alert's `absent(...)` branch interpreted
  missing TSDB data as tunnel failure, then Alertmanager resent one continuously
  firing alert every 1h plus its 5m group interval.
- **Investigation initially followed the Cloudflare/Longhorn symptoms instead
  of the alert semantics.** The decisive trace was metric producer → vmagent
  scrape → rejected vmsingle write → empty query → `absent()` true.
- **The watchdog shell passed Helm rendering but failed in BusyBox awk.** The
  first unparenthesized ternary was fixed in `c263b635`; a later run exposed a
  second `awk: cmd. line:1: Unexpected token`. Render-only tests did not execute
  the container's awk implementation, so `home-systems-el5.4` requires fixture
  execution against BusyBox plus live repeated-job verification.

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
  (`nixos` `a15f7cc`), deploy held.
- `~18:05` Longhorn expansion confirmed unrecoverable in place (leftover
  replica artifact). Chose to recreate: scaled vmsingle to 0, deleted the PVC
  (reclaim=Delete → old Longhorn volume gone), operator provisioned a fresh 10Gi
  PVC, pod returned to tp4. Read-only cleared; `status/tsdb` climbed from 0 to
  232k+ series as vmagent replayed its ~740MB backlog.
- `~18:20` Added `-retention.maxDiskSpaceUsageBytes=8GB` — WRONG: not a
  single-node flag; vmsingle crashlooped. Reverted (`e6a77375`); vmsingle back
  up clean. Pushed nixos `a15f7cc` + ran the deploy GitHub Action → gatus check
  deployed. Restarted a stuck vmagent → live ingestion confirmed
  (`count(up)=124`, read-only cleared, disk 10Gi). Incident resolved.

### 2026-07-12 — alert-semantics follow-up (UTC)
- `~16:45` User reports repeated `CloudflareTunnelDown` Discord notifications.
- `~16:50` cloudflared confirmed healthy with 4 connections; pod stable. vmalert
  showed one alert continuously firing since July 9 while vmsingle returned no
  current `cloudflared_tunnel_ha_connections` series.
- `~16:55` Root cause traced to the rule's `absent(...)` branch. Alertmanager's
  1h repeat plus 5m group interval explained the roughly 65-minute cadence.
- `19:35` ArgoCD synced the rule without `absent(...)` and created the approved
  direct-to-Alertmanager watchdog CronJob.
- `19:37` Initial watchdog jobs failed on BusyBox awk ternary parsing.
- `19:56` One watchdog job completed after parser fixes; later jobs regressed
  with another BusyBox awk `Unexpected token` error.
- `20:16` Opened `home-systems-el5.4` to stabilize the watchdog and verify its
  warning, critical, resolved, and Discord-routing behavior.

### 2026-08-16 — recurrence (UTC)
- `~08:1x` While investigating why hermes alerts were dark during an unrelated
  incident (`2026-08-16-hermes-dashboard-affinity-and-gh-multi-account`),
  queried `vmsingle-vmks` directly: `api/v1/status/tsdb` returned
  `totalSeries: 0`, `api/v1/label/job/values` returned an empty list — no
  ingestion at all, not just a hermes-specific gap.
- `~08:2x` `kubectl logs vmsingle-vmks-...` showed the same read-only rejection
  message as the original incident: `cannot store metrics: the storage is in
  read-only mode`. `df -h` inside the pod confirmed `/victoria-metrics-data`
  9.7G / 9.6G used / 94.6M free / 99% — the same 10Gi PVC from the July fix,
  full again.
- `~08:2x` Checked `vmalert` rule state for `KubePodNotReady`/`KubePodCrashLooping`/
  `HermesDown`: all `inactive`/`ok` — confirmed dark for lack of data, not
  because thresholds weren't crossed (`kube_pod_container_status_restarts_total{namespace="hermes"}`
  returns zero series).
- `~08:3x` Checked the `victoriametrics-watchdog` CronJob (the direct-to-
  Alertmanager bypass from `home-systems-el5.4`, left unstabilized in July):
  live job list showed 1 `Completed` followed by repeated `Failed` runs.
  `kubectl logs` on the failed jobs showed two causes: `curl: (6) Could not
  resolve host: vmsingle-vmks` (transient — manual `nslookup` from a scratch
  pod resolved cleanly moments later) and the unresolved `awk: cmd. line:1:
  Unexpected token` first seen in July. `el5.4` was never actually stabilized
  in the 5 weeks since.
- Could not verify the external gatus `VictoriaMetrics Ingestion` check's live
  status from this session (host unreachable from the exec context used) —
  `el5.2` (verify gatus fires on a real loss) remains genuinely unverified,
  and this recurrence is the real-loss event it should have been tested
  against.
- Reopened this postmortem rather than filing a new one (same component +
  same failure_mode = recurrence per the postmortem skill). See the
  Recurrence section under Follow-ups for the per-bead verdict and `FP.md`
  for the escalated plan.
