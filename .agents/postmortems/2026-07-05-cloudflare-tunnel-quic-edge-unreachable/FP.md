# Follow-up Plan: cloudflare-tunnel QUIC edge unreachable

Ledger for `.agents/postmortems/2026-07-05-cloudflare-tunnel-quic-edge-unreachable/PM.md`.
Approved 2026-07-05. Filter: `bd list -l pm:2026-07-05-cloudflare-tunnel-quic-edge-unreachable --all`

## Alerts
- [CREATE] VMRule `CloudflareTunnelDown` in `k8s/charts/support-cluster/templates/monitoring/cloudflare-tunnel.yaml` — current state: NOTHING scraped cloudflared and only argocd had an external gatus check. Alert fires when `sum(cloudflared_tunnel_ha_connections) == 0` or the metric is absent, for 3m (severity=critical, environment=production → Discord). One SPOF symptom alert covering every tunnel-fronted service. → done: committed pending (home-systems repo), bead home-systems-c67.1

## Dashboards / Panels
- [SKIP] partial-degradation (`< 4` connections) panel — connections gauge is now scraped; a Grafana panel can be added later, not worth building the JSON now. Not an alert per hygiene (2/4 still serves).

## Gatus Entries
- [SKIP] per-service checks (zigbee2mqtt, other *.syscd.live) — the `CloudflareTunnelDown` alert is the SPOF symptom alert and replaces N per-URL checks. Existing `ArgoCD` gatus entry unchanged.

## Scrape Targets
- [CREATE] `Service` cloudflare-tunnel-metrics (:2000) + `VMServiceScrape` cloudflare-tunnel — in the same file above. → done: committed pending

## Beads
- [CREATE] Epic: `Postmortem: cloudflare-tunnel QUIC edge unreachable` (label `pm:2026-07-05-cloudflare-tunnel-quic-edge-unreachable`) → done: home-systems-c67
  - [EDIT] reparent existing durable-fix bead under epic → done: home-systems-btm (P1: force http2 via GitOps + re-enable selfHeal)
  - [CREATE] task: add cloudflared scrape + VMRule → done: home-systems-c67.1
  - [CREATE] task (P3): verify CloudflareTunnelDown fires and routes → done: home-systems-c67.2

## Alert Cleanup
- none.
