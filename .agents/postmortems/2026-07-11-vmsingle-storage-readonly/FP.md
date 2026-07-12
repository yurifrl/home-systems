# Follow-up Plan — vmsingle storage exhausted → read-only

Epic: **home-systems-el5** (label `pm:2026-07-11-vmsingle-storage-readonly`)
Postmortem: `PM.md` (same directory)

| # | Action | Marker | Location | Result |
|---|--------|--------|----------|--------|
| 1 | Expand vmsingle PVC `3Gi → 10Gi` | `[EDIT]` | `k8s/applications/victoria-metrics-k8s-stack.yaml` (vmsingle.spec.storage) | DONE — commit `21dfb3b1`, ArgoCD synced VMSingle CR to 10Gi |
| 2 | Pin vmsingle to core node `tp4` | `[EDIT]` | same file (vmsingle.spec.nodeSelector) | DONE — commit `fd09602d`; keeps RWO PVC off cross-site nodes |
| 3 | External freshness dead-man's-switch (`count(up)` empty ⇒ ingestion dead) | `[EDIT]` | `nixos:modules/gatus/config.yaml` | DONE (config) — commit `a15f7cc`; DEPLOY HELD (`el5.3`) until ingestion restored + Tailnet-block bundling reviewed |
| 4 | Restore ingestion — resolve stuck Longhorn expansion (`volume-head-001.img already exists` on tp1 replica) | `[TODO]` | Longhorn (tp1 replica) | DONE (`el5.1`) — unstick failed; recreated volume fresh at 10Gi (scale 0 → delete PVC → operator reprovisions). Ingestion restored, ~740MB backlog drained |
| 4b | Size-based retention guard `maxDiskSpaceUsageBytes` | `[EDIT]` | `victoria-metrics-k8s-stack.yaml` | REVERTED (`e6a77375`) — flag is cluster-only, not single-node; crashlooped vmsingle. No size-based retention exists single-node. Recurrence protection = 10Gi headroom (~3.3× 30d) + gatus dead-man's-switch |
| 3b | Deploy gatus check | `[RUN]` | nixos push + `deploy.yml` action | DONE (`el5.3`) — pushed `a15f7cc`, ran `workflow_dispatch target=gatus`, deployed OK, check live+green |
| 5 | Verify gatus check fires + routes to Discord | `[TODO]` | gatus / Discord | OPEN (`el5.2`, P3) — after deploy + ingestion restored |
| 6 | `bd remember` root-cause + runbook | `[CREATE]` | beads memory | DONE — key `vmsingle-storage-readonly-self-blinded-2026-07-11` |

## Rejected / not done

- **In-cluster Watchdog VMRule** — rejected: any in-cluster alert is evaluated
  by vmalert against vmsingle and dies with the very failure it should catch.
  The external gatus check is the correct layer.
- **Alert on vmsingle pod restarts / read-only self-metric** — rejected: the
  self-metric (`vm_storage_is_read_only`) is itself stored in the read-only
  vmsingle, so it goes stale exactly when needed. Freshness-from-outside is the
  only reliable signal.
- **Second Longhorn replica for vmsingle** — not proposed here (would raise
  inter-node WireGuard traffic; single-node TSDB is intentionally expendable).
  Revisit only if metric history becomes worth protecting.
