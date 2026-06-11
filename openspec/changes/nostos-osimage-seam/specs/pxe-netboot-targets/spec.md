## MODIFIED Requirements

### Requirement: Per-node OS boot selection
The system SHALL select what a PXE-booting node boots from the node-level `os:`
block (not `boot.pxe.target`), routed through `osimage.For(node)`. A node with
no `os:` block boots Talos (unchanged default). A node with `os.name: proxmox`
boots the Proxmox installer. Dispatch SHALL be performed by calling
`NetbootScript` on the selected `OSImage`, with no `if proxmox`/`if talos`
branch in the serve path.

#### Scenario: Node omits os block boots talos
- **WHEN** `nostos pxe serve` resolves a booting MAC to a node with no `os:`
  block
- **THEN** it serves the Talos iPXE script via the talos OSImage

#### Scenario: Node selects proxmox boots installer
- **WHEN** a node sets `os.name: proxmox`
- **THEN** the serve path obtains its script from the proxmox OSImage
  (sanboot the resolved ISO)

#### Scenario: Mixed fleet under one serve
- **WHEN** a talos node and a proxmox node PXE-boot against the same running
  `nostos pxe serve`
- **THEN** each receives its OS's script via `osimage.For(node)`, keyed by MAC,
  with no per-OS conditional in the serve handler

### Requirement: PXE version selection moves to the os block
The system SHALL read the Proxmox version selector from `os.version` (not
`boot.pxe.version`), accepting `latest` or a pinned release, resolved by the
proxmox OSImage. The removed `boot.pxe.target` and `boot.pxe.version` fields
SHALL no longer be accepted.

#### Scenario: Latest resolved via os.version
- **WHEN** a proxmox node sets `os.version: latest`
- **THEN** the proxmox OSImage resolves the newest release and the serve path
  serves it

#### Scenario: Legacy boot.pxe.target rejected
- **WHEN** a node still declares `boot.pxe.target`
- **THEN** config loading rejects the unknown field (the OS choice now lives
  under `os:`)

## REMOVED Requirements

### Requirement: Per-node PXE boot target selection
**Reason**: Replaced by node-level `os:` selection routed through the `osimage`
registry; the OS choice is orthogonal to the PXE transport and must not live
under `boot.pxe`.
**Migration**: Replace `boot.pxe.target: <os>` / `boot.pxe.version: <v>` with
`os: {name: <os>, version: <v>}` at the node level. Nodes with no `os:` block
default to talos.

### Requirement: PXE version field and validation
**Reason**: Version validation moves to the node-level `os.version` field
(see osimage-registry "Node-level OS configuration").
**Migration**: Move `boot.pxe.version` to `os.version`; same `latest` / pinned
`^\d+\.\d+-\d+$` rules apply.

### Requirement: Schema surfacing of PXE fields
**Reason**: `boot.pxe.target`/`boot.pxe.version` are removed; the documented
surface becomes the node-level `os:` block.
**Migration**: Consult `os.name` / `os.version` in `nostos schema` and the
README instead of the old PXE fields.
