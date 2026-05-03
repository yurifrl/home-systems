## ADDED Requirements

### Requirement: Optional web dashboard
The system SHALL provide a web dashboard via `nostos web` that is opt-in (not started by default) and binds exclusively to loopback (`127.0.0.1`).

#### Scenario: Web command starts FastAPI on loopback
- **WHEN** `nostos web` is run
- **THEN** an HTTP server listens on `127.0.0.1:8080` (default port, configurable) and binding to any non-loopback address is refused

#### Scenario: Web is not started by other commands
- **WHEN** `nostos serve` or any other non-web command is run
- **THEN** no HTTP server on port 8080 is started

### Requirement: Live cluster view
The dashboard SHALL show, for each node in `config.yaml`: name, IP, role, reachability pill (`up`/`down`/`off`/`unknown`), detected Talos version when reachable, and time since last status poll.

#### Scenario: Reachable and unreachable nodes shown
- **WHEN** the dashboard is loaded against a cluster with dell01 running and tp1 offline
- **THEN** dell01 shows an `up` pill with its version; tp1 shows a `down` pill

#### Scenario: Auto-refresh
- **WHEN** the dashboard is open for 30 seconds
- **THEN** the displayed status has refreshed at least once without operator action

### Requirement: Mutation controls with confirmation
The dashboard SHALL allow triggering `wipe`, `bootstrap`, and `config refresh` per-node from the UI, each requiring a typed confirmation (the node name) before executing.

#### Scenario: Wipe requires typed confirmation
- **WHEN** the operator clicks "Wipe" on dell01
- **THEN** a modal appears requiring the operator to type `dell01` before the wipe executes, and pressing Enter without typing the name does not trigger the wipe

#### Scenario: Mutation calls the same CLI code path
- **WHEN** a mutation is triggered from the web UI
- **THEN** the underlying operation is invoked through the same code path as the CLI command (no parallel implementation)

### Requirement: Command cheat-sheet copying
The dashboard SHALL display, for each node, a "Copy install steps" button that copies a per-node boot sequence (BIOS keys, PXE flow, expected outcome) to the operator's clipboard.

#### Scenario: Cheat-sheet contains per-node specifics
- **WHEN** the operator clicks "Copy install steps" on dell01
- **THEN** the clipboard contains instructions that mention dell01's specific MAC, IP, install disk, and the F2/F12 keys for its hardware model (from a small HW database or the node's `hardware:` hint in config.yaml)

### Requirement: Zero auth by design, safe by bind
The dashboard SHALL NOT implement authentication, and SHALL protect itself by binding only to loopback and refusing to start with non-loopback bind arguments unless `--i-know-what-im-doing` is passed.

#### Scenario: Non-loopback bind refused by default
- **WHEN** `nostos web --host 0.0.0.0` is run without the safety flag
- **THEN** the command exits non-zero with a message explaining the loopback-only constraint and the safety flag name

### Requirement: Read-only fallback mode
The dashboard SHALL support `nostos web --read-only` that disables all mutation endpoints and hides mutation UI, for cases where the operator wants to expose the view (e.g., via SSH port-forward) without mutation risk.

#### Scenario: Read-only disables mutations
- **WHEN** `nostos web --read-only` is running and a mutation endpoint (e.g. POST `/api/nodes/dell01/wipe`) is called
- **THEN** the server responds with HTTP 403 and the UI hides all mutation buttons
