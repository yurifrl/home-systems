# Follow-up Plan — vmsingle storage exhausted → read-only

Epic: **home-systems-el5** (label `pm:2026-07-11-vmsingle-storage-readonly`)
Postmortem: `PM.md` (same directory)

| # | Action | Marker | Location | Result |
|---|--------|--------|----------|--------|
| 1 | Expand vmsingle PVC `3Gi → 10Gi` | `[EDIT]` | `k8s/applications/victoria-metrics-k8s-stack.yaml` (vmsingle.spec.storage) | DONE — commit `21dfb3b1`, ArgoCD synced VMSingle CR to 10Gi |
| 2 | Pin vmsingle to core node `tp4` | `[EDIT]` | same file (vmsingle.spec.nodeSelector) | DONE — commit `fd09602d`; keeps RWO PVC off cross-site nodes |
| 3 | External freshness dead-man's-switch (`count(up)` empty ⇒ ingestion dead) | `[EDIT]` | `nixos:modules/gatus/config.yaml` | DONE — commit `a15f7cc`; deployed and green via `el5.3` after ingestion was restored |
| 4 | Restore ingestion — resolve stuck Longhorn expansion (`volume-head-001.img already exists` on tp1 replica) | `[TODO]` | Longhorn (tp1 replica) | DONE (`el5.1`) — unstick failed; recreated volume fresh at 10Gi (scale 0 → delete PVC → operator reprovisions). Ingestion restored, ~740MB backlog drained |
| 4b | Size-based retention guard `maxDiskSpaceUsageBytes` | `[EDIT]` | `victoria-metrics-k8s-stack.yaml` | REVERTED (`e6a77375`) — flag is cluster-only, not single-node; crashlooped vmsingle. No size-based retention exists single-node. Recurrence protection = 10Gi headroom (~3.3× 30d) + gatus dead-man's-switch |
| 3b | Deploy gatus check | `[RUN]` | nixos push + `deploy.yml` action | DONE (`el5.3`) — pushed `a15f7cc`, ran `workflow_dispatch target=gatus`, deployed OK, check live+green |
| 5 | Verify gatus check fires + routes to Discord | `[TODO]` | gatus / Discord | OPEN (`el5.2`, P3) — after deploy + ingestion restored |
| 6 | `bd remember` root-cause + runbook | `[CREATE]` | beads memory | DONE — key `vmsingle-storage-readonly-self-blinded-2026-07-11` |
| 7 | Remove `absent(...)` from `CloudflareTunnelDown` so missing telemetry is not labeled a tunnel outage | `[EDIT]` | `k8s/charts/support-cluster/templates/monitoring/cloudflare-tunnel.yaml` | DONE — commit `fd79c7a6`; live expression is `sum(cloudflared_tunnel_ha_connections) == 0` |
| 8 | Add direct vmsingle/vmagent watchdog that posts to Alertmanager without querying stored metrics | `[CREATE]` | `k8s/charts/support-cluster/templates/monitoring/victoriametrics-watchdog.yaml` | IMPLEMENTED, NOT STABLE — commits `fd79c7a6`, `901b8e5f`, `c263b635`; one successful Job followed by BusyBox awk failures |
| 9 | Stabilize watchdog and verify warning/critical/resolved alerts route through Alertmanager to Discord | `[CREATE]` | Beads / live verification | OPEN — `home-systems-el5.4` (P2) |

## Rejected / not done

- **In-cluster Watchdog VMRule** — rejected: a VMRule is evaluated by vmalert
  against vmsingle and dies with the very failure it should catch. The external
  gatus check remains the primary independent layer. The approved CronJob is
  materially different: it reads process endpoints directly and posts to
  Alertmanager without querying stored metrics.
- **VMRule on vmsingle pod restarts / read-only self-metric** — rejected: the
  stored self-metric goes stale exactly when needed. Reading the process metric
  directly from a separate CronJob is valid supplementary detection, while
  freshness-from-outside remains the independent signal.
- **Second Longhorn replica for vmsingle** — not proposed here (would raise
  inter-node WireGuard traffic; single-node TSDB is intentionally expendable).
  Revisit only if metric history becomes worth protecting.
