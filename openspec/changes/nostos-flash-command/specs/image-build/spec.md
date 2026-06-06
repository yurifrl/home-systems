## ADDED Requirements

### Requirement: Generate self-joining disk image
The system SHALL produce a raw disk image for a configured node that contains Talos OS with the rendered machineconfig embedded, such that booting from the image requires zero operator intervention to join the cluster.

#### Scenario: Build image for RPi node
- **WHEN** operator runs `nostos flash rpi01 --output /path/to/image.raw`
- **THEN** system produces a raw disk image containing:
  - Talos arm64 with rpi_generic overlay and configured extensions
  - Rendered machineconfig with all secrets injected
  - Pre-minted Tailscale auth key
  - RPi EEPROM recovery partition (BOOT_ORDER=0xf21)

#### Scenario: Build image for x86 node
- **WHEN** operator runs `nostos flash dell02 --output /path/to/image.raw`
- **THEN** system produces a raw disk image containing:
  - Talos amd64 with configured schematic and extensions
  - Rendered machineconfig with all secrets injected
  - Pre-minted Tailscale auth key
  - No EEPROM partition (x86 doesn't need it)

#### Scenario: Flash directly to attached disk
- **WHEN** operator runs `nostos flash rpi01 --device /dev/disk10`
- **THEN** system writes the image directly to the specified block device
- **AND** ejects the device when complete

### Requirement: Image includes valid Tailscale auth key
The system SHALL mint a fresh Tailscale auth key via OAuth during image build and embed it in the machineconfig's Tailscale extension config.

#### Scenario: Key minting during build
- **WHEN** `nostos flash <node>` is invoked
- **THEN** system mints a single-use, pre-authorized Tailscale auth key tagged with the configured tags
- **AND** embeds the key as `TS_AUTHKEY` in the machineconfig

#### Scenario: OAuth credentials unavailable
- **WHEN** `nostos flash <node>` is invoked and Tailscale OAuth credentials cannot be resolved
- **THEN** system exits with exit code 12 (auth_error) and prints guidance

### Requirement: Dry-run support
The system SHALL support `--dry-run` that shows the build plan without producing an image or minting keys.

#### Scenario: Dry run
- **WHEN** operator runs `nostos flash rpi01 --dry-run`
- **THEN** system emits a JSON plan envelope showing all phases (download, render, mint-key, assemble, write)
- **AND** no disk image is created and no Tailscale key is minted

### Requirement: RPi EEPROM recovery partition
For nodes with `arch: arm64` and a `rpi_generic` overlay, the system SHALL prepend an EEPROM recovery partition to the disk image that configures network boot (BOOT_ORDER=0xf21) on first power-on.

#### Scenario: First boot on fresh RPi
- **WHEN** a Pi 4 boots from the image for the first time with unconfigured EEPROM
- **THEN** the EEPROM recovery runs (green LED blinks), sets BOOT_ORDER=0xf21
- **AND** Pi reboots into Talos from the same media

#### Scenario: Subsequent boots skip EEPROM
- **WHEN** a Pi boots from the image after EEPROM is already configured
- **THEN** Talos boots directly without EEPROM recovery running again
