---
date: 2026-08-09
status: draft
incident_status: resolved
sessions:
  - 019fe3fd-bcc5-7d67-9b55-90081f51654c
components:
  - argocd
  - applicationset
  - gitops
symptoms:
  - argocd.syscd.live 502 Bad Gateway (Cloudflare)
  - all ArgoCD core workloads deleted (server, repo-server, application-controller, redis, dex)
  - ApplicationSet error "function \"path\" not defined"
failure_mode: appset-revival-dual-ownership-prunes-argocd-install
affected_urls:
  - https://argocd.syscd.live
beads: []
memories: []
supersedes: []
related:
  - 2026-07-05-argocd-crossplane-webhook-blocks-sync
  - 2026-07-13-apiserver-crd-cache-oom
---

# ArgoCD core install pruned by a revived, redundant ApplicationSet

## Summary

While verifying an unrelated `cloudflare-access` consolidation, the operator
"fixed" the `applications` ApplicationSet — a bootstrap component that had been
broken and inert since Aug 2025. Reviving it created a **second manager** over
`k8s/applications/`, which was already owned by the `argocd` app-of-apps
(`path: k8s/applications`, `prune: true`, `selfHeal: true`, created
2026-06-06). The resulting ownership churn pruned the ArgoCD **core install**
(server/repo-server/application-controller/redis/dex) — which is installed
out-of-band via `manifests/argocd.yaml` and tracked by neither manager — and
the UI went `502`. Self-inflicted; not a monitoring failure.

## Impact

- `argocd.syscd.live` returned `502 Bad Gateway` (screenshot 2026-08-10 01:37 UTC).
- All ArgoCD control-plane workloads deleted; GitOps fully down (no reconcile).
- Recovery took a single `kubectl apply` of the install manifest + reverting
  the ApplicationSet change. No data loss (Applications and their resources
  survived; only the argocd install itself was pruned).

## Timeline

- Session task: verify the `cloudflare-access` chart consolidation another session had landed.
- Observed `cloudflare-access` Application absent from the cluster despite being on `origin/main`.
- Root-caused to the `applications` ApplicationSet erroring: `manifests/applicationset.yaml:28` had `include: '{{ path.filename }}'` but the set is `goTemplate: true`, so it errored `function "path" not defined` and generated zero apps. Broken since commit `0781e632` (2026-08-19), a partial goTemplate migration that converted the `name:` line to `.path.filename` but left the `include:` line un-migrated.
- Fixed the line to `{{ .path.filename }}`, committed `a3dbd29c`, pushed.
- `kubectl apply -f manifests/applicationset.yaml` — the now-working ApplicationSet began generating apps from `k8s/applications/*.yaml`, overlapping the `argocd` app-of-apps that already manages the same directory.
- ArgoCD core Deployments/StatefulSet (server, repo-server, application-controller, redis, dex) were pruned → `argocd.syscd.live` 502.
- User reported the 502.
- Diagnosis: zero ArgoCD core pods; only `argocd-image-updater` remained; no `argocd-server`/`repo-server`/`application-controller` Deployment or StatefulSet objects present.
- Mitigation: `kubectl apply -f manifests/argocd.yaml` (== `task argo:apply`) recreated the core install.
- Reverted the ApplicationSet template (back to `{{ path.filename }}`, commit `915e2c44`, applied live) → ApplicationSet inert again, single manager restored.
- Verified: `argocd-server`, `repo-server`, `application-controller-0`, `redis`, `dex` all `1/1 Running`; `argocd-server` endpoint `10.244.2.162:8080` live; 502 resolved.

## Dead Ends

- **"All `*.syscd.space` Access resources deleted"** — a `kubectl get trustaccessapplications` returned "No resources found" and was briefly read as a full deletion/outage. It was an apiserver query **timeout returning empty** under control-plane load; the resources were actually healthy (53m old). Cost several minutes of false-alarm panic.
- **Node-reaper / `macarm01 NotReady` as the cause** — considered whether node eviction killed ArgoCD. Red herring: node problems leave Deployments in place (pods pending), they do not delete Deployment/StatefulSet objects. The disappearance of the controller objects themselves pointed to a prune, not a scheduling failure.

## Root Cause

Two GitOps managers own the same directory:
1. `applications` **ApplicationSet** — git `files` generator over `k8s/applications/*.yaml`, `preserveResourcesOnDeletion: false`. Inert since 2026-08-19 (goTemplate `include` bug), so it silently generated nothing.
2. `argocd` **Application** (app-of-apps) — `path: k8s/applications`, `prune: true`, `selfHeal: true`, since 2026-06-06.

With the ApplicationSet inert, (2) was the sole active manager and the system was stable. Reviving (1) introduced dual ownership; the churn pruned the ArgoCD core install (installed out-of-band via `manifests/argocd.yaml`, owned by neither), taking down GitOps.

Contributing cause: the operator changed a broken-but-load-bearing bootstrap component that another session was actively working on, treating a broad "fix all" as license to touch the root ApplicationSet, without pausing to ask why a long-lived component was in a broken state.

## Mitigation / Runbook

- **ArgoCD core missing (502, no `argocd-server` pod, no core Deployments):**
  `kubectl apply -f manifests/argocd.yaml` (or `task argo:apply`) recreates the
  install. Pods come back in ~1–2 min; confirm `argocd-server` `1/1 Running`
  and its Endpoints are populated.
- **Do not revive the `applications` ApplicationSet** while the `argocd`
  app-of-apps manages `k8s/applications`. Exactly one manager owns that
  directory. The ApplicationSet is intentionally inert; leave it.
- The ArgoCD install (`manifests/argocd.yaml`) is applied out-of-band, not via
  GitOps — nothing prunes it in steady state, and nothing should start to.

## Detection Gap

- What the user saw first: `argocd.syscd.live` → `502` in the browser.
- This symptom is **already covered**: gatus has an external `ArgoCD` endpoint
  check (`nixos/modules/gatus/config.yaml`, 10s interval, `[STATUS] == 200` +
  body `<title>Argo CD</title>`, Discord alert). It tests from outside the
  cluster, so it survives an ArgoCD/apiserver outage — the correct backstop,
  and it exists. The gap here was **not monitoring** but a self-inflicted
  config change.
- The in-cluster `argocd-rules` VMRule (`ArgoCDClusterCacheDown`) is useless for
  this failure — when the argocd install itself is deleted there is no
  controller to emit `argocd_cluster_connection_status`, and it is in-cluster so
  lost with the outage. The external gatus check is the signal that matters.
- Open question for follow-up: did the gatus `ArgoCD` alert actually fire to
  Discord during this window? If not, that is the real detection defect to fix.
