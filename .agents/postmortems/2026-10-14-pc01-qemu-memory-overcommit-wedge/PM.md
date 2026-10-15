---
date: 2026-10-14
status: closed
incident_status: resolved
sessions:
  - 01a139bc-0a0c-71a3-8ac0-27509119e81e
components:
  - pc01
  - crossplane
  - proxmox
symptoms:
  - PVE host pc01 (192.168.68.112) API and SSH accepting TCP but not answering (sshd banner timeouts, pveproxy hangs)
  - host stayed pingable through the whole wedge (LAN + monitor agents saw "up")
  - VM 100 start failing "QEMU exited with code 1" every ~20 min for ~2 days (10-12T12:20Z onward, ~140 attempts)
  - one start attempt that got further anon-rss'd 14.0G and wedged the host ~40 min until OOM kill
  - "started just now, then crashed" as observed by the user
failure_mode: qemu-memory-overcommit-host-wedge
affected_urls:
  - https://192.168.68.112:8006
tk: [tk-wtv]
memories: [pc01-qemu-memory-overcommit-wedge-2026-10-14]
supersedes: []
related:
  - 2026-08-16-crossplane-finalizer-retry-storm
  - 2026-08-18-pc01-containerd-pleg-dead
  - 2026-07-05-pc01-tailscale-flag-drift-crashloop
---

# Postmortem: pc01 (Proxmox host) QEMU memory overcommit wedges host; VM 100 start-loop for 2 days

