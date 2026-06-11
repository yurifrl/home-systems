## ADDED Requirements

### Requirement: OS-agnostic flash writer
The system SHALL provide an image writer that consumes a `FlashPlan` (a main
image part plus zero or more sidecar parts) and writes it to a destination,
without any knowledge of Talos, Proxmox, machineconfigs, or EEPROM. Exactly one
part SHALL be the main bootable image.

#### Scenario: Writes the main image
- **WHEN** the writer is given a FlashPlan and a destination
- **THEN** it streams the main image part to the destination

#### Scenario: Decompresses xz main image
- **WHEN** the main image part path ends in `.xz`
- **THEN** the writer decompresses it while writing

#### Scenario: Writes sidecars in file mode
- **WHEN** the destination is a file and the FlashPlan contains sidecar parts
- **THEN** each sidecar is written beside the output file

#### Scenario: No OS-specific behavior
- **WHEN** the writer processes a FlashPlan
- **THEN** it performs no Talos- or Proxmox-specific logic; all such content is
  supplied as parts by the OSImage

### Requirement: File and device destinations
The writer SHALL support writing to a regular file or directly to a block
device, preserving the existing confirmation and compression semantics (xz
compression valid only for file destinations).

#### Scenario: Device write rejects compression
- **WHEN** a block-device destination is combined with compression
- **THEN** the writer returns a validation error

#### Scenario: Talos flash plan includes config sidecar
- **WHEN** the talos OSImage produces a FlashPlan for a file destination
- **THEN** the plan includes the machineconfig as a sidecar part and the writer
  emits it beside the image

#### Scenario: Proxmox flash plan is a single ISO part
- **WHEN** the proxmox OSImage produces a FlashPlan
- **THEN** the plan contains exactly one part (the ISO) as the main image and no
  sidecars
