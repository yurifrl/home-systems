---
date: 2026-07-05
pm: 2026-07-05-cross-family-vxlan-endpoint-mesh
epic: home-systems-9xk
---

# Follow-up Plan: cross-family-vxlan-endpoint-mesh

Approved 2026-07-05. Durable fix already landed in `home-systems-h27` (all-Tailscale mesh migration); these are detection + guardrails.

## Items

- **[DONE] cilium connectivity alerting** — `home-systems-9xk.1`
  - `[EDIT]` `k8s/applications/cilium.yaml`: `prometheus.enabled: true` + `operator.prometheus.enabled: true` (serviceMonitor off). Commit `0a64e9de`.
  - `[CREATE]` `k8s/charts/support-cluster/templates/monitoring/cilium.yaml`: `VMPodScrape cilium-agent` (pods `k8s-app=cilium`, named port `prometheus`/9962) + `VMRule CiliumNodeConnectivityDown` (`cilium_node_health_connectivity_status{status="unreachable",type="endpoint"} > 1 for 15m`, `severity: critical` + `environment: production` → Discord). Commit `7881ffdd`.
  - Live-verified the metric is served on `:9962`. Threshold is `>1` to tolerate the persistently-broken macarm01; tighten to `>0` once `.4` lands.

- **[CREATE] verify alert fires + routes** — `home-systems-9xk.2` (P3, open). Confirm `CiliumNodeConnectivityDown` reaches Discord (temporary threshold trip or vmalert/amtool check) and the metric name holds in 1.19.4.

- **[CREATE] drift guard** — `home-systems-9xk.3` (P2, open). Lint/CI/preflight asserting every `nostos/templates/*.yaml` keeps `nodeIP.validSubnets: 100.64.0.0/10` and `k8s/applications/cilium.yaml` keeps `MTU <= 1230` (reverting either silently reintroduces the fault).

- **[CREATE] macarm01 cilium ImagePullBackOff** — `home-systems-9xk.4` (P2, open). macarm01 has a TS InternalIP (nominally migrated) but its cilium pod can't pull its image → not a real mesh member. Also the reason the alert threshold is `>1` not `>0`.

- **[CREATE] Longhorn replication perf panel** — `home-systems-9xk.5` (P3, open). Dashboard panel (not an alert) for rebuild/replication throughput now that inter-node pod traffic is WireGuard-encrypted at MTU 1230 (~236 Mbit/s measured).

- **[SKIP] gatus tailnet icmp for macintel01/rpi01/macarm01** — `home-systems-9xk.6` (closed as skip). The gatus config deliberately excludes roaming nodes (`# Roaming laptops ... excluded on purpose - they go offline`) to avoid nuisance alerts; rpi01 is offsite/flaky for the same reason; and underlay icmp would not have caught this overlay-only fault.

## Artifacts
- Commits: `0a64e9de` (cilium metrics enable), `7881ffdd` (VMPodScrape + VMRule). Prior (durable fix): `ecae6d02` (MTU), `23c9313a` (node templates).
- Memory: `cilium-cross-family-vxlan-mesh-2026-07-05`
- Epic: `home-systems-9xk` (label `pm:2026-07-05-cross-family-vxlan-endpoint-mesh`)
