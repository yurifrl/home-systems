---
created: 2026-05-31T03:54:21Z
project: home-systems
description: Hermes agent chart hardening, refactoring, and dynamic file sync
context: Dedicated Helm chart for hermes-agent on Talos k8s homelab
tags: [hermes, helm, argocd, 1password, kubernetes]
session_name: hermes-chart-stabilization
purpose: Stabilize hermes StatefulSet on dell01, refactor chart defaults vs Application overrides, add git-login init, private values multi-source, and dynamic 1Password file attachment sync
session_id: 019e7c2b-2148-75bd-8a97-f3d8a975f5af
provider: pi
resume_with: cly agent-session resume --provider pi undefined
context_name: undefined
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/undefined.md
---

## Session

- **Name:** hermes-chart-stabilization
- **Purpose:** Stabilize the hermes StatefulSet deployment, refactor chart/application values split, add git authentication init, wire private values via multi-source, and implement dynamic 1Password file attachment sync.
- **Change ID:** Commits from `35a3809b` through `26fe09da` on `main`
- **Resume:** `cly agent-session resume --provider pi undefined`

## Context

The hermes-agent Helm chart was converted from Deployment to StatefulSet by the user. The session continued from that point: fixing scheduling (toleration for dell01 control-plane taint), stabilizing the pod, then progressively refactoring and adding features.

## Problem

1. StatefulSet not scheduling (missing control-plane toleration in live cluster)
2. Application valuesObject was bloated — all config inline instead of chart defaults
3. No git/gh authentication on pod start (github skill prerequisite)
4. Private values (Discord IDs, WhatsApp numbers) were at risk of landing in the public repo
5. 1Password file attachments (GCP SA JSONs) couldn't sync via ESO `dataFrom.extract` — only text fields work
6. OBSIDIAN_PATH pointed at read-only root filesystem (`/obsidian`)

## Decisions

- **migrate-perms init removed** — data already owned 1000:1000; fsGroup handles new files; eliminated the only privileged container
- **Chart defaults vs Application split** — chart holds all hermes infrastructure (security, RBAC, networkPolicy, camofox, litellm URL, externalSecret binding to hermes-env, resources, persistence); Application keeps only `virtualService.hosts`, `nodeSelector`, `tolerations` (+ `enabled: true` for VS)
- **git-login init** — runs `gh auth login --with-token` + `gh auth setup-git` at startup; must `unset GH_TOKEN/GITHUB_TOKEN` first (gh refuses to persist when env var is set)
- **Private values via `$values` multi-source** — `home-systems-values` private repo supplies Discord/WhatsApp identifiers through `$values/hermes/values.yaml`
- **Dynamic file sync via Connect REST API** — `op-files-sync` Python script baked into image; init container queries Connect to list+download all attachments by name (no filenames configured anywhere)
- **Obsidian on dedicated 10Gi PVC** (`hermes-obsidian`) mounted at `/obsidian`, separate from hermes state volume

## Current State

- **Branch:** `main` at `26fe09da`
- **Pod:** hermes-0 was Running 2/2 on dell01 but is currently blocked by **1Password Connect recovering** from the Longhorn v1.8→v1.12 upgrade crash. Connect pod just restarted and returned 200 on vault query. The `hermes-env` secret was deleted during the outage and needs ESO to recreate it.
- **op-files init:** Script is in the image (build `26859519578` succeeded), init is in the StatefulSet, but hasn't successfully run yet (Connect was returning 500). Should work on next pod restart now that Connect is back.
- **Longhorn:** Recovering — 2/3 managers running, endpoints populated. One manager still crashlooping (upgrade path issue from 1.8.1→1.12.0 on one node).
- **ArgoCD hermes app:** Shows OutOfSync (cosmetic SSA drift on StatefulSet + ExternalSecret).

## Next Steps

1. **Verify hermes-env secret recreated** — ESO should sync now that Connect is 200. If not, annotate the ExternalSecret to force reconcile: `kubectl annotate externalsecret -n hermes hermes force-sync=$(date +%s) --overwrite`
2. **Delete hermes-0 pod** to restart with fresh inits (op-files should now succeed)
3. **Verify op-files downloaded attachments** into `/opt/data/files` — `kubectl logs hermes-0 -c op-files` should show filenames
4. **Longhorn upgrade path** — the v1.8.1→v1.12.0 jump is unsupported; needs stepped minor-version upgrades (1.8→1.9→1.10→1.11→1.12) or pinning back to 1.8.x
5. **Rotate leaked PATs** — OpenAI, GH_TOKEN (`github_pat_11AAY35JA0p...`), RENOVATE_TOKEN (`ghp_uh7oQd9...`)
6. **hermes-helper script** — fixed locally (`.bin/` is gitignored), container name `hermes-agent` + secret `hermes-env`

## Key Files

- `k8s/applications/hermes.yaml` — minimal Application (VS host + node pinning)
- `k8s/charts/hermes/values.yaml` — all chart defaults (security, RBAC, netpol, camofox, inits, obsidian PVC)
- `k8s/charts/hermes/templates/extra-objects.yaml` — fixed separator for 2+ objects
- `k8s/images/hermes/Dockerfile` — added `COPY op-files-sync`
- `k8s/images/hermes/op-files-sync` — dynamic 1Password Connect file downloader
- `home-systems-values/hermes/values.yaml` (private repo) — Discord/WhatsApp env vars
- `.bin/hermes-helper` — local ops helper (gitignored)
