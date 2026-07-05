---
date: 2026-07-05
status: closed
incident_status: resolved
sessions:
  - 019f3230-1736-7b7d-a71d-58b37aa4133c
components:
  - pc01
  - cilium
  - coredns
symptoms:
  - cross-node TCP into pc01 pods times out / connection reset
  - DNS queries to pc01 coredns time out
  - ICMP into pc01 pods works (0% loss) while TCP/UDP-with-reply fails
  - node pc01 shows Ready while all cross-node pod networking into it is dead
  - argocd.syscd.live 502 flapping (downstream)
failure_mode: vxlan-tx-checksum-offload
affected_urls:
  - https://argocd.syscd.live
beads: [home-systems-w1c]
memories:
  - pc01-crossnode-tcp-ROOTCAUSE-vxlan-tx-offload-2026-07-05
supersedes: []
related:
  - 2026-07-05-argocd-crossplane-webhook-blocks-sync/PM.md
---

# Postmortem: pc01 cross-node pod networking silently broken by virtio VXLAN TX checksum offload

- **Severity/Impact:** All cross-node TCP/UDP traffic INTO pc01's pods (a GPU
  worker VM) silently failed while the node reported `Ready`. Blast radius
  included coredns, argocd-server, and the crossplane provider webhooks that
  live on pc01 — the latter cascaded into a full cluster-wide ArgoCD sync halt
  (see related postmortem). Node was cordoned as a band-aid for hours before the
  root cause was found. No data loss.
- **Root cause (one line):** vxlan-tx-checksum-offload — pc01's Proxmox virtio
  NIC (`ens18`) had guest TX checksum offload on, corrupting the inner L4
  checksum of VXLAN-encapsulated replies so every peer silently dropped them.

## What Happened

