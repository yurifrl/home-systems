## Context

Nostos currently supports PXE-based provisioning for x86 nodes on the same LAN (`192.168.68.x`). Adding a node at a remote site requires 6+ manual steps: EEPROM config, image flash, boot, apply-config, Tailscale route debugging, and etcd join verification. The tooling assumes co-located operator and nodes.

The cluster already uses Tailscale for cross-site connectivity (dell01 advertises `192.168.68.0/24`, workers accept routes). The missing piece is a single command that produces a "plug and play" disk image for remote deployment.

## Goals / Non-Goals

**Goals:**
- One command (`nostos ship <node>`) produces a flashable image with all config embedded
- Zero manual intervention at the remote site (plug in power + ethernet → joins cluster)
- Support RPi 4/5 (arm64 + EEPROM partition) and x86 (amd64) targets
- Multi-arch asset management in `nostos build`
- Default `--accept-routes` on all nodes for mesh connectivity

**Non-Goals:**
- Full PXE/TFTP support for RPi (deferred — requires serial number, complex TFTP staging)
- Web UI for remote node management (v0.3 topic)
- Automatic hardware detection / inventory
- Support for non-Talos operating systems
- Network boot over WAN (image is pre-flashed, not PXE'd remotely)

## Decisions

### 1. Embed machineconfig into the Talos raw image

**Decision**: Inject the rendered machineconfig into partition 6 (STATE) of the Talos raw disk image at build time, so Talos finds its config on first boot without needing `talosctl apply-config --insecure`.

**Alternatives considered**:
- *Config server*: Run an HTTP server that new nodes fetch config from. Requires internet-reachable endpoint, more moving parts.
- *Apply after boot*: Current approach. Requires operator to know the IP and run a command.

**Rationale**: Baking config in means true zero-touch. Talos supports reading config from the STATE partition on first boot.

### 2. Pre-mint Tailscale auth key at image build time

**Decision**: `nostos ship` mints a Tailscale auth key (via OAuth) and embeds it in the machineconfig within the image. Key is single-use, pre-authorized, tagged.

**Alternatives considered**:
- *Reusable key*: Security risk if image is intercepted.
- *Manual key*: Defeats zero-touch goal.

**Rationale**: Single-use keys are safe even if the image is lost/stolen. OAuth client mints fresh keys per node. Key expiry (90 days default) is fine since the node will auth on first boot.

### 3. RPi EEPROM recovery partition (dual-partition image)

**Decision**: For RPi nodes, produce a hybrid image: partition 1 is a small FAT32 with EEPROM recovery files (to set BOOT_ORDER on first-ever boot), partition 2+ is the standard Talos layout. After EEPROM flash, Pi reboots into Talos from the same card.

**Alternatives considered**:
- *Separate EEPROM step*: Requires two SD cards or two flash cycles.
- *Assume EEPROM is pre-configured*: Can't guarantee for new hardware.

**Rationale**: Single image handles everything. First boot flashes EEPROM (green LED blinks), auto-reboots, second boot loads Talos. Works on fresh-from-box Pi 4s.

### 4. `nostos build` downloads per-arch assets

**Decision**: Iterate all nodes in config.yaml, collect unique `(schematic_id, arch)` pairs, download kernel + initramfs for each. Cache in `~/.local/share/nostos/assets/<schematic>/<arch>/`.

**Rationale**: Current build only handles amd64. Multi-arch is needed for the RPi fleet.

### 5. Default `--accept-routes` on all Tailscale extensions

**Decision**: All node templates include `TS_EXTRA_ARGS=--accept-routes` by default. This enables cross-site mesh routing without per-node manual config.

**Rationale**: Without accept-routes, etcd peers on different subnets can't communicate (as we discovered with rpi01).

## Risks / Trade-offs

- **[Pre-minted key expiry]** → If image isn't used within 90 days, key expires. Mitigation: document this; `nostos ship` warns about expiry window.
- **[Image contains secrets]** → Machineconfig has cluster CA keys. Mitigation: same risk as current rendered configs in `state/configs/`. Document "treat images like secrets".
- **[EEPROM dual-boot complexity]** → First boot does EEPROM flash + reboot. If interrupted, Pi may be in unknown state. Mitigation: EEPROM flash is idempotent; re-flashing from same image is safe.
- **[etcd learner promotion delay]** → Talos doesn't always auto-promote learners quickly. Mitigation: Document expected delay; add `nostos status` check that surfaces learner state.
- **[Disk size assumptions]** → Image is sized for the raw Talos image (~2GB). Target disk must be larger. Mitigation: Talos auto-expands on first boot.
