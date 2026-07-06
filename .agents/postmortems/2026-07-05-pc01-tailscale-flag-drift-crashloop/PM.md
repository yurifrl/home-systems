---
date: 2026-07-05
status: closed
incident_status: resolved
sessions:
  - 019f3483-f957-757c-b783-6499199b27dc
components:
  - pc01
  - tailscale
symptoms:
  - pc01 absent from `tailscale status` (no 100.x address on the tailnet)
  - ext-tailscale service crashlooping every ~5s (STATE Waiting, restart forever)
  - "tailscale up failed: exit status 1" / "failed to auth tailscale"
  - "non-default flags ... use --reset" in ext-tailscale logs
  - node pc01 shows Ready and is LAN-pingable (192.168.68.104) while off the tailnet
failure_mode: tailscale-config-flag-drift-crashloop
affected_urls: []
beads: [home-systems-y9t]
memories: [pc01-tailnet-reset-fix-2026-07-05]
supersedes: []
related:
  - 2026-07-05-pc01-vxlan-tx-checksum-offload
  - 2026-07-05-cross-family-vxlan-endpoint-mesh
---

# Postmortem: pc01 off the tailnet (ext-tailscale flag-drift crashloop)

- **Severity/Impact:** pc01 (talos-pc01, 192.168.68.104) absent from the tailnet — no 100.x address, no mesh membership. Node stayed Ready and LAN-functional, so no user-facing service went down, but pc01 could not be a Tailscale-endpoint mesh member (blocked the TS-endpoint migration tracked in home-systems-h27, and pc01<->cross-site-guest pod traffic stayed dead). Latent for days.
- **Root cause (one line):** `tailscale-config-flag-drift-crashloop` — removing `--accept-routes` from the node's tailscale config without a one-time `--reset` left the persisted tailscaled state disagreeing with the boot command, so `tailscale up` refused the non-default flag change and crashlooped.

## What Happened
On 2026-07-03 `--accept-routes` was removed from pc01's tailscale `ExtensionServiceConfig` (`nostos/templates/talos-pc01.yaml`) to stop the LAN-hijack fault. The template change was correct and committed, but the node's persisted tailscaled prefs still carried `--accept-routes=true`. On every boot the extension runs `tailscale up` without `--accept-routes`; Tailscale refuses to silently flip a non-default persisted flag and exits 1 with "use --reset". The extension retried forever (~every 5s), never authenticated, and pc01 never registered — so it was simply missing from `tailscale status`. A fresh auth key does NOT fix this: the flag-reconciliation refusal is independent of the key, which is why prior reboots/re-renders never held.

## Detection Gap (how we catch it next time)
- **What the user saw first:** they noticed pc01 was missing from the tailnet ("find why pc01 [is out] of tailnet"). Nothing paged — pc01 stayed `Ready` (kubelet is fine; only the tailscale extension was down), so no node/service alert fired.
- **How we detect it before the user next time:** an external tailnet-reachability probe. Gatus runs on `digitalocean-gatus-01` (100.104.111.25), itself on the tailnet, so an `icmp://<node-tailscale-ip>` check per always-on node fails the moment that node drops off the tailnet — the exact symptom, tested from outside the cluster. This is the primary alert candidate.
- **Fix path once detected:** one-time `--reset` re-auth via nostos (see Mitigation) — a follow-up bead documents/automates this.

## Mitigation (runbook — how to detect & fix this again)
Symptom: node Ready + LAN-pingable but absent from `tailscale status`; `talosctl -n <ip> service ext-tailscale` shows STATE Waiting, restart-forever, logs `tailscale up failed: exit status 1` and `non-default flags ... use --reset`.

Fix (no reboot — an ExtensionServiceConfig change just restarts the ext-tailscale service):
1. Add a one-time `--reset` to the tailscale extension env in `nostos/templates/talos-pc01.yaml`:
   ```yaml
   - TS_EXTRA_ARGS=--reset
   ```
2. Render (mints a fresh authkey) and apply over LAN:
   ```bash
   cd .submodules/nostos
   go run ./cmd/nostos --config <repo>/nostos/config.yaml render talos-pc01
   go run ./cmd/nostos --config <repo>/nostos/config.yaml apply  talos-pc01 --yes
   ```
   (`nostos` is a fish wrapper for exactly this `go run`; from bash invoke it directly.)
3. Verify: `talosctl -n 192.168.68.104 service ext-tailscale` = STATE Running with a single clean start; `tailscale status | grep pc01` shows it back (100.101.182.40).
4. Remove the `--reset` line and re-apply so future boots don't wipe intentional prefs. Net template diff is zero.

General rule: whenever you REMOVE a `tailscale up` flag from a Talos node's ext config, it won't take until the node also `--reset`s once (or re-auths from a clean state). A silent template flag-reduction crashloops the extension.

## Dead Ends
- The pre-existing bead `home-systems-6ai` framed the cause as an expired/consumed auth key ("Auth key tskey-auth-k9su5... failing"). Partly a red herring: a fresh key alone would NOT have fixed it — the blocker was the persisted `--accept-routes` flag mismatch, which `--reset` clears regardless of key.
- The 9 "failed to auth" lines still visible in `talosctl service ext-tailscale` events AFTER the fix were stale buffered events, not fresh flapping — the most-recent events showed a single clean start. Trust the latest event block / a live `tailscale status`, not the retained event count.

## Timeline
### 2026-07-05
- (prior, 2026-07-03) `--accept-routes` removed from `nostos/templates/talos-pc01.yaml`; node's persisted tailscaled prefs still carried it.
- `23:00`–`23:02` ext-tailscale crashlooping every ~5s: `tailscale up --accept-dns=false --auth-key=... --accept-routes ... use --reset`, then SIGTERM, repeat (from service event buffer).
- User reports pc01 out of the tailnet.
- `tailscale status` confirms pc01 absent; node `Ready`, LAN ping 192.168.68.104 = 0% loss.
- `talosctl -n 192.168.68.104 service ext-tailscale` = STATE Waiting, restart-forever, "failed to auth" + "use --reset".
- Confirmed template already has no `--accept-routes`; found existing matching bead home-systems-6ai.
- Added `TS_EXTRA_ARGS=--reset`; `nostos render talos-pc01` (fresh key) + `nostos apply talos-pc01 --yes` over LAN.
- ext-tailscale → STATE Running (single clean start, 38s, no restart loop); pc01 rejoined tailnet at 100.101.182.40.
- Removed `--reset`; re-applied clean; re-verified Running + on tailnet; template net diff = zero.
- Closed home-systems-6ai; `bd remember pc01-tailnet-reset-fix-2026-07-05`.
