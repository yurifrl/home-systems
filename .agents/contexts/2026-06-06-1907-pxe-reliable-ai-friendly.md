---
created: 2026-06-06T19:58:16Z
project: home-systems
description: Migrated cluster CNI to Cilium and made the 1Password Connect bootstrap secret survive cluster rebuilds permanently
context: Talos/nostos home cluster — flannel→Cilium migration plus External Secrets bootstrap recovery
tags: [cilium, talos, nostos, 1password, external-secrets, cni, istio-ambient]
session_name: 2026-06-06-1907-pxe-reliable-ai-friendly
purpose: Research and migrate the cluster from flannel to Cilium, then permanently fix the 1Password Connect bootstrap secret so a rebuild needs no manual re-seed
session_id: 019e9e83-c6f8-79ba-a512-a5878040c344
provider: pi
resume_with: cly agent-session resume --provider pi 2026-06-06-1907-pxe-reliable-ai-friendly
context_name: 2026-06-06-1907-pxe-reliable-ai-friendly
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/2026-06-06-1907-pxe-reliable-ai-friendly.md
---

# Session: 2026-06-06-1907-pxe-reliable-ai-friendly

- **Name:** 2026-06-06-1907-pxe-reliable-ai-friendly
- **Purpose:** Research Talos+Cilium eBPF, migrate the cluster off flannel, then permanently fix the 1Password Connect bootstrap secret so cluster rebuilds need zero manual re-seeding.
- **Resume:** `cly agent-session resume --provider pi 2026-06-06-1907-pxe-reliable-ai-friendly`

## Context

Talos home cluster `talos-default` (control-plane `dell01` 192.168.68.100; workers `tp1`/`tp4` on Tailscale `100.x` InternalIPs), Istio Ambient mesh, ArgoCD GitOps, nostos-provisioned, secrets via 1Password Connect + External Secrets Operator (ESO). etcd is treated as throwaway — all state restores from ArgoCD/git on rebuild.

This session continued from a branch where `dell01` had already been reprovisioned with `cni: none` and Cilium installed. The cluster came back but 5 pods were stuck because the `op-credentials` bootstrap secret (the root of trust ESO can't sync itself) was destroyed by the etcd wipe and re-seeded malformed.

## Problem

1. flannel+kube-proxy → wanted Cilium eBPF (researched, then migrated).
2. After the dell01 rebuild, the `op-credentials` secret in ns `1password` was **double base64-encoded**, so 1Password Connect 500'd (`invalid character 'e'`), the `onepassword` ClusterSecretStore went `InvalidProviderConfig`, and every ExternalSecret silently failed → bind9 (internal DNS), cloudflare-tunnel, external-dns ×2, tailscale-operator all down.
3. User requirement: fix it **permanently and automatically** — a rebuild must need no manual task and no tribal knowledge.

## Decisions

- Cilium Phase 1: keep kube-proxy (`kubeProxyReplacement:false`), `routingMode:tunnel`/`vxlan`, `MTU:1450` (matches flannel; nodes ride Tailscale), `bpf.masquerade:false` (hostDNS `forwardKubeDNSToHost:true` conflict, talos#9200), Istio-safe flags `socketLB.hostNamespaceOnly:true` + `cni.exclusive:false`. Hubble `tls.auto.method=cronJob` so no secrets in git.
- Base CNI is NOT an ArgoCD app for first bootstrap (chicken-egg: no CNI → ArgoCD can't run); manual `helm install` during NotReady window, ArgoCD `k8s/applications/cilium.yaml` adopts day-2.
- 1Password fix: the committed inline-manifest in `dell01.yaml` already auto-seeds `op-credentials` from `op://` at bootstrap. The bug was the **1Password item encoding**, not the cluster. Its `data:` block needs single-base64 of the raw values. Re-stored both fields as clean single-base64 sourced from the proven-working **live secret `.data`** (which also eliminated embedded `\r` chars from base64 line-wrapping that broke `talosctl` YAML parsing).
- Reverted the manual recovery detour (docs section, `task kubernetes:op-credentials`, Taskfile include) — automatic mechanism is the only path.

## Current State

- **Cilium migration: done & proven.** flannel gone; Cilium 1.18.0 on all 3 nodes; `cilium status OK`, 112/112 controllers, 3/3 cluster health; routes accessible (external→MetalLB `.201`→istio-gateway HTTP 404 in 3ms); Istio mesh intact.
- **Cluster fully healthy:** ClusterSecretStore `onepassword` = Valid/Ready; 96 pods Running, 0 unhealthy.
- **1Password permanent fix verified:** `op://kubernetes/op-credentials` fields normalized to clean single-base64; `nostos render dell01` succeeds; `talosctl validate` → valid for metal mode; rendered inline secret decodes to `{` (creds) / `eyJ` (token).
- **Repo changes uncommitted** (git at `de0110f0`):
  - `nostos/templates/dell01.yaml` — `cni: none` + new `namespace-1password` inline manifest.
  - `nostos/templates/rpi01.yaml` — `cni: none`.
  - `k8s/applications/cilium.yaml`, `nostos/manifests/cilium/values-{base,phase1,phase2}.yaml`, `cilium-phase1-rendered.yaml` (Cilium GitOps + bootstrap values).
  - `taskfiles/kubernetes.yml` restored to original; `docs/nostos-guide.md` reverted.
- **Inherited (not mine):** `Taskfile.yml` `nostos:`/`# k8s:` include removals came from the prior branch (`taskfiles/nostos.yml` no longer exists) — left untouched per golden rule.
- `.submodules/nostos/*` has uncommitted changes from the prior branch's PXE work — not touched this session.

## Next Steps

1. Commit the 1Password fix: `nostos/templates/dell01.yaml` `namespace-1password` block (the permanent fix for this issue).
2. Decide whether to commit the Cilium migration files (templates `cni:none`, `k8s/applications/cilium.yaml`, `nostos/manifests/cilium/*`) — live state already diverges from git.
3. Optional later: Cilium Phase 2 (kube-proxy-free via KubePrism 7445), rpi01 offsite join (already prepped, `cni:none`), Hubble metrics into Grafana.
4. Research/plan docs in `.agents/tmp/`: `research-talos-cilium-ebpf.md`, `plan-talos-cilium-migration.md`.
