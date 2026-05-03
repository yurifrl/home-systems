# Changelog

## 2026-05-02 First 3080 Debug Session — Dell As New Control Plane Via PXE
- Session ID: 019da35b-d9dc-746e-b542-9e9f1d4b2c1d
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-04-19T01-29-59-004Z_019da35b-d9dc-746e-b542-9e9f1d4b2c1d.jsonl
- Session Name: first 3080 debug session
- Context Name: first 3080 debug session

### Added
- `pxe/` directory with README, `schematic-amd64.yaml` (Image Factory schematic for Tailscale+amd64), `nodes.yaml` (MAC/IP/role registry), and `templates/dell01.yaml` (control-plane machineconfig with `op://` refs reusing the kubernetes vault secrets).
- `scripts/pxe/detect-mac-ip.sh` — picks the Mac's ethernet interface on 192.168.68.0/24 (skips Wi-Fi).
- `scripts/pxe/1-build-assets.sh` — downloads Talos v1.10.3 kernel+initramfs, clones+builds iPXE `snponly.efi` (267KB, under the Dell UEFI TFTP ceiling) with an embedded `dhcp; chain <Mac>:9080/boot.ipxe`, renders top-level `boot.ipxe` referencing the current Mac IP.
- `scripts/pxe/2-render-config.sh` — `op inject`s a node template into `pxe/assets/configs/<mac-hex-hyphens>.yaml` so iPXE `${mac:hexhyp}` fetches the right config.
- `scripts/pxe/3-serve.sh` — starts Python HTTP:9080 + dnsmasq DHCP/TFTP on the detected ethernet; kills stale HTTP on port 9080; fast-fails with a clear error if passwordless sudo isn't set up.
- `taskfiles/pxe.yml` — `task pxe:setup`, `pxe:config NODE=`, `pxe:up`, `pxe:down`, `pxe:status`, `pxe:clean-assets`; wired into root `Taskfile.yml`.
- `docs/pxe-boot.md` — full troubleshooting notes: Dell BIOS settings, Tailscale network-extension interference on macOS, iPXE binary size limits, Secure Boot, Deco router DHCP race.

### Changed
- `talos/controlplane-192.168.68.100.yaml`: `machine.type` flipped from deprecated `init` to `controlplane`.
- `.gitignore`: ignore `pxe/assets/` (downloaded binaries + rendered secret-bearing configs) and `pxe/ipxe-src/` (iPXE build tree).
- `pxe/templates/dell01.yaml`: sanitized comments — removed literal `op://...` substrings that were triggering `op inject` matches.

## 2026-05-02 PXE Boot Script Fixes For macOS dnsmasq
- Session ID: 019de8d9-f5c8-765c-b738-f2c596a458a3
- Session File: /Users/yuri/.pi/agent/sessions/--Users-yuri-Workdir-Yuri-home-systems--/2026-05-02T13-21-31-593Z_019de8d9-f5c8-765c-b738-f2c596a458a3.jsonl
- Session Name: 2026-05-02-1312-pxe-boot-talos-setup
- Context Name: 2026-05-02-1312-pxe-boot-talos-setup

### Changed
- `scripts/pxe/3-serve.sh`: sudo precheck now runs `sudo -n dnsmasq --version` instead of `sudo -n true`, so a NOPASSWD sudoers entry scoped to dnsmasq actually satisfies it.
- `scripts/pxe/3-serve.sh`: TFTP root staged at `/tmp/pxe-tftp` with `ipxe.efi` copied and chmodded 755/644 on every start. Needed because `/Users/yuri` is 0750 and dnsmasq drops privileges to `nobody`, which couldn't traverse into the repo to read `pxe/assets/ipxe.efi`.
- `scripts/pxe/3-serve.sh`: removed per-MAC `--dhcp-host` pinning and the `pxe/nodes.yaml` scrape loop. Added `--dhcp-match=set:pxe,60,PXEClient` and `--dhcp-ignore=tag:!pxe` so dnsmasq only answers PXE clients, not arbitrary LAN devices (avoids fighting the Deco router's DHCP).
- `pxe/nodes.yaml`: dell01 `ip` corrected from `192.168.68.100` (outside the `.200-.210` dhcp-range) to `192.168.68.200`. No longer consumed by `3-serve.sh` but left accurate.

### Added
- `.agents/tmp/pxe-diff.html`: side-by-side HTML diff of the working manual `sudo dnsmasq ...` invocation vs the script-generated invocation used during triage.
