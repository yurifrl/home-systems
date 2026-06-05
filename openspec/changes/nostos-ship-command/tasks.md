## 1. Multi-Arch Asset Management

- [ ] 1.1 Refactor `internal/pxe/build.go` to iterate all nodes and collect unique (schematic_id, arch) pairs
- [ ] 1.2 Download kernel + initramfs per pair, cache under `~/.local/share/nostos/assets/<schematic>/<arch>/`
- [ ] 1.3 Add RPi firmware download (start4.elf, fixup4.dat) for arm64 rpi_generic nodes
- [ ] 1.4 Add Talos raw image download (metal-<arch>.raw.xz) per (schematic, arch) pair
- [ ] 1.5 Update `nostos build` CLI to report multi-arch progress

## 2. Config Schema Changes

- [ ] 2.1 Add optional `serial` field to node config schema (for future RPi PXE/TFTP)
- [ ] 2.2 Add `overlay` field to node config (e.g., `rpi_generic`) for detecting RPi-specific image assembly
- [ ] 2.3 Validate new fields in `internal/config/` with existing validator patterns

## 3. Image Assembly Package

- [ ] 3.1 Create `internal/image/` package with `Builder` struct
- [ ] 3.2 Implement base image extraction (decompress .raw.xz)
- [ ] 3.3 Implement machineconfig injection into Talos STATE partition (partition 6)
- [ ] 3.4 Implement RPi EEPROM recovery partition prepend (FAT32 with recovery.bin, pieeprom.bin, boot.conf, start4.elf, fixup4.dat)
- [ ] 3.5 Implement output modes: write to file (`--output`) or write to device (`--device`)
- [ ] 3.6 Add image compression option (xz output for file mode)

## 4. Ship Command CLI

- [ ] 4.1 Create `internal/cli/ship.go` cobra command with flags: `--output`, `--device`, `--dry-run`, `--yes`
- [ ] 4.2 Implement orchestration: build assets → render config → mint Tailscale key → assemble image → write
- [ ] 4.3 Implement `--dry-run` with JSON plan envelope matching nostos conventions
- [ ] 4.4 Add confirmation prompt before writing to device (skip with `--yes`)
- [ ] 4.5 Register command in root command and update `nostos schema`

## 5. Zero-Touch Enrollment Defaults

- [ ] 5.1 Update all node templates to include `TS_EXTRA_ARGS=--accept-routes` by default
- [ ] 5.2 Update `nostos render` to warn if a node template is missing accept-routes
- [ ] 5.3 Document the zero-touch enrollment flow in nostos README

## 6. Network Detection Improvements

- [ ] 6.1 Remove `192.168.68.x` hardcoding in `internal/pxe/serve.go` detectNetwork/ipForInterface
- [ ] 6.2 Accept any private subnet (10.x, 172.16-31.x, 192.168.x) or use `--iface` to determine
- [ ] 6.3 Update PXE server defaults (gateway, DHCP range) to derive from detected interface

## 7. Testing & Validation

- [ ] 7.1 Unit tests for `internal/image/` package (partition manipulation, config injection)
- [ ] 7.2 Integration test: `nostos ship --dry-run` produces valid plan JSON
- [ ] 7.3 Test multi-arch build downloads correct assets for mixed config
- [ ] 7.4 Manual end-to-end test: ship an image, boot a Pi, verify cluster join

## 8. Documentation

- [ ] 8.1 Update `.submodules/nostos/README.md` with `ship` command usage
- [ ] 8.2 Update `nostos/README.md` (data dir) with rpi01 node documentation
- [ ] 8.3 Add `docs/remote-node.md` guide covering the zero-touch workflow
- [ ] 8.4 Update `AGENTS.md` with `ship` command invariants and idempotency table