- **Severity/Impact:** The Proxmox host `pc01` (192.168.68.112) was
  administratively dead ~40 min (API/SSH accepting TCP, unresponsive above
  L4) during the wedged start attempt, and VM 100 (`talos-pc01`, sole
  consumer of the host's iGPU passthrough) failed to start for ~2 days
  before that. No Kubernetes outage: the k8s node `pc01` (the guest) was
  NotReady the whole time but carries no pinned workloads.

## What Happened

**Standing config was physically impossible.** The 2026-09-20 chart change
deliberately removed ballooning (`floatingMiB` deleted) and fixed the guest
at 14336 MiB, with a comment assuming "~1.6GiB left for PVE itself" on a
15.6 GiB host. QEMU does not run in a vacuum: anon-rss landed at 14.0G
with PVE userspace (~1.5G) on top — >16G committed. Any start either
instant-failed or overcommitted.

**Phase 1 — silent 2-day start loop.** From 2026-10-12T12:20Z every
Crossplane reconcile retried `qmstart 100` every ~20 min; QEMU exited
code 1 immediately ("TASK ERROR: start failed: QEMU exited with code 1",
PVE task UPIDs from 10-12T12:20Z to 10-14T11:40Z). Insta-fail stderr was
not captured in the task log (one line only; host journal not yet
harvested — see Open Ends) and nobody noticed: the node was simply
absent, and nothing alerts on a PVE host or its VMs.

**Phase 2 — the wedge.** On 2026-10-14 ~12:32 (one user reboot attempt +
one manual API start) an allocation got further, grabbed 14.0G anon-rss,
and starved the host: TCP accepted everywhere (LAN backlog), but sshd
never sent a banner, pveproxy hung, `qm`/API calls timed out. The host
stayed pingable the entire time — the standing Gatus `Tailnet pc01` ICMP
check (and any ping-based monitor) saw "up" throughout.

**Phase 3 — recovery + fix.** At 09:55:01 (2026-10-14, host clock) the
kernel OOM killer killed the wedged qemu:

```
Out of memory: Killed process 2537550 (kvm) total-vm:15.5G anon-rss:14.0G
100.scope: Failed with result 'oom-kill'
```

Load average had been 8.01 (15 min) and drained to ~1.3 within ~15 min.
The git fix (2d266077, `memoryMiB: 14336 → 12288`) was pushed during the
wedge; Crossplane reconciled it as soon as the API came back, and the
next start at 09:55 with `memory: 12288` succeeded: guest RSS 12.6G,
host settled at ~13.6/15.9G used with 2.2G+ available. Node `pc01`
(Talos v1.13.3 from the standing ISO) rejoined the cluster Ready within
minutes. No reboot was needed — `up 3 days` through the whole incident.

## Detection Gap

- **What the user saw first:** "started just now, then crashed" on the
  Proxmox web UI during a manual start attempt.
- **What existed:** `NodeMemoryCritical` VMRule — but it evaluates
  node_exporter metrics from Kubernetes nodes only; the PVE host is not a
  k8s node and has no node_exporter. Gatus had only an ICMP tailnet check
  mislabeled `Tailnet pc01` targeting 100.101.182.40 — that is the Talos
  **guest**, not the host, and ICMP survived the wedge anyway.
- **What did not exist:** any probe against the PVE API/8006, any
  memory-pressure signal from the host, any alert on VM-start failure
  loops (a ~140-attempt, 2-day QEMU-fail loop raised nothing).

## Mitigation / Runbook

1. If the PVE host wedges (TCP-accepts but no HTTP/SSH response): don't
   power-cycle first. The OOM killer reclaims the guest in tens of
   minutes; watch with `ping` + one-shot `curl -k https://…:8006` probes.
2. Root-cause a start-loop by reading the **host journal**, not the task
   log: PVE's `/nodes/{node}/tasks/{upid}/log` kept only the final
   `QEMU exited with code 1` line. `journalctl -u qemu-server@100` (or
   dmesg for OOM lines) carries the stderr.
3. Size guests against **committed, not configured**, memory: guest RAM
   + QEMU overhead (~1G) + PVE userspace (~1.5G) ≤ host RAM. 12288 is the
   largest safe fixed size for this 15.6G host.
4. Talos guests cannot balloon (no memory hotplug in the Talos kernel) —
   keep `balloon: 0`; the fix must be a smaller fixed allocation.

## Dead Ends

- SSH during the wedge: `Connection timed out during banner exchange` —
  kernel accepted TCP; userspace was stalled. Not an auth or firewall
  problem; no amount of retrying during the wedge helps.
- "Host must be boot-looping" theory: wrong — `uptime` showed 3 days;
  the host had never rebooted. The repeated "crash" the user saw was the
  manual start attempt failing live, not repeated reboots.
- `ps -C qemu-system-x86_64` on the host returned nothing — Linux `comm`
  is 15 chars; PVE's binary is `/usr/bin/kvm`. Match with `ps aux | grep
  kvm`.
- PVE API ticket POST with a password containing shell-special chars
  failed with "authentication failure" when the password went through
  shell interpolation; `curl --data-urlencode` fixed it.
- Trying to reach 100.101.182.40 for SSH/API during the outage: that
  tailnet IP belongs to the guest VM, which was down; the PVE host has no
  tailnet presence at all.

## Timeline

### 2026-10-12 (UTC)
- `12:20` First failed `qmstart 100` (task UPID pc01:000B84C6…). QEMU
  exit code 1, no alert. Retries every ~20 min begin and continue for
  ~2 days (~140 attempts). Driver unverified: provider-proxmox pod logs
  (current container since 10-11T09:19Z) contain zero `talos-pc01`
  lines before 10-14T12Z — the loop is invisible in Crossplane logs;
  the reconcile loop is plausible but unproven.
  - Node alerts were blind the entire window: kube-state-metrics
    (pod vmks-kube-state-metrics-…-8nw2x on rpi01) restart-looped from
    10-11T09:12Z on liveness `livez` timeouts (208 restarts) and had no
    stable samples until 10-14T13:00Z — `kube_node_status_condition` is
    absent in 10-11T10:00→10-14T13:00, so `KubeNodeNotReady`/`…Unreachable`
    matched nothing. Coverage returned 10-14T13:45: KubeNodeNotReady fired
    13:45→17:50 for pc01 and delivered ~59 Discord notifications
    (alertmanager_notifications_total 64→123). Verified via
    alertmanager_notifications_total + ALERTS history in vmsingle.

### 2026-10-14
- `~11:40Z` Last pre-incident failed start observed in task list.
- `~12:20Z` Host wedged by a start attempt that allocated 14.0G anon-rss
  (user reboot attempt + manual API start). API `000`, sshd banner
  timeouts; host pingable throughout.
- `12:32` Manual API start attempt fired while wedged.
- `~12:50Z` Git fix pushed: `crossplane-proxmox` values
  `talos-pc01.memoryMiB 14336 → 12288` (commit 2d266077).
- `09:55:01` (host local) Kernel OOM killer kills the wedged kvm
  (anon-rss 14.0G). Host load 8.01 → 1.24 within ~15 min.
- `09:55` Crossplane reconciles `memory: 12288` onto the host config;
  `qmstart 100` succeeds (UPID pc01:0026B96F…, TASK OK). Guest RSS
  12.6G; host 13.6/15.9G used.
- `~10:07` k8s node `pc01` (Talos v1.13.3) Ready. VM MR Synced=True,
  Ready=True, LastAsyncOperation=Success.

## Open Ends

- Insta-fail stderr for the 2-day start loop (phase 1): PVE task log
  kept one line only; the host journal (`journalctl -u qemu-server@100`
  around 10-12T12:20Z) was not harvested before LAN access was lost —
  pending, most plausible cause is KVM cannot create a 14G region
  (`cannot set up guest memory … Cannot allocate memory`) but unverified.
- The exact event that turned a 2-day insta-fail loop into one that
  allocated (user reboot attempt vs scheduled reconcile racing a
  transient host-memory dip) — ordering reconstructed from task UPIDs
  and user report, not from a single log.
- Node `pc01` runs Talos v1.13.3 from the standing ISO (fleet is
  v1.14.2). Pre-existing, out of scope here; flagged for a separate
  image-upgrade pass.
