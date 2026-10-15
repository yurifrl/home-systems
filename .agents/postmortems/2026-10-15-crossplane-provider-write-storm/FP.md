# Follow-up Plan: crossplane-provider-write-storm

<!-- Created 2026-10-15 after approval. Implementation ledger for PM.md in this
     directory. Epic label: pm:2026-10-15-crossplane-provider-write-storm.
     Filter the whole incident: tk list --all | grep pm:2026-10-15-crossplane -->

Action markers:
- `[CREATE]` new file/resource/bead, with exact path or title
- `[EDIT]`   change to an existing file, with exact path
- `[DELETE]` removal (e.g. noisy alert cleanup)
- `[SKIP]`   considered and rejected at approval — one-line why (nothing happens)

Each executed item gains `→ done: <bead-id | commit | link>` inline.

## Alerts
<!-- Current state verified 2026-10-15: k8s/charts/support-cluster/templates/monitoring/
     has argocd.yaml (cache-level ArgoCDClusterCacheDown only — no per-app sync signal),
     crashloop.yaml (KubePodCrashLooping, fired correctly), control-plane.yaml (blackbox
     probes, fired correctly). argocd metrics scraped via argocd-vmservicescrape.yaml
     (app.kubernetes.io/part-of=argocd, port http-metrics) — argocd_app_* series available. -->

- [CREATE] VMRule `ArgoCDAppSyncFailed` + `ArgoCDAppOutOfSync` in
  `k8s/charts/support-cluster/templates/monitoring/argocd-sync.yaml` —
  → done: file shipped with a third alert added during implementation,
  `ArgoCDAppSyncPendingStuck` (argocd_app_sync_pending{phase="Running"} stuck 30m+ —
  the incident's actual wedge shape: op stuck Running while app reported Synced).
  Live /metrics name-verification was BLOCKED (macarm01 CNI dead: vmsingle pod
  proxy i/o timeout, kubelet SPDY silent hang, port-forward no handshake);
  series names grounded instead via in-repo argocd-operational-overview.json
  (argocd_app_sync_total{cluster,dest_server,job,namespace,phase,project} in live
  queries). Live re-check is tk-gjx's first step.
- [CREATE] task (P3): "Verify ArgoCDAppSyncFailed/OutOfSync fire and route to Discord" —
  force a failing sync (duplicate-env values change on a scratch app), confirm the alert
  fires, confirm Discord delivery, revert scratch.
  → done: tk-gjx
- [EDIT] `k8s/charts/support-cluster/templates/monitoring/control-plane.yaml` —
  KubeAPIServerSlow description: add .agents/postmortems/2026-10-15-crossplane-provider-write-storm/PM.md
  beside the existing 2026-08-16 runbook link (provider status-write storm = second known cause).
  → done: shipped (this commit)

## Dashboards / Panels
- [SKIP] control-plane write-storm panel — EEMUA: diagnostic detail with no distinct human
  action; NodeCPUCritical/KubeAPIServerSlow already route and their runbooks now point here.

## Gatus Entries
- [SKIP] buzz.syscd.live check — declined at approval 2026-10-15; KubePodCrashLooping covers
  pod-level buzz failures.

## Beads
<!-- tk (bd is now a tk symlink with no remember command — verified 2026-10-15). -->
- [CREATE] Epic: `Postmortem: crossplane-provider-write-storm` (label
  `pm:2026-10-15-crossplane-provider-write-storm`, description links PM.md, note = runbook
  summary since bd-remember no longer exists)
  → done: tk-kft
  - [EDIT] tk-ac9 "Upgrade provider-proxmox-bpg past async-callback status-write conflict
    bug, then unpause EnvironmentVMs" — re-parent to this epic (created mid-incident under
    the pc01 label)
    → done: re-parented → tk-kft
  - [EDIT] tk-qpb "Investigate/resize macintel01 control-plane saturation" — re-parent to
    this epic (same)
    → done: re-parented → tk-kft
  - [CREATE] task (P3): "Audit + merge or delete stale fix branches" — image-updater fix
    sat unmerged 5 weeks (2026-09-05 → 2026-10-14), invisible to ArgoCD and alerts
    → done: tk-ndu

## Alert Cleanup
- (none — all firing alerts during this incident were real)
