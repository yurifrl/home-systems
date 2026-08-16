# Follow-up Plan: Crossplane finalizer retry storm

Approved 2026-08-16. Epic: **home-systems-cnm** (`pm:2026-08-16-crossplane-finalizer-retry-storm`).
Filter the incident: `bd list -l pm:2026-08-16-crossplane-finalizer-retry-storm --all`.

Action markers: `[CREATE]` `[EDIT]` `[DELETE]` `[SKIP]`. Executed items gain `→ done:`.

## Detection

- [EDIT] `/Users/yuri/Workdir/Yuri/nixos/modules/gatus/config.yaml` — replace the
  stale `kube-apiserver dell01` endpoint with the active control plane
  `kube-apiserver macintel01` at `https://100.65.212.5:6443/readyz`. Preserve
  both existing availability and two-second response-time conditions; both are
  production checks and route to Discord after 12 failed intervals.
  → done: pending validation and commit.
- [EDIT] `k8s/charts/support-cluster/templates/monitoring/control-plane.yaml` —
  add `environment: production` to the existing two critical control-plane
  alerts so they match the VMAlertmanager Discord route. Add
  `KubeAPIServerSlow`: `probe_duration_seconds` for the existing authenticated
  control-plane Pod LIST probe exceeds two seconds for five minutes. It is an
  actionable symptom alert; its runbook is this incident PM.
  → done: pending validation and commit.
- [CREATE] P3 bead to prove the slow-API alert evaluates and routes to Discord
  without disrupting the control plane.
  → done: **home-systems-cnm.1**.
- [SKIP] new external availability check — preserved the existing Gatus check
  instead because it already covers the external API availability symptom.

## Root Cause

- [CREATE] P1 bead to diagnose remaining `macintel01` kube-apiserver CPU and
  timeouts after the Crossplane finalizer loop cleared. Scope: recurring CNPG
  Cluster reads, kube-system Pod LISTs, and metrics scrapes.
  → done: **home-systems-cnm.2**.

## Incident Tracking

- [CREATE] Epic for this incident.
  → done: **home-systems-cnm**.
- [CREATE] `bd remember` knowledge entry with root cause and recovery runbook.
  → done: pending.
