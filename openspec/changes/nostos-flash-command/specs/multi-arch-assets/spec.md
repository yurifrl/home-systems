## ADDED Requirements

### Requirement: Build downloads assets for all architectures
`nostos build` SHALL download kernel and initramfs for every unique (schematic_id, arch) pair found across all nodes in config.yaml.

#### Scenario: Config has arm64 and amd64 nodes
- **WHEN** config.yaml contains dell01 (amd64, schematic A) and rpi01 (arm64, schematic B)
- **AND** operator runs `nostos build`
- **THEN** system downloads vmlinuz-amd64 + initramfs-amd64.xz for schematic A
- **AND** downloads vmlinuz-arm64 + initramfs-arm64.xz for schematic B
- **AND** caches both under `~/.local/share/nostos/assets/<schematic>/<arch>/`

#### Scenario: Assets already cached
- **WHEN** assets for a (schematic, arch) pair already exist in the cache
- **AND** operator runs `nostos build`
- **THEN** system skips download for that pair and reports "up to date"

### Requirement: Download RPi firmware for arm64 RPi nodes
For nodes with arch=arm64 and rpi_generic overlay, `nostos build` SHALL download `start4.elf` and `fixup4.dat` from the Raspberry Pi firmware repository.

#### Scenario: RPi firmware download
- **WHEN** config.yaml contains rpi01 with arch=arm64 and rpi_generic overlay
- **AND** operator runs `nostos build`
- **THEN** system downloads start4.elf and fixup4.dat
- **AND** caches them under `~/.local/share/nostos/assets/rpi-firmware/`

### Requirement: Download Talos raw image for ship command
`nostos build` or `nostos flash` SHALL download the Talos raw disk image (metal-arm64.raw.xz or metal-amd64.raw.xz) for the node's schematic when needed for image generation.

#### Scenario: Raw image download on ship
- **WHEN** operator runs `nostos flash rpi01` and the raw image is not cached
- **THEN** system downloads `metal-arm64.raw.xz` from factory.talos.dev for the node's schematic
- **AND** caches it for future use
- **AND** uses it as the base for the assembled image
