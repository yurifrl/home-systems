---
created: 2026-06-06T01:30:00Z
project: home-systems
description: Replace hermes camofox sidecar with standalone camofox + self-hosted firecrawl Applications, then trim memory for hobby use
context: hermes browser tooling, ArgoCD, k8s capacity, Crossplane GCP webhook fallout
tags: [hermes, camofox, firecrawl, argocd, crossplane, browser-stack, helm, kubernetes]
session_name: hermes-browser-backends-trim
purpose: Diagnose camofox combobox bug, replace sidecar with standalone Applications for camofox + firecrawl, then shrink the deployment footprint for a low-use hobby cluster
session_id: 019e8ae4-7c3c-7c96-aeb0-74aa13625541
provider: pi
resume_with: cly agent-session resume --provider pi undefined
context_name: undefined
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/undefined.md
---

# Hermes browser backends — camofox + firecrawl

## Session
- Name: hermes-browser-backends-trim
- Purpose: Replace the in-pod camofox-browser sidecar with a standalone Application, add a self-hosted Firecrawl Application as a second browser/scrape backend, get hermes routing to both, then drop the resource footprint to fit a hobby cluster.
- Resume: `cly agent-session resume --provider pi undefined`

## Context
- Repo: `/Users/yuri/Workdir/Yuri/home-systems` (GitOps via ArgoCD; private values repo `home-systems-values` referenced via `$values` source).
- Hermes Application uses both repos; `k8s/charts/hermes/values.yaml` lives in the public repo, private PII (`Discord/WhatsApp` IDs) stays in the values repo.
- Cluster: 1 control-plane (`dell01`, amd64, 7.2 GB) + arm64 SBCs `tp1`, `tp4` + an in-progress `rpi01`. tp1/tp4 had pre-existing `kube-proxy` `CreateContainerError` / `RunContainerError`, making in-cluster service IPs unreliable from those nodes.
- Existing Crossplane GCP providers (`provider-gcp-iam`, `provider-gcp-storage`, `provider-gcp-cloudplatform`) had unhealthy Deployments with 404'ing conversion webhooks. Argo's cluster cache sync iterates every CRD, so those dead webhooks blocked **every** Argo Application from reconciling.

## Problem
1. **Combobox bug in camofox**: server.js (v1.11.2) hardcodes `INTERACTIVE_ROLES` *without* `combobox`. ARIA-combobox search inputs (Mercado Livre, Amazon, eBay, ...) get no `[eN]` ref. Hermes' `browser_type` then fills the adjacent `<button class="nav-search-btn">`, Playwright `fill()` rejects it, server returns HTTP 500. Reproduced locally with `npx @askjo/camofox-browser` and the production image — same behaviour. No fix in any released version, no upstream issue.
2. **Hermes wanted firecrawl** alongside camofox for read-only scraping (`web_search`/`web_extract`/`web_crawl` already plumbed in `tools/web_tools.py`; reads `FIRECRAWL_API_URL` when set).
3. **Argo was wedged** for the entire cluster because of the Crossplane GCP webhook 404s.
4. **dell01 was getting overloaded** as we pinned more pods to it; control-plane + apiserver kept wobbling.

## Decisions
- **Run camofox as its own ArgoCD Application** (`k8s/applications/camofox.yaml`) using the local `support` chart with a `Deployment` + `Service` on port 9377 + 1 GiB memory-backed `/dev/shm` emptyDir.
- **Pin camofox to dell01** with control-plane toleration; the support chart's `deployment.yaml` was extended to support `tolerations` so the chart could carry it (commit `28500901`).
- **Self-host firecrawl by forking** `firecrawl/firecrawl/examples/kubernetes/firecrawl-helm` (pinned to upstream commit `42b46be4f75a`) into `k8s/charts/firecrawl/`. Couldn't use upstream directly because templates hardcoded pod specs without `nodeSelector`/`tolerations`. Single divergence from upstream: a `global.{nodeSelector,tolerations}` block inserted into all 9 deployment templates.
- **Pin all firecrawl pods to dell01**: tp1/tp4 are saturated with system services and have broken kube-proxy. RabbitMQ self-shut on tp4 via its own `system_memory_high_watermark` alarm.
- **Don't fix Crossplane**, decouple Argo from it: add `resource.exclusions` for every `*.gcp.upbound.io` apiGroup in `argocd-cm` (committed in `manifests/values/argocd.yaml`).
- **Trim hard for hobby use**: disable `extractWorker` entirely (AI extract unused), drop heap sizes in chart templates (api 6→1 GB, workers 3→0.75 GB, prefetch 2→0.4 GB), and override per-service `resources.requests` to a quarter of upstream defaults.

