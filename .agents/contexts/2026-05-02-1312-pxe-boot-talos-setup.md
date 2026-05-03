---
created: 2026-05-02T13:21:31Z
project: home-systems
description: Debugging and fixing task pxe:up so a dell01 Talos node PXE-boots from a macOS host
context: PXE/TFTP/DHCP bring-up on macOS using dnsmasq + http.server via Taskfile
tags: [pxe, dnsmasq, talos, macos, tftp, dhcp]
session_name: 2026-05-02-1312-pxe-boot-talos-setup
purpose: Make scripts/pxe/3-serve.sh work end-to-end so PXE clients on en5 get an IP, fetch ipxe.efi over TFTP, and chainload boot.ipxe over HTTP
session_id: 019de8d9-f5c8-765c-b738-f2c596a458a3
provider: pi
resume_with: cly agent-session resume --provider pi 2026-05-02-1312-pxe-boot-talos-setup
context_name: 2026-05-02-1312-pxe-boot-talos-setup
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/2026-05-02-1312-pxe-boot-talos-setup.md
---

# Session

- Name: 2026-05-02-1312-pxe-boot-talos-setup
- Purpose: get `task pxe:up` to serve DHCP+TFTP+HTTP for Talos PXE install on dell01
- Resume: `cly agent-session resume --provider pi 2026-05-02-1312-pxe-boot-talos-setup`

# Context

Host: macOS, en5 = 192.168.68.121 (ethernet to switch). Deco router is authoritative DHCP on the LAN. Target: dell01 (mac `d0:94:66:d9:eb:a5`) PXE-booting Talos. Tooling: `task pxe:up` runs `scripts/pxe/3-serve.sh` which starts `python3 -m http.server :9080` on `pxe/assets` and `sudo /opt/homebrew/sbin/dnsmasq` in foreground.

User had a hand-written `sudo dnsmasq ...` command that worked; the Taskfile version did not. Session was spent making the script match reality.

# Problem

The script failed in a chain of distinct ways:

1. `sudo -n true` precheck failed → "sudo requires a password".
2. After NOPASSWD sudoers for dnsmasq only, `sudo -n true` still failed (generic sudo still prompts).
3. dnsmasq bound, then clashed: `Address already in use` from a stale dnsmasq from previous runs.
4. `pxe/nodes.yaml` pinned dell01 to `192.168.68.100`, which is **outside** `--dhcp-range 192.168.68.200,.210`.
5. TFTP: `cannot access .../pxe/assets/ipxe.efi: Permission denied` because dnsmasq drops to `nobody` and `/Users/yuri` is `drwxr-x---` (no world traversal). User's working command used `/tmp/talos-pxe`.
6. User asked why per-MAC pinning was needed; agreed to drop it for simplicity on a home network.

# Decisions

- Precheck changed from `sudo -n true` to `sudo -n "${DNSMASQ}" --version` so the NOPASSWD rule (specific to dnsmasq) actually satisfies the check. Location: `scripts/pxe/3-serve.sh`.
- `pxe/nodes.yaml` dell01 IP changed `192.168.68.100` → `192.168.68.200` (inside dhcp-range). Then later, per-MAC pinning removed entirely.
- TFTP root staged at `/tmp/pxe-tftp` (world-traversable) because `/Users/yuri` is 0750 and blocks `nobody`. Script copies `ipxe.efi` there on every start and `chmod 755/644`.
- Tried `--user=$(id -un) --group=$(id -gn)` for dnsmasq to avoid the drop-to-nobody issue. It did not work on this macOS dnsmasq (still Permission denied). Reverted.
- Dropped `--dhcp-host` pins from `nodes.yaml`. Added `--dhcp-match=set:pxe,60,PXEClient` + `--dhcp-ignore=tag:!pxe` so dnsmasq only answers PXE clients, not random LAN devices.
- NOPASSWD sudoers entry installed by user at `/etc/sudoers.d/pxe-dnsmasq` for `/opt/homebrew/sbin/dnsmasq *`.
- Diff of script-vs-manual rendered at `.agents/tmp/pxe-diff.html` for triage.

# Current State

- `scripts/pxe/3-serve.sh`: updated (sudo check, /tmp/pxe-tftp staging, PXEClient filter, no per-MAC pins).
- `pxe/nodes.yaml`: dell01 ip is 192.168.68.200 (pinning no longer used by the script but value is correct).
- Last observed run (user's terminal, before PXE-filter commit): DHCP OFFER/ACK for dell01 at .200 worked, then TFTP permission denied on `pxe/assets/ipxe.efi`. That is the failure the `/tmp/pxe-tftp` staging + removed MAC pinning fixes.
- User has NOT yet re-run the script after the last two edits (PXE filter + /tmp staging). Pending validation.

# Next Steps

1. Kill stale dnsmasq: `sudo pkill -f dnsmasq`.
2. `task pxe:up` and confirm in logs: `dnsmasq-tftp: sent /tmp/pxe-tftp/ipxe.efi` and then HTTP request on :9080 for `boot.ipxe`.
3. If TFTP still fails, inspect `ls -la /tmp/pxe-tftp` and verify the process is actually reading our dnsmasq (not a brew-service one): `ps aux | grep dnsmasq`.
4. When dell01 boots iPXE and fetches `boot.ipxe`, proceed with Talos install using schematic in `pxe/nodes.yaml` (`schematic_amd64` + `talos_version: v1.10.3`).
