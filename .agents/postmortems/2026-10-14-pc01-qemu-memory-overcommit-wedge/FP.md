# Follow-up Plan: pc01 QEMU memory overcommit wedge

<!-- Approved 2026-10-14 ("all"). Implementation ledger for the sibling PM.md —
     tk issue ids, commits, and links land in place as each item commits. -->

Action markers:
- `[CREATE]` new file/resource/tk issue, with exact path or title
- `[EDIT]`   change to an existing file (path + precise change)
- `[DELETE]` remove an existing alert/file — with justification

## Summary

2026-10-14. The PVE host `pc01` has **zero** monitoring: no
node_exporter, no API probe, no memory-pressure signal, and its VMs can
sit in a start-fail loop for days unnoticed. One new probe closes the
host gap — a Gatus external check for the host's API (needs a tailnet
substrate first, see caveat) — plus one label fix, one routing fix, and
tk issues for the durable work. No new VMRule: k8s-node memory pressure is
already covered by `NodeMemoryCritical` (<8% available, 5m, critical,
→ Discord) in node-resources.yaml, and the wedge itself happens on the
host, before any guest exporter exists to see it. Phase-1 coverage (the
2-day silent start loop → vanished worker) rides the existing
`node-down` alert; a tk issue below verifies it actually fired during
2026-10-12→14.

## Probes

### [EDIT] `/Users/yuri/Workdir/Yuri/nixos/modules/gatus/config.yaml`

New endpoint in group `Tech`, next to the existing tailnet checks:

```yaml
  # Proxmox host pc01 API liveness. The host is NOT a k8s node and has no
  # node_exporter; 401 on an unauthenticated /api2/json/* call is the
  # cheapest possible "pveproxy is answering" signal (the UI root / is
  # 200 for everyone, so the check must hit an API path). ICMP stays green
  # through a TCP-accepts-but-unresponsive wedge (see
  # .agents/postmortems/2026-10-14-pc01-qemu-memory-overcommit-wedge/PM.md),
  # so this must be an HTTPS check, not icmp.
  - name: Proxmox pc01 API
    group: Tech
    url: https://192.168.68.112:8006/api2/json/version
    interval: 10s
    client:
      insecure: true
    conditions:
      - "[STATUS] == 401"
      - "[RESPONSE_TIME] < 2000"
    alerts:
      - type: discord
        failure-threshold: 12
```

Notes:
- No credential, no auth: pveproxy answers 401 unauthenticated, which is
  exactly the signal we want (API up = process scheduling works).
- **Caveat (verified 2026-10-14):** the DO gatus host is NOT on the home
  LAN, and no Tailscale subnet router currently advertises
  `192.168.68.0/24` (checked: routers cover 10.x/172.20.x only; the one
  commented-out LAN-referencing endpoint in config.yaml, bind9 at
  192.168.68.200, is disabled). The gatus check therefore requires a
  substrate first: either (a) a subnet router on the home LAN advertising
  192.168.68.0/24 → then point the check at `https://192.168.68.112:8006`,
  or (b) put the PVE host itself on the tailnet → point it at
  `https://<host-tailnet-ip>:8006`. The check body above is final;
  only the URL's IP changes per substrate chosen. Do not apply the
  endpoint before the substrate exists — it would alert-permanently-fail.

### [EDIT] `/Users/yuri/Workdir/Yuri/home-systems/k8s/charts/support-cluster/templates/monitoring/nic-offload-fix.yaml` — **NO**

Re-evaluated: this file is the off-load fix, not a monitoring template.
Do not touch it. The in-cluster VMRule for memory pressure belongs in the
existing pattern file:

### [EDIT] `k8s/charts/support-cluster/templates/monitoring/node-resources.yaml` — **NO**

Cut from the plan (was: a `NodeMemoryPressureWarning` rule): redundant
with the existing `NodeMemoryCritical` in the same file (<8% available
for 5m = >92% used, critical, → Discord) and outside this incident's
failure mode — the wedge lives on the PVE host, which has no
node_exporter, and the guest is powered off exactly when the wedge
happens, so no in-cluster rule can observe it. The Gatus probe above is
the only new signal this incident justifies.

### [DELETE] Gatus `Tailnet pc01` ICMP endpoint — **NOT APPROVED, KEEP**

Renamed in place instead (see below): the target (100.101.182.40) is the
Talos **guest**, not the host — the label is actively misleading.

### [EDIT] `/Users/yuri/Workdir/Yuri/nixos/modules/gatus/config.yaml` (rename)

