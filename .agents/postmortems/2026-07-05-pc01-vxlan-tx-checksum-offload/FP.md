# Follow-up Plan: pc01 vxlan-tx-checksum-offload

Approved 2026-07-05 (session 019f3230-1736-7b7d-a71d-58b37aa4133c).
Sibling: PM.md. Epic: home-systems-w1c (`bd list -l pm:2026-07-05-pc01-vxlan-tx-checksum-offload --all`).

## Alerts
- [CREATE] VMRule `NicOffloadFixNotRunning` in `k8s/charts/support-cluster/templates/monitoring/nic-offload-fix.yaml` — current state: no such alert existed; `kube_daemonset_status_*` is scraped (kube-state-metrics). Verdict: alert (actionable — guards the durable fix; near-zero noise). `severity: critical` + `environment: production` (only combo that routes to Discord). → done: committed (see Beads/commit below)
- [SKIP] Hubble drop/flow re-detection of the raw datapath fault — this fault produced ZERO BPF drops (kernel-level), Hubble never sees it. Diagnostic/log-only.
- [SKIP] new user-facing symptom alert — already covered by `ArgoCDClusterCacheDown` VMRule + gatus `argocd.syscd.live` (related incident). No new work.

## Dashboards / Panels
- [SKIP] nothing adds action beyond the alert above.

## Gatus Entries
- [SKIP] existing `argocd.syscd.live` outside-check covers the externally-visible blast radius.

## Beads
- [CREATE] Epic: `Postmortem: pc01 vxlan-tx-checksum-offload` (label `pm:2026-07-05-pc01-vxlan-tx-checksum-offload`, description links PM.md) → done: home-systems-w1c
  - [CREATE] task (P2): extend `nicOffloadFix.nodeSelector` to new Proxmox/UTM virtio nodes → done: home-systems-w1c.1
  - [CREATE] task (P3): verify `NicOffloadFixNotRunning` fires + routes to Discord → done: home-systems-w1c.2
- [SKIP] durable-fix task — already delivered and closed as home-systems-pzl (DaemonSet, commits 500add7c + 71fbaba2).

## Alert Cleanup
- [SKIP] no noisy/standing alert identified to remove.
