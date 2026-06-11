## ADDED Requirements

### Requirement: Resolve latest Proxmox version
The system SHALL resolve `version: latest` to the newest available Proxmox VE
release by discovering published `proxmox-ve_X.Y-Z.iso` artifacts and selecting
the highest version ordered by the numeric tuple `(X, Y, Z)`.

#### Scenario: Latest selects the highest version
- **WHEN** the Proxmox index lists `proxmox-ve_8.2-2.iso` and
  `proxmox-ve_8.3-1.iso` and the node requests `version: latest`
- **THEN** the resolver selects `8.3-1` and produces boot artifacts for it

#### Scenario: Latest ordering is numeric not lexical
- **WHEN** the index lists `proxmox-ve_8.9-1.iso` and `proxmox-ve_8.10-1.iso`
- **THEN** the resolver selects `8.10-1` (numeric tuple ordering, not string
  comparison)

### Requirement: Resolve pinned Proxmox version
The system SHALL resolve a pinned `version` (e.g. `8.3-1`) directly to its boot
artifacts using nostos's knowledge of the Proxmox download layout, without
needing the operator to supply any URL.

#### Scenario: Pinned version resolves directly
- **WHEN** a node requests `version: "8.3-1"`
- **THEN** the resolver produces boot artifacts for exactly `8.3-1`

#### Scenario: Pinned version not found
- **WHEN** a node requests a pinned version that does not exist in the Proxmox
  index
- **THEN** the resolver returns a not-found error rather than producing an
  invalid boot spec

### Requirement: Record resolved version and checksum
The system SHALL record the concrete resolved version and the checksum of the
resolved ISO so that a `latest` resolution is auditable and reproducible after
the fact.

#### Scenario: Resolved version is recorded for latest
- **WHEN** `version: latest` resolves to a concrete release
- **THEN** the concrete version and the ISO checksum are recorded/emitted so the
  operator can later pin the same release

### Requirement: Offline-testable resolution
The system SHALL allow resolution logic to be exercised in tests against a
captured Proxmox index fixture without performing any live network request.

#### Scenario: Resolver tested against fixture
- **WHEN** the resolver's tests run
- **THEN** they parse a captured index fixture and assert selection/ordering
  behavior without contacting download.proxmox.com
