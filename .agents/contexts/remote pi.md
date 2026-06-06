---
created: 2026-06-06T14:37:19Z
project: home-systems
description: Onboard offsite Raspberry Pi 4 (rpi01) as second controlplane via Tailscale; recover cluster after the resulting --accept-routes regression broke DNS
context: nostos remote-node provisioning + multi-site Talos + Tailscale routing architecture
tags: [nostos, talos, tailscale, raspberry-pi, kubernetes, dns, etcd, istio, openspec]
session_name: remote pi
purpose: Add an offsite RPi 4 to the cluster, build the `nostos flash` command and supporting infrastructure for zero-touch remote-node provisioning, and recover the cluster after the onboarding broke DNS via a Tailscale route hijack
session_id: 019e92b3-0b8b-77c2-aed5-e1427247e838
provider: pi
resume_with: cly agent-session resume --provider pi remote pi
context_name: remote pi
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/remote pi.md
---

# remote pi

## Session

- Name: remote pi
- Purpose: Onboard rpi01 (offsite Pi 4) as a second controlplane via Tailscale, productize the workflow as `nostos flash`, then recover the cluster after the onboarding broke DNS.
- OpenSpec change: `openspec/changes/nostos-flash-command/` (renamed from `nostos-ship-command` mid-session)
- Resume: `cly agent-session resume --provider pi remote pi`

## Context

Existing cluster: dell01 (controlplane, home, 192.168.68.100), tp1 + tp4 (workers, home, Turing Pi BMC), rpi01 (offsite, on 192.168.0.x, joins via Tailscale). Talos v1.13.3, flannel CNI, istio ambient mesh, ArgoCD GitOps, 1Password secrets, Tailscale across sites.

Tooling: nostos (`.submodules/nostos/`) is the bare-metal provisioner — Go CLI with cobra subcommands, schema registry, dryrun envelopes, multi-secret backends, AGENTS.md invariants. Lives in-tree in this repo.

## Problem

Three layered problems hit in sequence:

1. **Onboarding rpi01.** Pi was the decommissioned controlplane SD/SSD. Needed EEPROM network-boot config (manual rpi-imager step), Talos arm64 image with rpi_generic overlay, machineconfig with Tailscale extension, etcd join across LANs.

2. **`--accept-routes` regression.** To get etcd peer comms across sites, `TS_EXTRA_ARGS=--accept-routes` was added to dell01's Tailscale extension. This made dell01 import `10.244.0.0/16 → tailscale0` from advertised routes, hijacking the cluster pod CIDR. Pod return-traffic broke. CoreDNS crashlooped 268 restarts over 25h. Whole-cluster DNS down.

3. **kubelet picked Tailscale IP as InternalIP.** Talos auto-selected `100.96.13.49` (TS) over `192.168.68.100` (LAN). kube-apiserver advertised on TS IP, kube-proxy DNAT'd Service VIP traffic to TS IP, Tailscale's netfilter dropped non-TS-source packets. Compounded the breakage.

Plus: istiod 1.30 ConfigMap content vs 1.26 running pod (chart drift via ArgoCD); ArgoCD partially wedged by a crossplane GCP-provider conversion webhook with no endpoints.

## Decisions

- **Architectural rule:** Never `--accept-routes` on a node that hosts cluster pods. Cross-LAN reach uses Tailscale CGNAT (`100.x.x.x`) only. Pod networking stays on flannel. Etcd peer URLs advertise via `100.64.0.0/10` so they're reachable cross-LAN without route imports.
- **Pin kubelet `nodeIP.validSubnets`** to the LAN subnet on every node so Talos never picks the TS IP as InternalIP.
- **`nostos flash` command** ships a pre-rendered Talos image + sidecar machineconfig for any node. Renamed from `ship` mid-session per user direction. Uses Talos imager-style: download raw image, mint Tailscale key, render config, write to `--out FILE` or `--device /dev/diskN`. RPi nodes get an EEPROM recovery directory.
- **Sidecar config (not embedded)** for v1: ext4 STATE-partition injection is platform-specific; sidecar matches what `node install` already does and works cross-platform.
- **EEPROM recovery as a directory**, not a FAT32 image: cross-platform without `mkfs.fat` / `hdiutil`.
- **istio rolled back 1.30 → 1.26.2** in ArgoCD apps to match the running istiod pod version.
- **`allowSchedulingOnControlPlanes: true`** on dell01 because tp1/tp4/rpi01 were all NotReady at peak crisis and istiod needed to land somewhere.

