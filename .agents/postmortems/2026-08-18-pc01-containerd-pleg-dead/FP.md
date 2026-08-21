---
date: 2026-08-18
status: deferred
parent_pm: 2026-08-18-pc01-containerd-pleg-dead
---

# Follow-up Plan: pc01 containerd PLEG death

## Items

1. **[CREATE] Verify KubeNodeUnreachable alert routes to Discord**
   - Confirm vmalert evaluates `node-down.yaml` rules and alertmanager forwards `severity: critical` + `environment: production` to the Discord webhook
   - Test: temporarily lower `for:` or trip the alert, confirm Discord message arrives
   - Priority: P2

2. **[EDIT] Fix nostos status false-positive for rpi01**
   - nostos pings LAN IP (192.168.9.170) which is unreachable from the runner's subnet
   - Option A: fall back to `tailscale_ip` when LAN ping fails
   - Option B: skip ping-down alert for nodes on a different subnet
   - Location: `.submodules/nostos/` (code change)
   - Priority: P3

3. **[EDIT] Fix talosconfig endpoint → macintel01**
   - Current: endpoints = 192.168.68.100 (dell01, now a worker — causes "no request forwarding")
   - Target: endpoints = 192.168.68.91 (macintel01, current controlplane)
   - Command: `talosctl config endpoint 192.168.68.91`
   - Priority: P2

4. **[INVESTIGATE] pc01 Proxmox host management IP unreachable**
   - 192.168.68.101 doesn't respond to ping but VMs run fine
   - Blocks Crossplane reconciliation (talos-pc01 EnvironmentVM SYNCED=False)
   - Likely Proxmox firewall or NIC config; needs physical/console access
   - Priority: P3
