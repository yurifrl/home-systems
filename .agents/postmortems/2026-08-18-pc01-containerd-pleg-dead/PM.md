---
date: 2026-08-18
status: closed
incident_status: resolved
sessions:
  - 01a00a62-47dd-73b1-bf03-fefd0bcb9091
components:
  - pc01
  - containerd
  - kubelet
symptoms:
  - node pc01 NotReady for >2 days (kubelet stopped posting heartbeats)
  - "container runtime is down" in kubelet logs
  - "PLEG is not healthy: pleg was last seen active 3m+ ago" in kubelet logs
  - all pods on pc01 in Error/RunContainerError/CreateContainerConfigError
  - containerd service health OK but kubelet health Fail
  - nostos status shows pc01 (Proxmox host 192.168.68.101) as down (separate from talos-pc01 VM)
failure_mode: containerd-hang-pleg-death
affected_urls: []
beads: []
memories: []
supersedes: []
related:
  - 2026-07-05-pc01-tailscale-flag-drift-crashloop
  - 2026-07-05-pc01-vxlan-tx-checksum-offload
---

# Postmortem: pc01 (talos-pc01) container runtime hang → PLEG death → NotReady

- **Severity/Impact:** The talos-pc01 worker node was NotReady for ~2 days
  (2026-08-16 08:43 UTC onwards). No user-facing services ran exclusively on
  this node, so the impact was limited to: ztunnel, cilium, istio-cni, and
  kube-proxy pods stuck in Error/RunContainerError on pc01. Workloads that
  could schedule elsewhere did. The Proxmox host (192.168.68.101) remained
  unreachable via management IP throughout (separate issue — likely a Proxmox
  networking/firewall config, not hardware-down since the VM continued running).
- **Root cause (one line):** containerd-hang-pleg-death — containerd became
  unresponsive on the talos-pc01 VM (exact trigger unknown — no preceding OOM
  or disk-full observed), kubelet's PLEG stopped getting container status
  updates, kubelet declared runtime down and stopped syncing pods, node went
  NotReady.

## What Happened

The talos-pc01 Proxmox VM (k8s node registered as `pc01`) stopped posting
kubelet heartbeats on 2026-08-16 ~08:40 UTC. The kubelet remained running but
reported "container runtime is down" and "PLEG is not healthy" in a loop. The
Talos `containerd` service reported Health=OK, but CRI calls (ContainerStatus,
ListContainers) returned DeadlineExceeded — indicating containerd was alive at
the process level but its gRPC server was hung or blocked.

All DaemonSet pods on the node (cilium, ztunnel, kube-proxy, istio-cni) entered
Error/RunContainerError. The node remained reachable via Talos API (apid) over
the LAN and via Tailscale.

## Timeline

- **2026-08-16 08:40 UTC** — Last successful kubelet heartbeat from pc01
- **2026-08-16 08:43 UTC** — Node condition transitions to Ready=Unknown (NodeStatusUnknown)
- **2026-08-16 08:43+ UTC** — KubeNodeUnreachable alert should have fired (5m `for:` → ~08:48)
- **2026-08-18 ~14:50 UTC** — User reports "rpi and pc01 are failing" via nostos status
- **2026-08-18 ~14:52 UTC** — Agent confirms containerd hung via talosctl logs; issues `talosctl reboot`
- **2026-08-18 ~14:53 UTC** — Node reboots, containerd + kubelet come back healthy
- **2026-08-18 ~14:54 UTC** — Node transitions to Ready; all pods Running

## Dead Ends

- **rpi01 "failing"**: nostos status showed rpi01 as "down" because its LAN IP
  (192.168.9.170) is on a different subnet unreachable from the nostos runner.
  The node was healthy the entire time (Ready in k8s, all services OK via
  Tailscale IP). Not an incident — network topology expected behavior.
- **pc01 Proxmox host (192.168.68.101) unreachable**: Separate from talos-pc01.
  The Proxmox management IP doesn't respond to ping, but the host IS running
  (VMs are alive). Likely a Proxmox firewall/network config issue, not related
  to the containerd hang on the VM.
- **talosctl "no request forwarding"**: Initial talosctl attempts failed because
  the talosconfig endpoints pointed to dell01 (former CP, now worker). Fixed by
  using macintel01 (192.168.68.91) as the endpoint.

## Detection Gap

The `KubeNodeUnreachable` VMRule alert (`node-down.yaml`) should have fired
within 5 minutes of the node going Unknown. Either:
1. It did fire and routed to Discord, but wasn't acted on for 2 days, OR
2. The alerting pipeline (vmalert → alertmanager → Discord) had a gap

Unable to confirm which — vmalert pod's API endpoint (8880) was refusing
connections during investigation. The alert definition is correct.

## Mitigation

**Reboot the node via talosctl:**
```bash
talosctl -e 192.168.68.91 -n 192.168.68.104 reboot
```
This is the standard recovery for a containerd hang on Talos (no SSH, no
systemctl — Talos has no shell). The reboot clears all container state cleanly.

## Runbook (next occurrence)

1. Confirm node is NotReady: `kubectl get node pc01`
2. Check talos services: `talosctl -e 192.168.68.91 -n 192.168.68.104 services`
   - If kubelet=Fail + containerd=OK (but CRI calls timeout) → containerd hung
3. Reboot: `talosctl -e 192.168.68.91 -n 192.168.68.104 reboot`
4. Verify recovery: wait 30s, check `kubectl get node pc01` → Ready
5. If recurring: check talosctl dmesg/logs for OOM, disk full, or kernel panics
