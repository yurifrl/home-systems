---
created: 2026-10-14
project: home-systems
description: Proxmox host pc01 wedged by 14G QEMU overcommit; OOM killed, right-sized to 12G, node rejoined; PM/FP drafted, approvals pending.
session_id: 01a139bc-0a0c-71a3-8ac0-27509119e81e
resume_with: omp --resume 01a139bc-0a0c-71a3-8ac0-27509119e81e
snapshot_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/snapshots/pc01-qemu-overcommit-wedge.md
---

## Context
pc01 (Proxmox PVE host, 192.168.68.112, Ryzen 3400G 4C/8T, 15.6GiB) wedged by QEMU memory overcommit: chart values fixed talos-pc01 guest at 14336MiB with ballooning removed (2026-09-20), so every qmstart either insta-failed (exit 1, every ~20min from 2026-10-12T12:20Z, ~140 attempts) or overcommitted. The one that allocated anon-rss 14.0G wedged the host ~40min (TCP accepts, sshd banner timeout, pveproxy dead, ping OK). OOM killer killed kvm at host-time 09:55:01 ("100.scope: Failed with result 'oom-kill'"). Git fix 2d266077 pushed memoryMiB 14336->12288 mid-wedge; Crossplane reconciled it as soon as the API returned; start at 09:55 succeeded, guest RSS 12.6G, host 13.6/15.9G, node pc01 (Talos v1.13.3 ISO) Ready. Host never rebooted (up 3 days).

## Decisions
- Guest fixed memory for 15.6G host: 12288 MiB max (guest + ~2.5G QEMU/PVE overhead); keep balloon:0 (Talos has no memory hotplug)
- Detection via Gatus HTTPS probe https://192.168.68.112:8006 expecting 401 + RTT<2000 (kube-apiserver pattern); NOT ICMP - ping stays green through the wedge
- In-cluster VMRule NodeMemoryPressureWarning (>90% used 10m, warning) as guest-side backstop; PVE host itself remains covered only by the Gatus probe
- Rename gatus 'Tailnet pc01' -> 'Tailnet talos-pc01' (it probes the guest 100.101.182.40, label was wrong)

## Current State
- Incident RESOLVED and verified end-to-end: VM 100 running on 12G since host-time 09:55, node pc01 Ready, host 13.6/15.9G used, Crossplane MRs all Synced=True/Ready=True.
- Git fix pushed: commit 2d266077, k8s/charts/crossplane-proxmox/values.yaml talos-pc01.memoryMiB 14336 -> 12288.
- PM.md and FP.md written at .agents/postmortems/2026-10-14-pc01-qemu-memory-overcommit-wedge/ (not yet committed — LAN lost before push).
- Workstation is on iPhone hotspot (172.20.10.6/28); home LAN 192.168.68.x and cluster unreachable; Tailscale has no route to home LAN (ai-subnet-router-b serves 10.31.0.0/16 only).
- PVE API creds reusable at /tmp/pve-creds.json (endpoint/username/password); auth works via curl --data-urlencode.
- Blocked live evidence: host journal harvest (insta-fail stderr) and crossplane provider-log timeline — both need LAN or console.

## Lessons
- PVE task logs keep only the final line for qmstart ("QEMU exited with code 1") — stderr lives in the host journal (journalctl -u qemu-server@100), not the API task log.
- ps -C qemu-system-x86_64 misses PVE VMs (comm is 15 chars, binary is /usr/bin/kvm — use ps aux | grep kvm).
- TCP-connect success is NOT liveness during memory thrash: kernel backlog accepts while userspace stalls.
- ICMP-based monitors are blind to exactly this failure mode.

## Next Steps
- Get user approval on FP.md plan (2 gatus edits + 1 VMRule + 4 beads), then implement
- Restore workstation to home LAN (currently on iPhone hotspot) - blocks: journal harvest for insta-fail stderr, crossplane provider-log timeline check, final commit+push of the two postmortem files
- Open Ends: verify insta-fail cause (expect KVM cannot allocate 14G region), confirm node pc01 ISO upgrade path (v1.13.3 vs fleet v1.14.2) as separate work
