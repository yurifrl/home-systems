---
created: 2026-05-31T20:42:38Z
project: home-systems
description: Enable remote kube-apiserver access over Tailscale and teach nostos to set both kube contexts
context: Tailscale k8s operator API server proxy + nostos kubeconfig tooling
tags: [tailscale, kubernetes, talos, nostos, argocd, kubeconfig]
session_name: tailscale
purpose: Give remote kubectl access to the Talos cluster over Tailscale (least-privilege, no LAN routing) and make nostos write both the LAN and Tailscale kube contexts.
session_id: 019e7f8d-841f-725c-a533-a62f11abe6de
provider: pi
resume_with: cly agent-session resume --provider pi tailscale
context_name: tailscale
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/tailscale.md
---

# Session: tailscale

- **Name:** tailscale
- **Purpose:** Remote kubectl access to the Talos cluster over Tailscale via the operator's API server proxy, plus nostos support for setting both kube contexts.
- **Change id:** commits `7197f842` (tailscale.yaml) and `8bfbe437` (pre-existing batch); nostos submodule + `nostos/config.yaml` changes uncommitted.
- **Resume:** `cly agent-session resume --provider pi tailscale`

## Context
Home k8s is Talos (`admin@talos-default`, API at `192.168.68.100:6443`, tailnet `tailcecc0.ts.net`). The Tailscale operator runs in-cluster (device `tailscale-operator`, tag `tag:k8s-operator`), managed by ArgoCD app `tailscale` (helm chart 1.90.6) at `k8s/applications/tailscale.yaml`. nostos (Go CLI, `.submodules/nostos`) manages Talos and fetches kubeconfig to `~/.talos/kubeconfig`.

## Problem
Off-LAN the cluster API was unreachable. Chosen approach: operator API-server proxy (auth mode) over subnet routing — exposes only the API server, no `--accept-routes`, survives IP changes. Then: make `nostos kubeconfig` produce both the LAN and the Tailscale contexts.

## Decisions
- Operator API-server proxy in **auth mode** over subnet routing (least privilege, no LAN-wide exposure).
- Tailnet grant: `autogroup:admin` -> `tag:k8s-operator` impersonating `system:masters` (cluster-admin). ACLs already allow-all, so no extra tcp:443 grant needed.
- nostos: opt-in `cluster.tailscale_operator` field (empty = disabled); best-effort (warn if `tailscale` CLI missing); restore LAN context as default after `tailscale configure kubeconfig` (which otherwise switches current-context).

## Current State (DONE, working)
- `k8s/applications/tailscale.yaml`: `apiServerProxyConfig.mode: "true"` — committed `7197f842`, pushed, ArgoCD synced; operator log confirms "API server proxy in auth mode is listening on :443".
- Root-cause fixed: ACME DNS-01 cert kept failing with `SetDNS ... 500 failed to create DNS record`; cleared by deleting stale offline device `tailscale-operator-2`. Cert provisioned.
- Verified: `kubectl --context tailscale-operator.tailcecc0.ts.net get nodes` works; `auth can-i '*' '*'` -> yes.
- nostos: added `cluster.tailscale_operator` (config.go), `ConfigureTailscaleContext` + kubeconfig current-context helpers (bootstrap.go), wired into `kubeconfig`/`bootstrap` commands (commands.go), `nostos/config.yaml` set to `tailscale-operator`, 2 new tests. `go build`/`vet` clean, full suite 196 pass.
- Graph refreshed via `graphify update .`.

## Next Steps
- Commit nostos submodule changes (config.go, bootstrap.go, commands.go, bootstrap_test.go) and parent `nostos/config.yaml` — both uncommitted, awaiting user OK.
- Optional: live-run `nostos kubeconfig` once on-LAN (talos fetch needs `192.168.68.100`) to confirm end-to-end both-context write.
