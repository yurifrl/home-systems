# Follow-up Plan: pc01 QEMU memory overcommit wedge

<!-- Proposed 2026-10-14; awaiting user approval. Once approved: declares exactly what
     gets created — nothing else — and is updated in place with the resulting
     bead ids, commits, and links as each item lands. This file persists as the
     implementation ledger for the sibling PM.md. -->

Action markers:
- `[CREATE]` new file/resource/bead, with exact path or title
- `[EDIT]`   change to an existing file (path + precise change)
- `[DELETE]` remove an existing alert/file — with justification

## Summary

2026-10-14. The PVE host `pc01` has **zero** monitoring: no
node_exporter, no API probe, no memory-pressure signal, and its VMs can
sit in a start-fail loop for days unnoticed. Two probes close the gap
using existing patterns — a Gatus external check for the host's API
(needs a tailnet substrate first, see caveat) and an in-cluster VMRule
for guest-visible memory pressure — plus beads for the durable work.
Alert hygiene rule from 2026-08-16 applies: production-critical only or
a dashboard panel — nothing intermediate.

## Probes

### [EDIT] `/Users/yuri/Workdir/Yuri/nixos/modules/gatus/config.yaml`

New endpoint in group `Tech`, next to the existing tailnet checks:

```yaml
  # Proxmox host pc01 API liveness. The host is NOT a k8s node and has no
  # node_exporter; 401 on the unauthenticated ticket endpoint is the
  # cheapest possible "pveproxy is answering" signal. ICMP stays green
  # through a TCP-accepts-but-unresponsive wedge (see
  # .agents/postmortems/2026-10-14-pc01-qemu-memory-overcommit-wedge/PM.md),
  # so this must be an HTTPS check, not icmp.
  - name: Proxmox pc01 API
    group: Tech
    url: https://192.168.68.112:8006
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

### [EDIT] `k8s/charts/support-cluster/templates/monitoring/node-resources.yaml`

Append a VMRule warning-level alert for the guest's view of the host:
not actionable as production-critical per the 2026-08-16 hygiene rule
(windows/workstation VMs are not production), so severity stays
`warning` and routes to Discord per the existing VMRule convention:

```yaml
        - alert: NodeMemoryPressureWarning
          expr: |
            (
              1 - (
                avg_over_time(node_memory_MemAvailable_bytes[5m]) /
                node_memory_MemTotal_bytes
              )
            ) * 100 > 90
          for: 10m
          labels:
            severity: warning
            environment: production
            service: node-memory
          annotations:
            summary: "Node memory pressure >90% used for 10m"
            description: "Node {{ `{{ $labels.instance }}` }} is above 90% memory pressure ({{ `{{ printf \"%.1f\" $value }}` }}% used) for 10m; QEMU-style overcommit wedges live here."
```

This watches every node_exporter-scraped node (dell01, tp1, tp4, …) and
would have caught **phase 2** on any Linux node with 90%+ pressure for
10m. It does **not** cover the PVE host (no exporter) — that is the
Gatus probe's job above. (The VM guest itself runs the same exporter
family via Talos, so once the guest boots it also reports; the wedge
happens *before* the guest boots, hence the Gatus probe on the host API.)

### [DELETE] Gatus `Tailnet pc01` ICMP endpoint — **NOT APPROVED, KEEP**

Renamed in place instead (see below): the target (100.101.182.40) is the
Talos **guest**, not the host — the label is actively misleading.

### [EDIT] `/Users/yuri/Workdir/Yuri/nixos/modules/gatus/config.yaml` (rename)

```yaml
  - name: Tailnet talos-pc01   # was: Tailnet pc01
```

Same target (`icmp://100.101.182.40`), same alerts; label now matches
the guest node it actually probes.

## Beads

<!-- Epic labeled pm:<YYYY-MM-DD>-<slug>; children created with --parent inherit it.
     Filter the whole incident: bd list -l pm:2026-10-14-pc01-qemu-memory-overcommit-wedge --all -->

- [CREATE] Epic: `Postmortem: pc01 QEMU memory overcommit wedge`
  (label `pm:2026-10-14-pc01-qemu-memory-overcommit-wedge`,
  description links PM.md)
  - [CREATE] task: `Expose PVE host pc01 to the tailnet (subnet route for 192.168.68.0/24 or host join)`
    (P1 — prerequisite for the monitoring task below; without it the DO
    gatus host cannot reach 192.168.68.112:8006 and the check would
    permanently fail)
  - [CREATE] task: `PVE host pc01 has no monitoring: Gatus API probe + rename guest ICMP check`
    (P1 — after the substrate task lands: the two Gatus edits above,
    plus nixos apply; catch phase 2 in ~2 min instead of ~40)
  - [CREATE] task: `Harvest pc01 host journal for 2-day start-loop stderr` (P2 —
    verify the insta-fail cause recorded in PM.md Open Ends; needs LAN or
    console access)
  - [CREATE] task: `Never size fixed guest RAM > host RAM minus QEMU+PVE overhead (~2.5G)` (P3 —
    comment-only doc change in `k8s/charts/crossplane-proxmox/values.yaml`
    near `memoryMiB`; prevents re-committing the 2026-09-20 assumption)
  - [CREATE] task (P3): verify each created alert actually fires and routes
    (Gatus: pause pveproxy or drop 8006 for >2 min; VMRule: mem-stress a
    non-prod node or synth the series, then confirm Discord delivery)

## Alert Cleanup

- [DELETE] none — no noisy/standing alert targets this failure mode
  today; the mislabeled `Tailnet pc01` entry is renamed, not deleted
  (it still probes the guest's tailnet reachability).
