---
created: 2026-05-31T13:06:00-03:00
project: home-systems
description: Talos v1.13.3 upgrade + Longhorn enablement via nostos, plus an interactive mock dashboard
context: nostos provisioning tool, Talos rolling upgrade, Longhorn storage, dashboard TUI design
tags: [nostos, talos, longhorn, upgrade, tui, dashboard, beads]
session_name: dashboard-mock-and-ci
purpose: Enable Longhorn by upgrading Talos to v1.13.3 (adding iscsi-tools), make nostos upgrade-aware, and design the nostos dashboard via an interactive HTML mock
session_id: 019e7bf4-f940-7ded-a97e-f21d6b006f25
provider: pi
resume_with: cly agent-session resume --provider pi 2026-05-31-1306-dashboard-mock-and-ci
context_name: 2026-05-31-1306-dashboard-mock-and-ci
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/2026-05-31-1306-dashboard-mock-and-ci.md
---

# Session: dashboard-mock-and-ci

- **Name:** 2026-05-31-1306-dashboard-mock-and-ci
- **Purpose:** Enable Longhorn (replacing local-path) by rolling Talos v1.10.3 → v1.13.3 with iscsi-tools, make `nostos` upgrade-aware, and design the future nostos dashboard through an interactive HTML mock.
- **Resume:** `cly agent-session resume --provider pi 2026-05-31-1306-dashboard-mock-and-ci`

## Context
Home lab: 3-node Talos cluster `talos-default` — dell01 (.100, amd64, controlplane), tp1 (.107, arm64 RK1, worker), tp4 (.114, arm64 RK1, worker). ArgoCD GitOps. Provisioned by `nostos` (Go, in `.submodules/nostos/` — a plain tracked dir, NOT a real git submodule; its git IS the parent repo). Issue tracking via beads (`bd`), umbrella `home-systems-qfn`.

## Problem
Longhorn was chosen to replace local-path storage. Longhorn's `longhorn-manager` CrashLoops on Talos because `iscsiadm` is missing — Talos needs the `siderolabs/iscsi-tools` + `util-linux-tools` system extensions, which are baked into the factory installer image (schematic). Required a full Talos upgrade. nostos had no upgrade command and duplicated the install image in templates.

## Decisions
- Target Talos **v1.13.3** (latest stable); step through adjacent minors **v1.11.6 → v1.12.8 → v1.13.3** (Talos only tests adjacent-minor migrations).
- New factory schematics (tailscale + iscsi-tools + util-linux-tools): amd64 `8f04ea6b6016f12a593fa8a87441270075c648cb75482c2d9d3db8cecda47da1`; arm64 (+turingrk1 overlay) `6f9371bccd9df78d8c26521528700b463f25bce1cad97691722a4189719e6aa9`.
- **No etcd snapshot** (user choice). UX-first: `nostos upgrade` computes versions/path/order itself.
- Per-hop **version-matched talosctl**: PATH talosctl v1.13.3 vs v1.10.3 servers caused gRPC `too_many_pings` GoAway; each one-minor hop now uses a talosctl matching the node's current version.
- Dashboard: single interactive HTML mock (not multiple files). Notify on done only (progress is on-screen). Demo/simulate controls live OUTSIDE the TUI frame. Upgrade prompts only in the Upgrade tab.

## Current State
- **nostos code (committed locally, NOT pushed):** qfn.1 render install.image from config (`{{ .InstallImage }}`); qfn.2 `nostos upgrade` (planner ParseVersion/ComputePath/OrderNodes + GitHub catalog + DetectVersion + health-gated exec); qfn.3 upgrade TUI; qfn.4 config.yaml bumped to v1.13.3 + new schematics. `docs/mock-dashboard.html` committed.
- **qfn.7 (version-matched talosctl + client-version parser fix) — UNCOMMITTED** in working tree; this is what made the live upgrade work.
- **Live upgrade:** ran in another session, reached **all 3 nodes on v1.11.6** (sweep 1 done); was mid sweep 2/3 (→v1.12.8) last checked. NOT yet on v1.13.3. Resume: `nostos upgrade --to v1.13.3 --yes` (idempotent).
- **beads:** qfn.1–4 closed; qfn.5 (execute upgrade) + qfn.6 (Longhorn datapath fix + verify) open; qfn.7 + `home-systems-fqt` (test pollutes real ~/.talos/config) open.
- ArgoCD: longhorn app deployed but managers CrashLoop until nodes get iscsi-tools (v1.13.3). longhorn.yaml needs `defaultDataPath: /var/lib/longhorn` (qfn.6).
- Dashboard mock: `docs/mock-dashboard.html` (= `.agents/tmp/nostos-sim.html`), interactive, opens in browser.

## Next Steps
1. Commit qfn.7 (and consider pushing the whole stack).
2. Finish the live upgrade to v1.13.3 (sweeps 2+3), confirm `talosctl get extensions` shows iscsi-tools; close qfn.5.
3. qfn.6: set Longhorn `defaultDataPath: /var/lib/longhorn`, refresh the ArgoCD app, verify managers start and `nodes.longhorn.io` are schedulable.
4. Fix `home-systems-fqt` (a nostos test overwrites the real ~/.talos/config).
5. Optionally turn the dashboard mock into a real nostos `dashboard` rebuild.
