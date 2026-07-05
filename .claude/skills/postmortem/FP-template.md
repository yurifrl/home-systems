# Follow-up Plan: <name>

<!-- Created ONLY after the user approved implementation. Declares exactly what
     gets created — nothing else — and is updated in place with the resulting
     bead ids, commits, and links as each item lands. This file persists as the
     implementation ledger for the sibling PM.md. -->

Action markers:
- `[CREATE]` new file/resource/bead, with exact path or title
- `[EDIT]`   change to an existing file, with exact path
- `[DELETE]` removal (e.g. noisy alert cleanup)
- `[SKIP]`   considered and rejected at approval — one-line why (nothing happens)

Each executed item gains `→ done: <bead-id | commit | link>` inline.

## Alerts
<!-- Hygiene verdict per signal: alert | dashboard-panel | log-only | skip. -->
<!-- State the current state you verified (gatus config, VMRule dir, dashboards). -->
- [CREATE] VMRule <signal> in `k8s/charts/support-cluster/templates/monitoring/<topic>.yaml` — current state: <what exists today>

## Dashboards / Panels
<!-- Prefer [EDIT] of an existing dashboard over [CREATE]. -->

## Gatus Entries
<!-- Outside checks: use when losing the alert on cluster/alert-infra death matters,
     or when the internet→service path is what's verified (live URLs). If an existing
     entry covers it, propose nothing. Everything else is a VMRule. -->

## Beads
<!-- Epic labeled pm:<YYYY-MM-DD>-<slug>; children created with --parent inherit it.
     Filter the whole incident: bd list -l pm:<slug> --all -->
- [CREATE] Epic: `Postmortem: <name>` (label `pm:<YYYY-MM-DD>-<slug>`, description links PM.md)
  - [CREATE] task: <durable fix>
  - [CREATE] task (P3): verify each created alert actually fires and routes

## Alert Cleanup
- [DELETE] <existing noisy/standing alert> — <why>
