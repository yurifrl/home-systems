# Follow-up Plan: CNPG/supabase down, cluster-wide volume provisioning dead, no detection

Approved 2026-07-18 (beads). Epic: `home-systems-5sw` (label `pm:2026-07-18-cnpg-supabase-down-no-alert`).
Runbook/write-up: sibling `PM.md`.

Action markers: `[CREATE]` `[EDIT]` `[DELETE]` `[SKIP]`. Executed items gain `→ done:`.

## Alerts
- [CREATE] VMRule `cnpg-health` in `k8s/charts/support-cluster/templates/monitoring/cnpg-health.yaml` — CNPG `pg` ready instances `< 3` (or `cnpg_collector_up` count below desired) for >10m, `severity=critical`+`environment=production`. Verdict: **alert** (symptom, act-now, KSM-independent). Current state: `cnpg-vmpodscrape.yaml` scrapes CNPG metrics; **no VMRule consumes them**. Tracked by `home-systems-5sw.3`.
- [CREATE] container-not-ready rule appended to `k8s/charts/support-cluster/templates/monitoring/pod-readiness.yaml` — `kube_pod_container_status_ready==0` for >20m. Verdict: **alert** (symptom; catches CrashLoopBackOff that `KubePodNotReady` misses). KSM-dependent → gated on `home-systems-5sw.1`. Tracked by `home-systems-5sw.4`.
- [SKIP] new pod-Pending rule — `KubePodNotReady` (pod-readiness.yaml) already covers Pending/Unknown; the gap was KSM health, not a missing rule.

## Dashboards / Panels
- [SKIP] — `cloudnative-pg.json` (gnetId 20417) dashboard already exists; the gap is an alert, not a panel.

## Gatus Entries
- [SKIP] — supabase has no external URL (VirtualService host `supabase` is internal-only); `app.syscd.live` loading wouldn't prove supabase-backend health. In-cluster CNPG VMRule + KSM is the correct detection layer. Existing gatus already covers argocd/prometheus/alertmanager reachability.

## Beads
- [CREATE] Epic: `Postmortem: CNPG/supabase down, cluster-wide volume provisioning dead, no detection` → done: `home-systems-5sw`
  - [CREATE] P1 Fix kube-state-metrics (re-arm KubePodNotReady) → done: `home-systems-5sw.1` (recurrence of hermes `home-systems-i8t.1`, never-done)
  - [CREATE] P1 Make csi-provisioner un-pin + Longhorn cross-site scheduling durable in git → done: `home-systems-5sw.2`
  - [CREATE] P2 CNPG cluster-health VMRule → done: `home-systems-5sw.3`
  - [CREATE] P2 Container-not-ready VMRule (deps: 5sw.1) → done: `home-systems-5sw.4`
  - [CREATE] P3 Verify cnpg-health fires+routes (deps: 5sw.3) → done: `home-systems-5sw.5`
  - [CREATE] P3 Verify container-not-ready fires+routes (deps: 5sw.4) → done: `home-systems-5sw.6`

## Alert Cleanup
- [SKIP] — no standing/noisy alert identified to delete. (KSM's own crashloop is noise but is fixed by `5sw.1`, not by muting.)

## Memory
- [CREATE] `bd remember` key `csi-provisioner-pinned-cordoned-node-2026-07-18` → done.

## Open (pending question)
- The exact durable form of `home-systems-5sw.2` (git-captured Longhorn node scheduling / macintel01 cordon vs. runbook-only) awaits the user's answer to postmortem question #2. Recorded in the bead; not blocking.