```yaml
  - name: Tailnet talos-pc01   # was: Tailnet pc01
```

Same target (`icmp://100.101.182.40`), same alerts; label now matches
the guest node it actually probes.

## tk issues

<!-- Epic labeled pm:<YYYY-MM-DD>-<slug>; children created with --parent inherit it.
     Filter the whole incident: tk list -l pm:2026-10-14-pc01-qemu-memory-overcommit-wedge --all -->

- [CREATE] Epic: `Postmortem: pc01 QEMU memory overcommit wedge` → tk-wtv
  (label `pm:2026-10-14-pc01-qemu-memory-overcommit-wedge`,
  description links PM.md)
  - [CREATE] task: `Expose PVE host pc01 to the tailnet (subnet route for 192.168.68.0/24 or host join)` → tk-ait
    (P1 — prerequisite for the monitoring task below; without it the DO
    gatus host cannot reach 192.168.68.112:8006 and the check would
    permanently fail)
  - [CREATE] task: `PVE host pc01 has no monitoring: Gatus API probe + rename guest ICMP check` → tk-yyk
    (P1 — after the substrate task lands: the two Gatus edits above,
    plus nixos apply; catch phase 2 in ~2 min instead of ~40)
  - [CREATE] task: `Verify node-down alert fired for the vanished pc01 worker (2026-10-12→14)` → tk-lqz (P2 —
    the VM was off for 2 days; node-down.yaml should have paged on
    kube_node_status_condition Ready!=true. Check Discord delivery; if it
    stayed silent (e.g. the node object vanished from kube-state-metrics
    instead of going NotReady), extend the expr with an
    absent()/kube_node_info clause so a missing production worker pages)
  - [CREATE] task: `Harvest pc01 host journal for 2-day start-loop stderr` → tk-nn5 (P2 —
    verify the insta-fail cause recorded in PM.md Open Ends; needs LAN or
    console access)
  - [CREATE] task: `Never size fixed guest RAM > host RAM minus QEMU+PVE overhead (~2.5G)` → tk-ckr (P3 —
    comment-only doc change in `k8s/charts/crossplane-proxmox/values.yaml`
    near `memoryMiB`; prevents re-committing the 2026-09-20 assumption)
  - [CREATE] task (P3): `Make warning-severity alert delivery explicit in the VMAlertmanagerConfig` → tk-fah
    (only severity=critical + environment=production has a matching
    sub-route today; disk.yaml/longhorn-backup.yaml fire severity=warning
    rules whose delivery is not established. Add an explicit
    severity=warning → discord sub-route, then confirm delivery with a
    synthetic warning alert)
  - [CREATE] task (P3): verify each created alert actually fires and routes → tk-pqp
    (Gatus: pause pveproxy or drop 8006 for >2 min; VMRule: mem-stress a
    non-prod node or synth the series, then confirm Discord delivery)

## Alert Cleanup

