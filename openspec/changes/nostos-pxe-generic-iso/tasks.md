# Implementation Tasks

## 1. Config schema (`internal/config`)

- [x] 1.1 Add a `PXEBoot` struct (`Target`, `Version`) and wire it under the node `Boot` struct as `boot.pxe` in `internal/config/config.go`
- [x] 1.2 Default `Target` to `talos` when `boot.pxe` is absent or `target` is empty (backward compatibility)
- [x] 1.3 Validate: `version` required when `target != talos`; accept `latest` or a value matching `^\d+\.\d+-\d+$`; return a validation error (exit code 10) before any network call
- [x] 1.4 Add config unit tests: absent block → talos; explicit talos; proxmox+latest; proxmox+pinned; proxmox missing version (fails); malformed version (fails)
- [x] 1.5 Surface `boot.pxe.target` and `boot.pxe.version` in `internal/cli/schema/schema.go` and confirm via `nostos schema`

## 2. Proxmox release resolver (`internal/netboot/proxmox`)

- [x] 2.1 Create package `internal/netboot/proxmox` with `Resolve(version) (BootSpec, error)` where `BootSpec` carries `KernelURL`, `InitrdURL`, `ISOURL`, `Cmdline`, `Version`
- [x] 2.2 Implement `latest`: fetch the Proxmox ISO index, parse `proxmox-ve_X.Y-Z.iso`, sort by numeric tuple `(X,Y,Z)`, select newest
- [x] 2.3 Implement pinned: build artifacts for the exact version; return a not-found error when the version is absent from the index
- [x] 2.4 Record the concrete resolved version + ISO sha256 (Proxmox-published checksum) on the result for auditability
- [x] 2.5 Capture a Proxmox index listing as a test fixture; add unit tests for latest selection, numeric (not lexical) ordering, pinned hit, and pinned miss — no live network
- [x] 2.6 Cache the resolved ISO under the existing assets dir so `latest` resolves/downloads once per serve, not per PXE request

## 3. Per-MAC dispatch + generic-ISO boot script (`internal/pxe`)

- [x] 3.1 In `internal/pxe/serve.go`, branch the emitted iPXE script on the resolved node `boot.pxe.target` (default `talos` → existing path unchanged)
- [x] 3.2 Add a generic-ISO (memdisk) iPXE template that loads the resolved Proxmox ISO into RAM and boots the installer; embed any required memdisk asset in `internal/pxe/embed.go`
- [x] 3.3 For `target: proxmox`, call the resolver, then render and serve the memdisk script for that MAC
- [x] 3.4 Emit NDJSON events for the proxmox path (`discover → tftp → iso → boot`) consistent with the existing event tap
- [x] 3.5 Implement installed → boot-local: once a node is known-installed, serve a boot-local/`exit` iPXE script so a stray re-PXE does not reinstall (manual ack acceptable for v1)
- [x] 3.6 Add serve-path tests: a `proxmox` MAC gets the memdisk script, a Talos MAC is unchanged, in the same serve session

## 4. Onboard pc01

- [x] 4.1 Add the `pc01` node to `nostos/config.yaml`: `mac: fc:3c:d7:27:66:17`, `ip: 192.168.68.101`, `role: worker`, `install_disk: /dev/nvme0n1`, `boot.method: pxe`, `boot.pxe.target: proxmox`, `boot.pxe.version: latest`
- [x] 4.2 `nostos render`/validate dry-run for pc01 succeeds and `nostos pxe serve` selects the proxmox path for pc01's MAC

## 5. Live bring-up + docs

- [ ] 5.1 `nostos pxe serve`, power on pc01, confirm it appears in the event log (`discover → tftp`) — validates BIOS Network Stack empirically  _(LIVE HARDWARE — requires physical pc01; run manually)_
- [ ] 5.2 Confirm the Proxmox installer launches and shows the disk picker; install to `/dev/nvme0n1`; reboot and confirm Proxmox boots from disk  _(LIVE HARDWARE — requires physical pc01; run manually)_
- [x] 5.3 Update nostos README/AGENTS with the `boot.pxe.{target,version}` schema, the `latest` behavior, and the pc01 example
