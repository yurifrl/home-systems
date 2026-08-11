---
date: 2026-08-11
postmortem: 2026-08-11-tailscale-gateway-orphaned-device
epic: home-systems-sys
status: in-progress
---

# Follow-up Plan: *.syscd.tech Tailscale gateway orphaned device

Epic `home-systems-sys` (label `pm:2026-08-11-tailscale-gateway-orphaned-device`).
Approved via `/close postmortem` (recommended defaults applied: default ProxyClass for metrics; liveness-probe kept as investigate-only; no new gatus check; incident_status stays `mitigated` pending verification of v1.102.2).

## Durable fix

- `[DONE]` **Operator upgrade 1.98.9 → v1.102.2** — candidate fix for the auth-key-reissue optimistic-lock wedge. Committed to git out-of-band by the user (`9a9a646f chore(deps): update helm release tailscale-operator to v1.102.2`), synced by the `private-apps` app-of-apps, operator `Running` on v1.102.2. **Unproven** until a real reissue event is observed.
- `[CREATE] home-systems-sys.1` (P1) — Verify v1.102.2 actually fixes the wedge on the next reissue (no optimistic-lock churn, in-place re-auth, no manual device deletion). Gates `incident_status: resolved`.
- `[CREATE] home-systems-sys.3` (P2) — Orphaned tailnet-device reaping so a re-provision reclaims the stable `syscd-gateway` hostname automatically (no manual Tailscale-API deletion). No device-deleting controller.
- `[CREATE] home-systems-sys.5` (P3) — Investigate-only: liveness probe tied to tailscaled health to auto-restart a logged-out proxy. Noted as NOT sufficient for this incident (name conflict needed device deletion). Keep/drop decision after `.1`.

## Detection (KSM-independent — KSM is dark again)

- `[CREATE] home-systems-sys.2` (P1) — ProxyClass `metrics.enable=true` (CRD already installed, metrics currently off) → `VMPodScrape` of the proxy pods (vmagent healthy) → `VMRule TailscaleGatewayUnhealthy` (`k8s/charts/support-cluster/templates/monitoring/`) firing on `tailscaled_health_messages{type=~"login-state|not-in-map-poll"}` for the gateway proxy >10m, `severity: critical` + `environment: production` (confirm routing in `alertmanagerconfig.yaml`). Verify exact metric name after enabling. gatus stays as external backstop.
- `[CREATE] home-systems-sys.4` (P3) — Trip the new VMRule and confirm it pages to Discord.
- `[SKIP]` New gatus `.tech` check — existing `Httpbin Via Tailscale` + app checks already cover the path; the VMRule adds the in-cluster direct signal. Flap-masking is inherent to gatus and not cleanly fixable with a threshold.
- `[SKIP]` KSM-derived VMRule (pod-not-ready / restart-rate) — `vmks-kube-state-metrics` CrashLoopBackOff, metrics dark (tracked by `home-systems-5sw.1` / `home-systems-k5b`).

## Reconcile / metadata

- `[EDIT] home-systems-8f0` (was P0 → P2) — root cause corrected (macarm01-flap theory did not apply; healthy proxy landed on tp4), PM linked, labelled `pm:2026-08-11-tailscale-gateway-orphaned-device`, kept open only for the macarm01 VM-stability angle.
- `[EDIT]` `2026-07-05-pc01-tailscale-flag-drift-crashloop/PM.md` — add bidirectional `related:` back-link.
- `[DONE]` `bd remember --key tailscale-gateway-orphaned-device-2026-08-11`.
- `[DONE]` PM.md `beads: home-systems-sys`, `memories:`, `status: closed`.