## Current State

Cluster:
- dell01 (controlplane, home): Ready, hosting CoreDNS (1/1, 1/1), kube-apiserver (advertise on 192.168.68.100), istio-cni, ztunnel.
- tp1 + tp4 (workers, home): Ready, recovered on their own (Tailscale auth refresh).
- rpi01 (controlplane, offsite): NotReady, last seen 1d ago.
- DNS works inside cluster, Service VIPs work, ambient mesh on dell01 healthy.

Live regressions found at session-end:
- istiod still in CrashLoopBackOff (11 restarts, 4h) — ArgoCD shows istio apps at `1.30.0` again because the rollback edits are uncommitted, so ArgoCD pulls 1.30 from `main`.
- dell01 has `node-role.kubernetes.io/control-plane:NoSchedule` taint despite source having `allowSchedulingOnControlPlanes: true` — running config drift.

Source-control state (uncommitted, in working tree):
- `nostos/templates/dell01.yaml` — kubelet nodeIP pin, allowSchedulingOnControlPlanes, no `--accept-routes`.
- `k8s/applications/istio-{base,cni,gateway,istiod,ztunnel}.yaml` — `targetRevision: "1.26.2"` rollback.
- `.submodules/nostos/internal/**` — full `flash` command, multi-arch build, image package, network-detection rewrite, schema registration.
- `k8s/applications/hermes.yaml` + `k8s/charts/hermes/` + `k8s/images/hermes/` — unrelated, not from this session.

OpenSpec change `nostos-flash-command` (in `openspec/changes/`): proposal/design/specs/tasks all done; 32/33 tasks ticked, only the manual physical e2e test is outstanding.

## Next Steps

1. **Commit + push the uncommitted Phase 0 fixes**, split by topic:
   - A: nostos `flash` rename + multi-arch + image package (the openspec change body)
   - B: `nostos/templates/dell01.yaml` changes
   - C: `k8s/applications/istio-*.yaml` rollback to 1.26.2
   - D: hermes changes (separate PR — not from this session)
   - Pushing makes ArgoCD stop reverting the istio rollback; istiod stops crashing.
2. **Verify dell01 running config** has `allowSchedulingOnControlPlanes: true`. If not, render dell01 fresh and `talosctl apply-config`.
3. Add `machine.kubelet.nodeIP.validSubnets: [192.168.0.0/24]` and `cluster.etcd.advertisedSubnets: [100.64.0.0/10]` to `nostos/templates/rpi01.yaml`.
4. Add `cluster.etcd.advertisedSubnets: [100.64.0.0/10]` to dell01 too.
5. `nostos flash rpi01 --out /tmp/rpi01.raw.xz --compress`, ship to offsite, flash + apply-config.
6. Open beads issues for: istio 1.30 upgrade investigation (chart vs `RespectIgnoreDifferences` interaction), crossplane GCP-provider webhook cleanup, rpi01 onboarding tracking.
7. Verify cross-site: pod on rpi01 hitting a Service backed by a pod on tp1/tp4.

## Lessons

- I applied `--accept-routes` revert before proving causation; took dell01 briefly off the tailnet and forced LAN access to recover. Right move would have been route-table + kubelet nodeIP inspection first.
- Static pods in Talos are managed by containerd via `cri`, not kubelet. `kubectl delete pod` on `kube-apiserver-dell01` does NOT restart the actual process. Need kubelet restart + node reboot to guarantee a fresh apiserver process picking up new POD_IP.
- The runtime fix and the source-control fix are different things. Live patches without git commits get reverted by ArgoCD on next sync.
