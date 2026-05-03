---
created: 2026-05-02T16:14:08Z
project: home-systems
description: Add Dell OptiPlex 3080M as a new Talos control plane via PXE booted from the Mac
context: Talos homelab cluster recovery + productizing the PXE installer
tags: [talos, pxe, dell-3080m, raspberry-pi, homelab]
session_name: first 3080 debug session
purpose: Replace the dying Raspberry Pi control plane with the Dell 3080M, booted over PXE, and package the whole flow into reusable task/script so it's reproducible in 6 months
session_id: 019da35b-d9dc-746e-b542-9e9f1d4b2c1d
provider: pi
resume_with: cly agent-session resume --provider pi first 3080 debug session
context_name: first 3080 debug session
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/first 3080 debug session.md
---

# First 3080 Debug Session

## Session
- Name: first 3080 debug session
- Purpose: Make the Dell OptiPlex 3080M a Talos control plane via PXE, retire/sideline the flaky rpi, and leave behind `task pxe:*` tooling so it's reproducible.
- Resume: `cly agent-session resume --provider pi first 3080 debug session`

## Context
Home cluster is Talos v1.10.3 on: rpi (192.168.68.100, arm64 control plane, flaky SD card/USB), tp1 (.107), tp4 (.114), pc01 (.104). Secrets live in 1Password personal vault "kubernetes" (account `my.1password.com`, id `QLRNFDHQCNF73METCXTM2DK5BM`). ArgoCD redeploys everything from this git repo. The rpi has a long history of failures: containerd segfault on v1.10.0 (Go 1.24.2 bug), XFS corruption on mmcblk0p6, USB XHCI timeouts, RTC stuck at 1970 because NTP never syncs (suspected Tailscale extension intercepting outbound UDP 123 in favor of a dead tunnel).

## Problem
- Dell 3080M (MAC `d0:94:66:d9:eb:a5`, amd64) has no bootable OS yet. Needs to boot Talos via PXE, install to `/dev/nvme0n1`, and take over as control plane at `.100` (or sit at `.200` if rpi is alive).
- Mac is the only machine that can host the PXE server. Router is TP-Link Deco — no DHCP boot-file options. Mac must be on ethernet on the same switch as the Dell to win the DHCP race.
- Tailscale network extension on the Mac intercepts all TCP to `192.168.68.0/24` because nodes advertise `TS_ROUTES=192.168.68.0/24`. Workaround: `tailscale set --accept-routes=false` (must be re-enabled when away from LAN).
- `boot.ipxe.org` returns HTML instead of EFI binaries → built our own via Docker cross-compile (`alpine:latest`, `make bin-x86_64-efi/snponly.efi EMBED=embed.ipxe`). Dell UEFI rejects files >~256KB over TFTP, so `snponly.efi` (267KB) is the ceiling.
- Dell UEFI ignores proxy-DHCP responses — must run dnsmasq as a full DHCP server scoped to the Dell's MAC (`--dhcp-host`), with `--dhcp-authoritative` to win the race against the Deco.
- Dell BIOS needed: UEFI Boot Path Security = Never, Integrated NIC = Enable w/PXE, Secure Boot = Disabled (otherwise our unsigned iPXE is rejected), Onboard NIC (IPv4) first in boot sequence.

## Decisions
- **Productized the PXE installer under `pxe/` + `scripts/pxe/` + `taskfiles/pxe.yml`** instead of ad-hoc commands.
  - `pxe/nodes.yaml` is the source of truth for MAC → IP → role mapping.
  - `pxe/templates/<node>.yaml` holds raw machine configs with `op://` refs.
  - `scripts/pxe/1-build-assets.sh` downloads kernel+initramfs, clones+builds iPXE, renders `boot.ipxe` with the Mac's current ethernet IP.
  - `scripts/pxe/2-render-config.sh` `op inject`s template → `pxe/assets/configs/<mac-hex-hyphens>.yaml` (matches iPXE `${mac:hexhyp}`).
  - `scripts/pxe/3-serve.sh` starts Python HTTP on 9080 + dnsmasq DHCP/TFTP on ethernet interface; auto-kills stale HTTP on port 9080; fast-fails if passwordless sudo isn't set up.
  - `.gitignore` excludes `pxe/assets/` and `pxe/ipxe-src/` (both contain secrets or are regeneratable).