pc01 is a Proxmox VM with a virtio-net NIC (`ens18`). Cilium runs in VXLAN
tunnel mode. With guest TX checksum offload enabled (`tx-checksum-ip-generic`,
plus `tx-udp_tnl-*`), the NIC computed a wrong inner checksum for
VXLAN-encapsulated TCP/UDP on egress. pc01 RECEIVED cross-node requests fine and
SENT its reply out `cilium_vxlan`, but the encapsulated reply arrived at peers
with a bad checksum and was dropped by the kernel before Cilium ever saw it.
ICMP survived because the kernel software-checksums small locally-generated
replies, and node-local (pc01→pc01) traffic is never encapsulated — so the node
looked completely healthy (`Ready`, ICMP 0% loss) while every cross-node TCP/UDP
flow into it timed out. This is a well-known virtio/VMware + VXLAN interaction,
not a Cilium bug (cf. cilium/cilium#26300, flannel-io/flannel#1279,
projectcalico/calico#7807).

## Detection Gap (how we catch it next time)

- **What the user saw first:** `argocd.syscd.live` returning 502 / flapping, and
  (on manual probing) cross-node TCP into pc01 pods timing out while ICMP worked
  and the node showed `Ready`. Monitoring was entirely green.
- **How we detect it before the user next time:** the *user-facing* symptom is
  already covered — `ArgoCDClusterCacheDown` (VMRule) + the gatus
  `argocd.syscd.live` check both fire on the blast radius (added by the related
  incident). The *residual* gap is "the durable fix silently stopped running":
  if the `nic-offload-fix` DaemonSet is not Running on a targeted node, that node
  will corrupt cross-node traffic again on its next reboot — and nothing watches
  it. That is the primary new signal.
- **Fix path once detected:** confirm offload state on the node
  (`ethtool -k ens18 | grep tx-checksum-ip-generic`), then the runbook below.

## Mitigation (runbook — how to detect & fix this again)

**Durable fix (applied, committed `500add7c` + `71fbaba2`):** the
`nic-offload-fix` DaemonSet in
`k8s/charts/support-cluster/templates/nic-offload-fix.yaml`, nodeSelector-pinned
to affected virtio VM nodes (currently `pc01`), runs
`ethtool -K <default-route-iface> tx-checksum-ip-generic off` in a 60s loop
(the setting reverts on reboot / link reset, so it must be re-applied).
Deployed via ArgoCD; verified `tx-checksum-ip-generic: off` on `ens18` and
cross-node TCP 0/3 → 3/3. Talos machineconfig exposes no ethtool/offload knob
and the Proxmox VM config (crossplane bpg `EnvironmentVM.networkDevice`) has no
offload toggle, which is why the fix lives in-cluster.

**Diagnose from scratch (the method that worked):**
1. Test from a REAL pod, not a cilium-agent pod — agent pods are hostNetwork and
   host-SNAT'd, a different datapath: `kubectl run nettest --image=nicolaka/netshoot
   --overrides '{"spec":{"nodeName":"tp4","tolerations":[{"operator":"Exists"}]}}'`.
2. From it: `ping` a pc01 pod (works) vs `nc -zvw2 <pc01-pod> <tcp-port>` and
   `dig @<pc01-coredns-pod> ...` (both time out) → cross-node TCP/UDP-with-reply
   into pc01 is dead while ICMP is fine.
3. `cilium-dbg monitor -v` on BOTH the pc01 agent and a peer agent, filtered by
   the pod IP: pc01 shows the SYN arriving AND a `SYN,ACK ... ifindex
   cilium_vxlan` reply leaving, but the peer never receives it → egress
   corruption, not ingress/BPF.
4. Confirm the cause: privileged hostNetwork+hostPID netshoot pinned to pc01,
   `ethtool -k ens18` → `tx-checksum-ip-generic: on`, `tx-udp_tnl-*: on`.
5. Prove it: `ethtool -K ens18 tx-checksum-ip-generic off` → cross-node TCP
   flips 0/3 → 3/3 instantly.

## Dead Ends

- **"Kernel/conntrack corruption, needs a node reboot"** (the prior-incident
  theory, `argocd-502-pc01-crossnode-tcp-broken` memory) — wrong. A reboot only
  *temporarily* fixes it because offloads default back on at boot; that is
  exactly why earlier cilium-agent restarts "didn't hold."
- **`xx drop (Stale or unroutable IP)` in `cilium monitor`** — looked like the
  smoking gun, but it only fired for a stale/dead pod IP (`10.244.3.120`, no
  longer assigned). Live pods showed ZERO BPF drops. Red herring.
- **Testing from cilium-agent (hostNetwork) pods** — host-SNAT'd traffic takes a
  different path and gave inconsistent results; only a real pod source is
  representative.
- **virtio → e1000 NIC swap** (the one crossplane/Proxmox-config lever) —
  rejected: lower throughput, higher host CPU, the known e1000 "Detected
  Hardware Unit Hang" bug, and it re-enumerates the guest NIC, which would break
  Talos's static-IP interface binding and take the node offline.
- **A Proxmox/crossplane VM-config offload toggle** — does not exist; the bpg
  `networkDevice` schema has no offload field. Offload is negotiated inside the
  guest.

## Timeline

### 2026-07-05 (GMT-0300, approximate — clock times not fully captured)
- `~10:30` User reports cross-node TCP into pc01 pods dead (ICMP OK, node
  `Ready,SchedulingDisabled`); asks whether it's a config root cause or just a
  restart.
- `~10:35` Confirmed pc01 is a Proxmox virtio VM (`ens18`, MAC `bc:24:11…`),
  VXLAN tunnel mode, kube-proxy/iptables; MTU correct (1500 → vxlan 1450).
- `~10:45` `cilium monitor --type drop` on pc01 during failing connects → ZERO
  BPF drops for live pods; the one drop seen was a stale dead pod IP.
- `~10:55` Reproduced: netshoot on tp4 → pc01 coredns — ICMP OK, TCP (8080/8181/
  9153) and DNS-over-UDP all time out; pc01-local → coredns works.
- `~11:05` Two-sided `cilium monitor -v`: pc01 receives the SYN, delivers it to
  the pod, sends `SYN,ACK` out `cilium_vxlan`; peer never receives it → egress
  reply corruption.
- `~11:15` `ethtool -k ens18` (privileged netshoot on pc01): `tx-checksum-ip-
  generic on`, `tx-udp_tnl-segmentation on`, `tx-udp_tnl-csum-segmentation on`.
- `~11:20` `ethtool -K ens18 tx-checksum-ip-generic off` → cross-node TCP 0/3 →
  3/3 from all nodes, DNS resolves. **Root cause confirmed.**
- `~11:40` Web research + crossplane bpg schema check confirm: known virtio+VXLAN
  issue; no Proxmox/VM-config offload toggle; guest-side (or host-tap) ethtool
  is the fix.
- `~12:00` Built `nic-offload-fix` DaemonSet in `support-cluster`; committed
  `500add7c`, pushed; ArgoCD synced; pod Running on pc01; `tx-checksum-ip-
  generic: off` verified.
- `~12:10` `kubectl uncordon pc01` (verified 3/3 first); node `Ready`.
- `~12:20` Expanded the DaemonSet comment with context + issue links (`71fbaba2`).
