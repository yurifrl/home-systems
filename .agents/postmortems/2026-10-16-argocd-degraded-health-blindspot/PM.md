---
date: 2026-10-16
status: closed
incident_status: resolved
beads:
follow_ups: none (declined by user 2026-10-16 — documentation only)
sessions:
  - 01a139bc-0a0c-71a3-8ac0-27509119e81e
components:
  - argocd
  - crossplane
failure_mode:
  - argocd-degraded-health-blindspot
symptoms:
  - ArgoCD app health Degraded (crossplane-proxmox, crossplane-gcp, argocd) with no alert, no page, no notice
  - HPA ScalingActive/FailedGetResourceMetric ("missing request for cpu") invisible at app level
  - OutOfSync alert noise for unrelated benign drift (applications, buzz, nostos-root-0, private-apps)
detection: user-initiated sweep
---

# ArgoCD Degraded-Health Blindspot (paused VMs, HPA cpu request, disabled GCP billing)

## Summary

Three ArgoCD Applications sat `Degraded` with **zero alerting**: no ArgoCD
health-based alert exists, Gatus only probes the UI returning 200, and the
sync-status alerts shipped 2026-10-15 only cover `OutOfSync`/sync-pending.
The Degraded states were only discovered by a user-initiated sweep of
`kubectl get applications`.

Three independent root causes were hiding behind one symptom:

| App | Root cause | Outcome |
|---|---|---|
| `crossplane-proxmox` | Chart-declared `crossplane.io/paused: "true"` on all 3 `EnvironmentVM`s; argo-cd builtin crossplane lua maps `Synced=False` → Degraded regardless of `Ready=True` | Fixed — paused-aware health lua ([642d938c]) |
| `argocd` (self) | `argocd-server` HPA: `ScalingActive/FailedGetResourceMetric` — server container had no cpu **request** while the chart HPA targets cpu+memory | Fixed — cpu requests ([efb1a726]) |
| `crossplane-gcp` | GCP billing disabled on `syscd-443112` + `555680140769`; every Crossplane observe 403s | Accepted state (user decision: leave disabled), documented |

## Impact

- Trust erosion in ArgoCD health: `Degraded` became ambient noise, which
  defeats the signal for real failures. During this incident all three apps
  kept serving workloads (`Ready=True`), so blast radius was informational.
- The cost-control stack itself is offline: `billing-discord` /
  `billing-killswitch` Cloud Functions cannot call GCP APIs while billing is
  disabled, so the automated killswitch cannot manage billing if costs spike.
- Unknown duration of degraded states — HPA conditions date to HPA creation
  (~2026-10-14), VM pauses to the 2026-10-14 write-storm mitigation. Nothing
  surfaced them for 1–2 days.

## Detection

**Gap.** Manual sweep. Evidence gathered after the fact:

- `argocd_app_info` **is** scraped (VMServiceScrape, 30s interval) — the
  metric needed for a health alert was available the whole time; only the
  alert rule was missing.
- vmalert during the incident: only `ArgoCDAppOutOfSync` firing (for
  unrelated benign drift on `applications`/`buzz`/`nostos-root-0`/
  `private-apps`) — proving routing works, and proving no health rule exists.
- Gatus: `https://argocd.syscd.live == 200` only — server liveness, not app
  health.

## Root Cause

Three contributing gaps, one systemic failure mode:

1. **Health semantics**: argo-cd v3.5.1 builtin
   `resource_customizations/_.upbound.io/_/health.lua` maps `Synced=False` →
   `Degraded` unconditionally. The proxmox chart deliberately pauses VMs
   (async-callback status-write storm protection) but never added the matching
   ArgoCD health customization. The repo's own hardening checklist
   (`docs/argo-crossplane-hardening.md`, item B3) prescribed exactly this fix
   but it was never implemented.
2. **Chart values**: argo-cd chart HPAs (server + repo-server) target both
   cpu and memory; values only set **memory** requests. Without a cpu request
   the HPA can never compute utilization → `FailedGetResourceMetric` →
   builtin HPA health → Degraded → app Degraded.
3. **GCP billing** disabled across all of Yuri's projects (one open billing
   account `000600-2E9369-0C8480`, zero linked projects) — deliberate cost
   stop, but it 403s every Crossplane GCP observe.
4. **No health alert** on `argocd_app_info{health_status="Degraded"}` — the
   actual recurrence enabler. Without it, items 1–3 are invisible until a
   human looks.

## Red Herrings