- **Passwordless sudo** only for `/opt/homebrew/sbin/dnsmasq` in `/etc/sudoers.d/pxe-dnsmasq` so `task pxe:up` runs under the pi `process` tool without prompting.
- **Abandon the rpi** as reliable control plane hardware. Plan: Dell at `.100`, rpi can be retired or become a worker later. Secrets stay unchanged (reused from 1Password), so tp1/tp4/rpi can re-join without regenerating configs.
- Previous session's `talos/controlplane-192.168.68.100.yaml` was flipped from `type: init` to `type: controlplane`. The new `pxe/templates/dell01.yaml` is type `controlplane` targeting `/dev/nvme0n1` with `wipe: true`.

## Current State
- `task pxe:setup` ✅ works. Assets built:
  - `pxe/assets/vmlinuz-amd64` 19M
  - `pxe/assets/initramfs-amd64.xz` 114M
  - `pxe/assets/ipxe.efi` 267K (custom-built snponly with embedded boot script)
  - `pxe/assets/boot.ipxe` 352B (points to current Mac IP; serves `/configs/${mac:hexhyp}.yaml`)
- `task pxe:config NODE=dell01` ✅ works after fixing an `op://` substring in the comments that triggered `op inject`.
  - Rendered: `pxe/assets/configs/d0-94-66-d9-eb-a5.yaml` (32KB, valid for metal mode).
- `task pxe:up` runs with passwordless sudo (process `pxe-up` PID varies). dnsmasq args verified correct: `--dhcp-host=d0:94:66:d9:eb:a5,192.168.68.100`, `--tftp-root=/Users/yuri/Workdir/Yuri/home-systems/pxe/assets`, `--dhcp-boot=tag:ipxe,http://192.168.68.121:9080/boot.ipxe`.
- Dell physical state (as of last photo): booted from its internal NVMe into leftover config from a previous install — hostname `talos-157-p6f`, **v1.9.3**, type **worker** (not control plane), trying to reach rpi at `.100:50001`. Undervoltage warnings. Ports 22/80/443/6443/50000 all refuse.
- rpi state: pingable at `.100`, uptime visible, but clock stuck at 1970 again → TLS certs all appear expired → Talos API unreachable from the Mac. Essentially zombie.
- Tailscale is set to `--accept-routes=false` so Mac can actually reach `192.168.68.0/24` directly.

## Next Steps
1. In Dell BIOS (F2), move **Onboard NIC (IPv4)** above NVMe so the next boot triggers PXE even with leftover disk state.
2. Reboot the Dell. With `task pxe:up` running, verify in logs: `DHCPDISCOVER → DHCPACK → TFTP sent ipxe.efi → HTTP GET boot.ipxe → HTTP GET vmlinuz-amd64 → HTTP GET initramfs-amd64.xz → HTTP GET configs/d0-94-66-d9-eb-a5.yaml`. If the config GET shows up, Talos will wipe the NVMe (`wipe: true`) and install fresh as `dell01` control plane.
3. Once Dell's Talos apid is reachable on `192.168.68.100:50000`, regenerate the admin talosconfig from 1Password secrets:
   - `talosctl gen config talos-default https://192.168.68.100:6443 --with-secrets /tmp/talos-secrets/secrets.yaml --output-types talosconfig --output ~/.talos/config`
4. Bootstrap etcd: **do not** run `talosctl bootstrap` on a node that was already bootstrapped before. If this Dell is the first up in a fresh cluster, run it once; if the cluster already has etcd state, skip.
5. `kubectl get nodes` via `talosctl kubeconfig`; confirm ArgoCD comes back; let it reconcile all apps.
6. Decide rpi fate: flash a fresh v1.10.3 rpi_generic SBC image to a fresh SD card, re-join as worker; or retire.
7. Eventually: commit the new `pxe/`, `scripts/pxe/`, `taskfiles/pxe.yml`, `docs/pxe-boot.md` (already written), and a follow-up `pxe/templates/{tp1,tp4,rpi}.yaml` set once the cluster is healthy.
