## Why

Adding an offsite node to the cluster today requires 6+ manual steps: EEPROM update, image flash, boot, apply-config, Tailscale route approval, and etcd join debugging. A single command should produce a ready-to-ship disk image that self-joins the cluster when plugged in anywhere with internet access.

## What Changes

- New `nostos ship <node>` command that produces a flashable disk image with machineconfig baked in
- New `nostos image build <node>` subcommand for image generation without flashing
- Modify `nostos build` to download multi-arch assets (arm64 kernel/initramfs for RPi nodes)
- Modify `nostos render` to support embedding config into a disk image (not just writing to state dir)
- Add `--accept-routes` as default for all Tailscale extension configs in new nodes
- Add RPi EEPROM recovery partition to generated images (for RPi nodes)
- New config field `nodes.<name>.serial` for RPi TFTP boot (future PXE support)

## Capabilities

### New Capabilities
- `image-build`: Generate a self-joining Talos disk image for any node, embedding machineconfig + Tailscale auth key. Supports multi-arch (amd64/arm64) and board-specific overlays (RPi EEPROM partition).
- `zero-touch-enroll`: Node boots from image, Tailscale connects, reaches cluster endpoint via tailnet, and joins etcd automatically. No operator intervention after physical power-on.
- `multi-arch-assets`: Build command downloads and caches kernel/initramfs per unique (schematic_id, arch) pair across all configured nodes.

### Modified Capabilities
<!-- No existing specs to modify -->

## Impact

- **Code**: `.submodules/nostos/internal/` — new `image/` package, changes to `pxe/build.go`, `config/` schema, `cli/` commands
- **Config schema**: New optional fields: `nodes.<name>.serial`, default `--accept-routes` in Tailscale extension
- **Dependencies**: Needs `xz` for image compression, `dd` or Go raw disk writer for image assembly
- **Templates**: All node templates should default to `TS_EXTRA_ARGS=--accept-routes`
- **Breaking**: None — additive only