- Initial source dive of v3.5.1 controller aggregation concluded "zero
  health-checkable kinds in these apps → app must be Healthy" — wrong because
  `ResourceHealthSource: appTree` + per-GVK **lua** health overrides
  (crossplane/upbound kinds) also feed the aggregation; built-in Go checks are
  not the whole story.
- argocd-server v3.5 REST API returns 404 for `/api/v1/*` (grpc-first
  restructure) — wasted a port-forward + probe cycle; in-pod `argocd` CLI was
  the working path.
- "`The CronJob never completed successfully`" health messages on
  `crossplane-proxmox-iso-pipeline`/`...-cloud-init-render` looked causal;
  both CronJobs were actually `Healthy` in the tree.
- The `argocd` self-app's Degraded was first attributed to its own cm drift
  from the lua commit; real cause was the HPA.

## Timeline

### 2026-10-16
- `~06:30` Sweep begins: user asks to investigate `crossplane-proxmox` /
  `crossplane-gcp` `Degraded` (pre-compaction session work; board-games and
  victoria-metrics items of the same sweep already resolved earlier).
- `~06:47` argocd-server probing: REST API 404s (v3.5 grpc-first); switch to
  in-pod CLI. Tree dump reveals real degraded nodes.
- `~06:50` `crossplane-gcp`: 6 MRs `observe failed ... Error 403` — billing
  disabled on `syscd-443112`; confirmed disabled on `555680140769` too.
- `~06:55` `crossplane-proxmox`: 3 `EnvironmentVM`s `Synced=False
  ReconcilePaused`, `Ready=True`; pause annotations are chart-declared.
- `~07:00` Paused-aware lua added to `configs.cm`
  (`manifests/values/argocd.yaml`); rendered cm validated; pushed [642d938c].
- `07:05–07:06` `argocd` app auto-sync runs (redis-secret-init hook churn,
  benign); cm lands, lua key present.
- `07:08:39` `crossplane-proxmox` re-reconciles → **Synced/Healthy**.
- `~07:20` GCP billing relink proposed; **user decision: leave disabled**.
- `~07:30` Final health check exposes `argocd` self-app `Synced/Degraded`;
  flat list shows no degraded resource → hunt via tree, then HPAs.
- `~07:35` `argocd-server` HPA `ScalingActive=False/FailedGetResourceMetric`
  ("missing request for cpu in container server"); cpu requests added to
  server + repo-server values; pushed [efb1a726].
- `~07:40` Sync + rollout; HPA → `ScalingActive=True/ValidMetricFound`;
  `argocd` → **Synced/Healthy**; changelog [e69cc072]+[52df487f].
- Post-incident evidence pass: vmalert shows `ArgoCDAppOutOfSync` firing for
  benign drift; **no health alert ever fired during the incident**;
  `argocd_app_info` live with 124 Healthy / 1 Degraded (crossplane-gcp,
  accepted).

## Verified Current State

| Application | Sync | Health |
|---|---|---|
| `crossplane-proxmox` | Synced | Healthy |
| `argocd` | Synced | Healthy |
| `crossplane-gcp` | Synced | Degraded (accepted — billing off) |

## Follow-ups

Proposed 2026-10-16, **declined by user — documentation only**:

- `ArgoCDAppDegraded` VMRule on `argocd_app_info{health_status="Degraded"}`
  (critical + production, deduped by app name, excluding crossplane-gcp).
- Dedupe existing `ArgoCDAppOutOfSync` / `ArgoCDAppSyncPendingStuck`
  (5 argocd-server replicas → duplicate series → double notifications).
- Bead for standing OutOfSync noise (applications/buzz/nostos-root-0/
  private-apps — appset self-reference churn, currently firing).

None created. The postmortem is the record. Related prior art:

- Unpausing the proxmox VMs (removes the underlying `Synced=False` cause) is
  tracked upstream: upgrade `provider-proxmox-bpg` past the async-callback
  bug, then unpause — see
  `.agents/postmortems/2026-10-15-crossplane-provider-write-storm/` and its
  follow-up task.
- GCP billing stays disabled by decision; if the `billing-discord` /
  `billing-killswitch` functions are ever needed live, relink both projects
  (`syscd-443112`, `555680140769`) to billing account `000600-2E9369-0C8480`.

[642d938c]: https://github.com/yurifrl/home-systems/commit/642d938c
[efb1a726]: https://github.com/yurifrl/home-systems/commit/efb1a726
[e69cc072]: https://github.com/yurifrl/home-systems/commit/e69cc072
[52df487f]: https://github.com/yurifrl/home-systems/commit/52df487f
