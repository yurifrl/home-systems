## ADDED Requirements

### Requirement: Per-node PXE boot target selection
The system SHALL allow each node in `config.yaml` to declare a PXE boot target
via an optional `boot.pxe.target` field with allowed values `talos` and
`proxmox`. When the field is absent, the system SHALL default to `talos` so that
existing node configurations behave exactly as before.

#### Scenario: Node omits boot.pxe block
- **WHEN** `nostos pxe serve` resolves a booting MAC to a node that has no
  `boot.pxe` block
- **THEN** the system emits the existing Talos iPXE script for that node

#### Scenario: Node declares target talos explicitly
- **WHEN** a node sets `boot.pxe.target: talos`
- **THEN** the system emits the Talos iPXE script, identical to the default

#### Scenario: Node declares target proxmox
- **WHEN** a node sets `boot.pxe.target: proxmox`
- **THEN** the system emits a generic-ISO iPXE script that launches the Proxmox
  installer instead of the Talos boot script

### Requirement: PXE version field and validation
The system SHALL require a `boot.pxe.version` value when `boot.pxe.target` is
not `talos`, accepting either the literal `latest` or a pinned release matching
the pattern `^\d+\.\d+-\d+$`. Validation SHALL occur before any network call.

#### Scenario: Missing version for non-talos target
- **WHEN** a node sets `boot.pxe.target: proxmox` without `boot.pxe.version`
- **THEN** config validation fails with a validation error (exit code 10) and no
  network call is made

#### Scenario: Malformed pinned version
- **WHEN** a node sets `boot.pxe.version: "8.x"` (not matching the pinned
  pattern and not `latest`)
- **THEN** config validation fails with a validation error before any download
  is attempted

#### Scenario: Valid pinned version
- **WHEN** a node sets `boot.pxe.version: "8.3-1"`
- **THEN** validation passes and the value is carried to the resolver

### Requirement: Per-MAC dispatch under a single serve
The system SHALL serve a mixed fleet under one `nostos pxe serve` invocation,
selecting the iPXE script per booting MAC according to that node's resolved boot
target, without requiring separate commands or daemons per target.

#### Scenario: Mixed fleet boots concurrently
- **WHEN** a Talos node and a `proxmox` node PXE-boot against the same running
  `nostos pxe serve`
- **THEN** the Talos node receives the Talos script and the `proxmox` node
  receives the generic-ISO script, each keyed by its MAC

### Requirement: Generic-ISO boot via memdisk
The system SHALL launch the Proxmox installer by serving a generic-ISO iPXE
script that loads the resolved ISO into memory (memdisk) and boots it.

#### Scenario: Proxmox installer launches
- **WHEN** a `proxmox` node PXE-boots and the resolver has produced a boot spec
- **THEN** the emitted iPXE script loads the Proxmox ISO into RAM and boots the
  installer, which presents its interactive disk picker

### Requirement: Schema surfacing of PXE fields
The system SHALL surface the `boot.pxe.target` and `boot.pxe.version` fields
through `nostos schema` so clients and operators can discover them.

#### Scenario: Schema lists the new fields
- **WHEN** an operator runs `nostos schema` for the node config
- **THEN** the output includes `boot.pxe.target` and `boot.pxe.version` with
  their allowed values