- [DELETE] none — no noisy/standing alert targets this failure mode
  today; the mislabeled `Tailnet pc01` entry is renamed, not deleted
  (it still probes the guest's tailnet reachability).
- [FIX via tk, P3] warning-severity delivery in the
  VMAlertmanagerConfig — see tk issues; disk.yaml/longhorn-backup.yaml
  warning alerts may be firing into the void today.

## Landed 2026-10-14

- nixos `modules/gatus/config.yaml`: `Proxmox pc01 API` endpoint was staged
  **commented out** pending tk-ait (uncommenting without a route would
  permanently alert-fail); after the substrate was rejected the commented
  block was deleted 2026-10-15 and a one-line "no API probe" note remains.
  Rename `Tailnet talos-pc01` landed.
- Epic tk-wtv + 7 tk issues created (ids inline above); deps wired:
  tk-ait→tk-yyk→tk-pqp, tk-fah→tk-pqp.
- Provider-log timeline pass done 2026-10-14 (negative finding): zero
  `talos-pc01` provider-proxmox log lines before 10-14T12Z — the 2-day
  loop left no trace in Crossplane logs (PM.md Timeline updated).
  values.yaml RAM comment landed with the fix itself (2d266077, line 102)
  — tk-ckr closed.
- Killed by user 2026-10-15: substrate tk-ait, probe tk-yyk, journal
  harvest tk-nn5 — no host-API probe for pc01 by decision.
- tk-lqz closed 2026-10-15 (verified): KubeNodeNotReady fired for pc01
  10-14T13:45→17:50 and delivered (~59 Discord sends, AM counter 64→123)
  — but only after KSM stabilized. During 10-11T10:00→10-14T13:00
  kube-state-metrics (pod 8nw2x, rpi01) restart-looped on liveness
  `livez` timeouts (208 restarts), so node condition series were absent
  and every node alert was blind the whole gap; the watchdog does not
  cover KSM (node-down.yaml comment was wrong — fixed). Fix landed:
  `KubeNodeMetricsAbsent` (absent() for 10m, critical) added to
  node-down.yaml; expr verified against the incident — returns 1 at
  2026-10-13T12:00Z, empty now (no false positive). New follow-up:
  tk-elx (KSM restart-loop root cause).
- Still open: tk-fah (warning route) → tk-pqp (fire + route verify).

## Landed 2026-10-15 (KSM root cause, tk-elx)

Root cause of the 4-day KSM kill-loop (evidence at fix time):
- KSM watched the **full default 28-collector set incl. `secrets`**
  (`--resources=certificatesigningrequests,...,secrets,...` in the live
  Deployment args); over the offsite Tailscale link to the apiserver the
  cache sync/catch-up is slow, and after every restart it re-lists
  everything.
- Pod is BestEffort (no requests/limits) on rpi01, which was at
  **96% allocatable CPU (3786m/3950m)** — longhorn 289m, cilium 217m,
  vmagent 134m; KSM steady-state itself only 32m/35Mi. The post-restart
  catch-up spike starves `/livez` (5s timeout ×3 failures, period 10s,
  initialDelay 5s → kill at ~35-38s), matching the observed ~38s
  pod lifetime ×214 restarts since 10-11T09:12Z. No fatal error in logs —
  clean startup then probe kill (exit 2).
- Fix `96922cb1` (victoria-metrics-k8s-stack app values,
  `kube-state-metrics:` block): collectors trimmed to the 4 families the
  repo's alerts/dashboards actually use (`daemonsets,nodes,pods,
  statefulsets` — grep'd `kube_*` usage repo-wide), requests
  100m/64Mi + limit 256Mi (Burstable, no CPU limit), liveness
  timeoutSeconds 5→15, failureThreshold 3→5 (window 5+5×~15s ≈ 80s).
  Helm-rendered and verified before push.
- Verified live 2026-10-15 (~17:45Z), tk-elx closed: vmks sync Succeeded
  on retry #4 (the "HEAD,HEAD" poison was my bad force-sync patch — see
  below); Deployment now `--resources=daemonsets,nodes,pods,statefulsets`,
  requests 100m/64Mi + 256Mi limit, probe 15s×5. New pod
  `vmks-kube-state-metrics-956cffd66-9m257` Running **0 restarts** (old
  pod died every ~38s); `kube_node_status_condition{condition="Ready"}`
  samples 30s old — 3-day gap closed; `KubeNodeMetricsAbsent` correctly
  silent with metrics flowing.
- Alert fix LIVE 2026-10-15 ~16:40Z: support-cluster sync applied
  node-down-rules with `KubeNodeMetricsAbsent` (VMRule gen=3 observed=3,
  `ConfigParsedAndApplied`, updateStatus operational — vmalert has it
  loaded).

## Systemic finding: openapi schema fetch is the ArgoCD wedge mechanism

During burn windows the apiserver's `/openapi/v2` body read degrades
catastrophically: measured 2026-10-15 17:4xZ from tp4 pod (LAN, healthy
node): **200 OK, 12MB, 66.3s**; same hour from the workstation: 22MB in
0.09s. Every ArgoCD apply validates manifests against this schema, so
during burn windows ALL syncs fail with `error validating data: failed
to download openapi: net/http: request cancel` / `context deadline
exceeded while reading body` — observed on the vmks op (82 resources
SyncFailed at 17:2xZ) and the support-cluster op (17 VMRules stuck
OutOfSync). Two argocd-application-controller restarts (macarm01→tp4,
18:00+ cache rebuild) didn't fix it — the path is the problem, not the
controller. Retry cycles DO eventually get through (vmks Succeeded on
attempt #4), so syncs self-heal when the burn dips. Noted on tk-qpb;
the fix (bigger control-plane VM / off-host CP) addresses this too.
Ops lesson: ArgoCD `operationState` in this cluster's Application CRD
has no `terminated` field — finalize wedged ops with a status patch
setting `phase: Failed` + `finishedAt`, then let the automated policy
resync; and never force-sync a multi-source app with
`revisions:["HEAD",...]` (pins helm-repo sources to an invalid semver).
