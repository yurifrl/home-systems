# Follow-up Plan: pc01 off the tailnet (ext-tailscale flag-drift crashloop)

Ledger for PM.md `.agents/postmortems/2026-07-05-pc01-tailscale-flag-drift-crashloop/PM.md`.
Approved 2026-07-05. Filter: `bd list -l pm:2026-07-05-pc01-tailscale-flag-drift-crashloop --all`

## Alerts
- [SKIP] VMRule for ext-tailscale health — Talos doesn't expose the extension service state as a scraped metric, and the failure (tailnet unreachability) is best tested from outside the cluster. Covered by the gatus checks below instead.
- [SKIP] per-node "ext-tailscale restarted" cause alert — restarts are routine; the tailnet-reachability symptom check covers the case that matters.

## Dashboards / Panels
- none.

## Gatus Entries
- [CREATE] icmp tailnet-reachability checks in `/Users/yuri/Workdir/Yuri/nixos/modules/gatus/config.yaml` — current state: config had ZERO node/tailnet checks (all entries were HTTPS service checks). Added `Tailnet pc01/dell01/tp1/tp4` (`icmp://<100.x>`, group Tech, `[CONNECTED] == true`, discord, failure-threshold 12 ≈2min). Roaming laptops excluded. → done: commit pending (nixos repo)

## Beads
- [CREATE] Epic: `Postmortem: pc01 off tailnet (tailscale flag-drift crashloop)` (label `pm:2026-07-05-pc01-tailscale-flag-drift-crashloop`) → done: home-systems-y9t
  - [CREATE] task: Add gatus tailnet-reachability checks for always-on nodes → done: home-systems-y9t.1
  - [CREATE] task (P3): Verify pc01 tailnet gatus alert fires and routes to Discord → done: home-systems-y9t.2
- [SKIP] durable-fix bead — incident already durably resolved (persisted state cleared, template diff net-zero); general rule captured in `bd remember pc01-tailnet-reset-fix-2026-07-05`.

## Alert Cleanup
- none.
