---
created: 2026-05-31T22:40:00Z
project: home-systems
description: Migrate terraform infra to Crossplane/external-dns; debug istio gateway HTTPS outage
context: GitOps homelab — GCP via native Crossplane, Cloudflare via external-dns, istio ambient gateway TLS
tags: [crossplane, gcp, cloudflare, external-dns, istio, metallb, argocd, 1password, eso]
session_name: 2026-05-31-2240-hermes-config-secrets
purpose: Move ../terraform infra into the cluster (native Crossplane for GCP, external-dns for Cloudflare DNS) keeping secrets out of the public repo, then chase an istio-gateway HTTPS/443 outage uncovered along the way.
session_id: 019e79b5-eed4-75d3-8f93-824723f8dddd
provider: pi
resume_with: cly agent-session resume --provider pi 2026-05-31-2240-hermes-config-secrets
context_name: 2026-05-31-2240-hermes-config-secrets
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/2026-05-31-2240-hermes-config-secrets.md
---

# Session: 2026-05-31-2240-hermes-config-secrets

- **Name:** 2026-05-31-2240-hermes-config-secrets
- **Purpose:** Migrate `../terraform` infra into the cluster while keeping secret values out of the public repo, then fix an istio-gateway HTTPS outage surfaced during the work.
- **Resume:** `cly agent-session resume --provider pi 2026-05-31-2240-hermes-config-secrets`

## Context

Public GitOps repo `home-systems` (ArgoCD app-of-apps in `k8s/applications/`, charts in `k8s/charts/`). Crossplane core was already installed (`k8s/applications/crossplane.yaml`, v1.20.0). Goal: bring `../terraform/{google,cloudflare,proxmox}` under cluster management.

Key infra facts learned:
- ESO ClusterSecretStore `onepassword` reads the **Kubernetes** vault (personal 1Password `my.1password.com`).
- GCP project = `syscd-443112`. Cloudflare token already in 1Password item `externaldns-cloudflare` (field `cloudflare_api_token`), all-zones.
- Nodes: `dell01` (control-plane, taint), `tp1`, `tp4`. All k8s InternalIPs are tailscale `100.x`. dell01 LAN IP `192.168.68.100`.
- istio runs **ambient mesh**. istio-gateway LB VIP = `192.168.68.201` (MetalLB L2, `externalTrafficPolicy: Local`). bind9 = `192.168.68.200`.
- Another concurrent session was doing tailscale API-server-proxy + cluster upgrade (corrupted `~/.talos/kubeconfig`, rewrote `~/.kube/config` to `192.168.68.100:6443`). Use context `admin@talos-default` on LAN; `~/.talos/kubeconfig` has a `tailscale-operator.tailcecc0.ts.net` context when remote.

## Problem

1. Migrate GCP + Cloudflare DNS into the cluster, hiding bucket names/UUIDs/project-id from the public repo.
2. Decide native Crossplane vs OpenTofu vs external-dns per provider (Cloudflare native Crossplane providers are immature/archived).
3. Mid-session, `https://argocd.syscd.dev` "refused to connect" — turned into a deep istio-gateway HTTPS/443 outage investigation.

## Decisions

- **GCP → native Crossplane** (Upbound official `provider-gcp-{storage,cloudplatform,iam}` v2.5.4 on Crossplane 1.20 cluster-scoped MRs). Import existing infra via `crossplane.io/external-name` + `deletionPolicy: Orphan`.
- **Cloudflare → external-dns DNSEndpoints** (NOT Crossplane, NOT OpenTofu). Native Crossplane CF providers are archived/v0.1.x; OpenTofu was scaffolded then scrapped per user. Records are public (zone names/tunnel IDs not treated as secret).
- **Secret hiding** via ArgoCD multi-source `$values` from PRIVATE repo `git@github.com:yurifrl/home-systems-values.git` (GCP only).
- **Proxmox** left in Terraform.
- istio-gateway: `skipSchemaValidation: true` required (chart 1.26.2 vs newer Helm). **dell01 pin was WRONG** — MetalLB produced no L2 announcement for `.201` from dell01; reverted.

## Current State

WORKING (verified live):
- GCP Crossplane: 4 buckets, 4 SAs, 3 project services, WIF pool+provider all SYNCED/READY (imported). SA `crossplane@syscd-443112` created with roles; key in 1Password item `crossplane-gcp`/`creds`. ArgoCD repo creds for private repo set via ESO (1Password `argocd-home-systems-values`, deploy key `argocd-readonly`).
- Cloudflare: 7 DNSEndpoints applied; external-dns cloudflare+tailscale instances Running, records present in CF with TXT ownership.
- external-dns: fixed dead Bitnami image via `global.security.allowInsecureImages: true` (all 3 instances).
- istio-gateway: app syncs (`skipSchemaValidation`), dell01 pin reverted, gateway on tp4, `.201` announced, **HTTP/80 → 301 works**.

BROKEN (unresolved):
- **istio-gateway HTTPS/443 → 000 (resets)** for ALL hosts. Pre-existing (reset on tp1/tp4 before any change). Ruled out: DNS, cert validity, cert/key match (syscd-tls valid LE through Aug 28, modulus matches), node placement, MetalLB, ambient capture (gateway `dataplane-mode=none`), ztunnel. 443 dies at TLS layer; HTTP/80 fine.

## Next Steps

1. Answer: did `.dev` HTTPS ever work (regression vs never-configured)?
2. Focused HTTPS/443 probe now that gateway is settled: verbose curl + correlate istio-gateway envoy access log (does 443 conn reach envoy?); inspect 443 filter-chain/route in `pilot-agent request GET /config_dump`; check envoy SSL stats. Suspect MTU on large TLS ClientHello, or envoy 443 filter-chain/route binding.
3. Consider concurrent cluster-upgrade/istio churn from the other session as a cause.
4. Verify final HTTPS from a LAN browser (this session can't always reach `.201` when on tailscale).
