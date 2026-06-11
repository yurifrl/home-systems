## ADDED Requirements

### Requirement: OS-image registry and selection
The system SHALL provide an `osimage` registry that maps an OS name (e.g.
`talos`, `proxmox`) to an `OSImage` implementation, using the same
self-registration pattern as the provisioner registry. The system SHALL select
a node's `OSImage` via `osimage.For(node)`, defaulting to `talos` when the node
declares no OS.

#### Scenario: Node with no os block defaults to talos
- **WHEN** `osimage.For(node)` is called for a node with no `os:` block
- **THEN** the talos `OSImage` is returned

#### Scenario: Node selects proxmox
- **WHEN** a node declares `os.name: proxmox`
- **THEN** `osimage.For(node)` returns the proxmox `OSImage`

#### Scenario: Unknown OS is rejected
- **WHEN** a node declares an OS name with no registered implementation
- **THEN** `osimage.For(node)` returns a not-found error rather than a nil image

#### Scenario: A new OS registers without touching call sites
- **WHEN** a new `OSImage` implementation is added and registers itself in init
- **THEN** it becomes selectable via `osimage.For(node)` with no changes to
  flash, pxe serve, or render

### Requirement: OSImage packaging contract
The `OSImage` interface SHALL expose packaging operations only: resolve a node's
version selector to a concrete `Ref`; render any node config bytes the OS needs
applied; render the per-MAC netboot iPXE script; and produce a `FlashPlan`. It
SHALL NOT expose install-lifecycle operations (those remain in `provisioner`).

#### Scenario: Talos renders a machineconfig
- **WHEN** `NodeConfig` is called on the talos OSImage for a configured node
- **THEN** it returns the rendered Talos machineconfig bytes

#### Scenario: Proxmox renders no machineconfig
- **WHEN** `NodeConfig` is called on the proxmox OSImage
- **THEN** it returns nil bytes (Proxmox installs no machineconfig)

#### Scenario: Netboot script differs per OS without caller branching
- **WHEN** a caller invokes `NetbootScript` on the selected OSImage
- **THEN** talos returns a kernel+initrd+talos.config script and proxmox returns
  a sanboot-the-ISO script, with the caller performing no OS conditional

### Requirement: Node-level OS configuration
The system SHALL accept a node-level `os:` block with `name` (default `talos`)
and `version`. The system SHALL require `version` when `name` is not `talos` and
validate a Proxmox version as `latest` or a pinned release matching
`^\d+\.\d+-\d+$`, before any network call. The system SHALL NOT accept the
removed `boot.pxe.target`/`boot.pxe.version` fields.

#### Scenario: Proxmox node requires a version
- **WHEN** a node sets `os.name: proxmox` without `os.version`
- **THEN** config validation fails with a validation error (exit code 10)

#### Scenario: Talos node needs no version or template change
- **WHEN** a node omits the `os:` block
- **THEN** it validates as a talos node exactly as before this change

#### Scenario: Malformed pinned proxmox version rejected
- **WHEN** a node sets `os: {name: proxmox, version: "8.x"}`
- **THEN** config validation fails before any download is attempted
