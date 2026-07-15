# Follow-up Plan: kube-apiserver CRD-cache OOM on the sole control-plane (dell01)

Approved 2026-07-13. Epic: **home-systems-0r5** (`pm:2026-07-13-apiserver-crd-cache-oom`).
Filter the incident: `bd list -l pm:2026-07-13-apiserver-crd-cache-oom --all`.

Action markers: `[CREATE]` `[EDIT]` `[DELETE]` `[SKIP]`. Executed items gain `→ done:`.

## Durable Fix (chosen)
- [CREATE] bead — migrate the sole control plane to **macintel01** (more RAM +
  battery/UPS). Bigger RAM removes the apiserver-CRD-cache OOM headroom problem;
  the battery removes the no-BMC "needs a physical power cycle" problem that
  forced two manual reboots this incident. Caveat recorded in the bead:
  macintel01 is a cross-site node (home-systems-aqt fault line) — validate
  apiserver/etcd/cilium reachability before cutting over the SOLE CP.
  → done: **home-systems-0r5.3** (P1)

## Alerts
- [CREATE] gatus external kube-apiserver liveness check —
  `/Users/yuri/Workdir/Yuri/nixos/modules/gatus/config.yaml`. Current state:
  gatus had `Tailnet <node>` ICMP checks (node up, not apiserver) and an
  `ArgoCD` UI check (server on macarm01, stayed 200) — nothing caught
  "sole-CP apiserver down while node pings". Verdict: **alert (gatus)** —
  external, independent of the dark in-cluster VM metrics stack.
  `url: https://100.82.148.37:6443/readyz`, `client.insecure: true`,
  `[STATUS] == 401` (anonymous-auth=false → 401 = up; refused/timeout = down),
  `failure-threshold: 12`.
  → done: edited in git; deploy pending (nixos rebuild on the gatus host).
  Tracked by **home-systems-0r5.2**; repoint IP on CP migration.
- [CREATE] (P3) verify the gatus apiserver check actually fires + routes to
  Discord. → done: **home-systems-0r5.4**
- [SKIP] new memory VMRule — `NodeMemoryCritical` (<8% avail 5m → Discord)
  already exists and would have fired (avail hit 277 MiB); real blocker is VM
  ingestion being dark. Linked, not duplicated: **home-systems-k5b**.
- [SKIP] apiserver-RSS / CRD-count cause alert — diagnostic; and metrics are
  dark. Dashboard/log-only, nothing created.

## Dashboards / Panels
- [SKIP] no apiserver dashboard added while the VM metrics stack is dark
  (home-systems-k5b); revisit after ingestion is restored.

## Gatus Entries
- Covered above (the apiserver liveness check is the only new gatus entry).

## Beads
- [CREATE] Epic `Postmortem: kube-apiserver CRD-cache OOM on sole control-plane (2026-07-13)`
  → done: **home-systems-0r5** (label `pm:2026-07-13-apiserver-crd-cache-oom`)
  - [CREATE] migrate sole CP to macintel01 → done: **home-systems-0r5.3** (P1)
  - [CREATE] remove dead NFS mounts on dell01 (10.102.60.110, 10.104.12.25)
    that hang graceful reboot → done: **home-systems-0r5.1** (P2)
  - [CREATE] gatus apiserver liveness check → done: **home-systems-0r5.2** (P2)
  - [CREATE] (P3) verify gatus check fires + routes → done: **home-systems-0r5.4**
- [SKIP] duplicate of `home-systems-k5b` (VM metrics dark) — linked from the
  epic instead.

## Memory
- [CREATE] `bd remember` root-cause + runbook →
  done: key **apiserver-crd-cache-oom-dell01-2026-07-13**

## Alert Cleanup
- [SKIP] no noisy/standing alert identified to remove for this incident.
</content>