## Current State
- ArgoCD Applications: `camofox=Synced/Healthy`, `firecrawl=Synced/Healthy`, `argocd=Synced/Healthy`.
- All firecrawl pods running on dell01 with the new sizing (verified live before the last cluster wobble):
  - `firecrawl-api` 256Mi req / 1Gi lim
  - `firecrawl-worker` 256Mi / 768Mi
  - `firecrawl-nuq-worker` 384Mi / 768Mi
  - `firecrawl-nuq-prefetch-worker` 128Mi / 384Mi
  - `firecrawl-rabbitmq` 256Mi / 512Mi
  - `firecrawl-nuq-postgres` 128Mi / 512Mi
  - `firecrawl-playwright` 256Mi / 1Gi (fixed in `bc6ee21c` after first OOM-loop)
  - `firecrawl-extract-worker` deleted
- camofox 256Mi req on dell01.
- dell01 memory-request alloc dropped from 92% → 72%; CPU limit alloc 88% → 67%.
- Hermes was working end-to-end mid-session (`curl ... /v0/scrape` returned markdown for `example.com`), but `hermes-0` is currently stuck `Pending` because Longhorn refuses to attach its PVC (`node dell01 is not ready` per Longhorn). Pre-existing Longhorn confusion, not browser-stack.

## Direct cluster mutations made (NOT in git)
- `DeploymentRuntimeConfig/default` in `crossplane-system` was patched directly to pin Crossplane providers to dell01 with control-plane toleration + small resource limits. Saved at `.agents/tmp/crossplane-drc-default.yaml`. Should be moved into the `crossplane-providers` chart eventually.
- `Deployment/provider-gcp-iam-2328da873e88` and the matching `ProviderRevision` were deleted during debugging. Crossplane never recreated them (lease/leader-election timeouts). User can either restart `deploy/crossplane` or delete the `Provider` resource if GCP IAM isn't in active use.
- The `argocd-cm` `resource.exclusions` field was patched live multiple times. The committed `manifests/values/argocd.yaml` value has the same content, so future Argo syncs will preserve it.

## Next Steps
- Fix Longhorn so hermes' PVC can attach and the new hermes deploy (with `git-clone`/`git-sync` containers) can complete.
- Repair `kube-proxy` on tp1 and tp4 (`CreateContainerError`, `RunContainerError`). Talos-side fix; until done, in-cluster service IPs are flaky on those nodes and dell01 carries everything.
- Decide what to do with the broken Crossplane GCP providers: either restart `crossplane` core to regenerate the missing IAM `ProviderRevision`/`Deployment`, or delete the GCP `Provider` resources entirely if you don't actively use them. The `resource.exclusions` is fine to keep as a defense-in-depth either way.
- Optional follow-up: add an upstream issue at `jo-inc/camofox-browser` proposing that `INTERACTIVE_ROLES` either include `combobox` or filter combobox-with-`aria-haspopup=dialog` instead of all comboboxes — current behaviour breaks searching on every major site.
- Optional follow-up: tweak hermes' tool descriptions in `tools/browser_tool.py` to nudge the LLM toward "navigate directly to search results URL" instead of typing into nav search boxes — workaround for the combobox bug as long as camofox is in use.
