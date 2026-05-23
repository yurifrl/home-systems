## ADDED Requirements

### Requirement: Provisioner interface decouples install method from orchestrator
The system SHALL define a Go interface in internal/provisioner with five lifecycle hooks (Preflight, Prepare, Boot, WaitMaintenance, Cleanup). The orchestrator (internal/cluster/orchestrate.go::Install) SHALL invoke only these hooks; it MUST NOT contain method-specific code paths (no PXE-only branches, no tpi-only branches, no Proxmox-only branches).

#### Scenario: Adding a new boot method requires no orchestrator change
- **WHEN** a new provisioner package is added that implements the interface and registers itself in init()
- **THEN** building nostos with that package blank-imported makes the new method available via boot.method=<new>, with no edits required in internal/cluster/orchestrate.go

#### Scenario: Existing PXE flow goes through the interface
- **WHEN** nostos node install dell01 runs with boot.method defaulted to pxe
- **THEN** the orchestrator calls Preflight, Prepare, Boot, WaitMaintenance, ApplyConfigInsecure, WaitApid, Bootstrap (controlplane), then Cleanup, in that order

### Requirement: Cleanup runs on every termination path
The system SHALL invoke Provisioner.Cleanup exactly once per install run, regardless of whether earlier phases succeeded, returned an error, or the parent context was cancelled. Cleanup SHALL receive a fresh context not derived from the cancelled run context, so it can complete tear-down work even after Ctrl-C.

#### Scenario: Cleanup runs after a phase error
- **WHEN** Provisioner.Boot returns a non-nil error
- **THEN** Provisioner.Cleanup is called before Install returns, and the orchestrator emits a single error event followed by no further progress events

#### Scenario: Cleanup runs after Ctrl-C
- **WHEN** the operator sends SIGINT during Provisioner.WaitMaintenance
- **THEN** the orchestrator calls Cleanup with a non-cancelled context, and Cleanup completes within its own timeout (default 30s)

### Requirement: Provisioner registry rejects duplicate methods
The system SHALL maintain a registry mapping method strings (pxe, tpi, redfish, proxmox, usb, rpi-imager) to constructor functions. Registering two providers under the same method string SHALL panic at program startup.

#### Scenario: Duplicate registration panics
- **WHEN** a process imports two packages that both call provisioner.Register("tpi", ...)
- **THEN** the second Register call panics with a message naming the conflicting method

#### Scenario: Unknown method returns a typed error
- **WHEN** the orchestrator looks up an unregistered method (e.g. boot.method=fictional)
- **THEN** provisioner.New returns an error matching errors.Is(err, provisioner.ErrUnknownMethod)

### Requirement: BMC contention key serializes hardware-shared installs
Each Provisioner SHALL expose BMCKey(NodeView) returning a string. Two installs whose BMCKey returns the same non-empty value SHALL be serialized internally; an empty key means no contention. The orchestrator SHALL acquire and release a per-key mutex around the Boot phase.

#### Scenario: Two slots on one Turing Pi serialize
- **WHEN** nostos node install --parallel 2 starts installs for tp1 and tp4 (same BMC host)
- **THEN** the second install blocks at Boot acquisition until the first install releases its key

#### Scenario: Distinct BMCs parallelize
- **WHEN** two installs target nodes on different BMC hosts
- **THEN** both Boot phases run concurrently

### Requirement: NodeView is the only data passed to providers
Provisioners SHALL receive an immutable NodeView (name, MAC, IP, role, arch, install_disk, template path, BootConfig) plus orchestrator-supplied dependencies (resolved secrets, paths, mockable Commander). Providers SHALL NOT import internal/config or internal/cluster.

#### Scenario: Providers compile without orchestrator imports
- **WHEN** go list -deps is run on internal/provisioner/tpi
- **THEN** the dependency graph contains internal/provisioner but not internal/cluster

### Requirement: Install events are journaled to a per-run JSONL log
Every Event emitted during a Provisioner-driven install SHALL also be appended to ~/.local/state/nostos/runs/<run-id>.jsonl. Each line is a JSON object with run_id, ts (RFC3339), node, kind, phase, and message. Resolved secret values SHALL never appear in the log.

#### Scenario: Run log captures install events
- **WHEN** an install completes successfully
- **THEN** the JSONL file at ~/.local/state/nostos/runs/<run-id>.jsonl exists, contains one line per emitted Event, and the last line has kind=ready

#### Scenario: Secrets never reach the run log
- **WHEN** an install resolves an op:// reference whose value is the literal string "S3CR3T"
- **THEN** no line in the run log contains the substring "S3CR3T"
