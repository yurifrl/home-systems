## ADDED Requirements

### Requirement: Provisioner interface decouples install method from orchestrator

The system SHALL define a Go interface in `internal/provisioner/` with five
lifecycle hooks (`Preflight`, `Prepare`, `Boot`, `WaitMaintenance`, `Apply`)
plus an always-called `Cleanup`. The orchestrator
(`internal/cluster/orchestrate.go::Install`) SHALL invoke only these hooks;
it MUST NOT contain method-specific code paths (no PXE-only branches, no
tpi-only branches).

The v0.2 method enum is **{pxe, tpi}**. Other methods land in their own
openspec changes; v0.2 does NOT reserve enum entries or sub-block schema
bytes for unimplemented providers.

#### Scenario: Adding a new boot method requires no orchestrator change

- **WHEN** a future provisioner package implements the interface and registers
  itself in `init()`
- **THEN** building nostos with that package blank-imported makes the new
  method available, with no edits required in
  `internal/cluster/orchestrate.go`

#### Scenario: Existing PXE flow goes through the interface

- **WHEN** `nostos node install dell01` runs with `boot.method` defaulted to
  `pxe`
- **THEN** the orchestrator invokes `Preflight`, `Prepare`, `Boot`,
  `WaitMaintenance`, `Apply`, `WaitApid`, `Bootstrap` (controlplane only),
  in that order, followed by `Cleanup` in a deferred block
- **AND** the PXE provider's `Apply` is a no-op (config delivered in-band
  via the iPXE chain); the orchestrator does NOT additionally invoke
  `talosctl apply-config -i`

#### Scenario: Unimplemented method fails closed

- **WHEN** `nostos/config.yaml` declares `boot.method: redfish` (not in the
  v0.2 enum)
- **THEN** config validation rejects the value with a typed error naming
  the supported set `{pxe, tpi}` and citing the field path

### Requirement: Cleanup runs on every termination path with a fresh context

The system SHALL invoke `Provisioner.Cleanup` exactly once per install run,
regardless of whether earlier phases succeeded, returned an error, or the
parent context was cancelled. Cleanup SHALL receive a context derived from
`context.Background` with a 60-second deadline (NOT the run context), so it
can complete tear-down work after Ctrl-C. If the provisioner declared a
non-empty `ContentionKey`, Cleanup MAY re-acquire the same key before
issuing destructive teardown commands.

#### Scenario: Cleanup runs after a phase error

- **WHEN** `Provisioner.Boot` returns a non-nil error
- **THEN** `Provisioner.Cleanup` is invoked before `Install` returns; the
  orchestrator emits a single error event followed by no further progress
  events

#### Scenario: Cleanup runs after Ctrl-C

- **WHEN** the operator sends SIGINT during `Provisioner.WaitMaintenance`
- **THEN** the orchestrator calls `Cleanup` with a non-cancelled context;
  `Cleanup` returns within its 60-second deadline

#### Scenario: Cleanup is idempotent

- **WHEN** `Cleanup` is called twice on the same provisioner instance
- **THEN** the second call returns nil and produces no observable side
  effects beyond the first

### Requirement: Provisioner registry rejects duplicate methods

The system SHALL maintain a registry mapping method strings to constructor
functions. Registering two providers under the same method string SHALL
panic at program startup with a message naming the conflicting method.

#### Scenario: Duplicate registration panics

- **WHEN** a process imports two packages that both call
  `provisioner.Register("tpi", ...)`
- **THEN** the second `Register` call panics; the recovered message contains
  the literal string `"tpi"`

#### Scenario: Unknown method returns a typed error

- **WHEN** the orchestrator looks up an unregistered method
- **THEN** `provisioner.New` returns an error matching
  `errors.Is(err, provisioner.ErrNotRegistered)`

### Requirement: ContentionKey serializes shared-resource installs

Each `Provisioner` SHALL expose `ContentionKey(node)` returning a string.
Two installs whose `ContentionKey` returns the same non-empty value SHALL
be serialized internally; an empty key means no contention. The
orchestrator SHALL acquire the per-key lock before `Boot` and release it
after `Apply` (or in `Cleanup` when an earlier phase failed). The lock is
in-process; cross-process serialization for the same node is provided
separately by a per-node flock (see "Per-node flock" requirement).

#### Scenario: Two slots on one Turing Pi serialize

- **WHEN** two installs target nodes whose `ContentionKey` both return
  `"tpi:192.168.68.10"`
- **THEN** the second install's `Boot` blocks until the first install
  releases the key

#### Scenario: Distinct keys parallelize

- **WHEN** two installs return `ContentionKey` values that differ
- **THEN** both `Boot` phases run concurrently

#### Scenario: PXE server is single-threaded

- **WHEN** the `pxe` provisioner is asked for its `ContentionKey`
- **THEN** it returns `"pxe:server"` (a non-empty key), so concurrent PXE
  installs serialize on the single dnsmasq + 9080 binding

### Requirement: Provisioner Apply hook owns config delivery

The orchestrator SHALL call `Provisioner.Apply(ctx, node, configPath, emit)`
after `WaitMaintenance` succeeds. Each provisioner declares delivery
semantics in its `Apply` body:

- **In-band** providers (PXE) return nil immediately because the rendered
  config was delivered during the boot chain.
- **Out-of-band** providers (tpi, future redfish/proxmox/usb) invoke
  `talosctl apply-config -i --file <configPath>` against the node IP.

The orchestrator MUST NOT invoke `talosctl apply-config -i` itself.
`configPath` is a 0600 temp file under `~/.cache/nostos/secrets/<run-id>/`
owned by the orchestrator and unlinked after `Apply` returns (success or
failure).

#### Scenario: PXE Apply does not invoke talosctl apply-config

- **WHEN** `nostos node install dell01` completes the PXE flow
- **THEN** captured subprocess invocations (via the test `Commander`) include
  ZERO `talosctl apply-config -i` calls

#### Scenario: tpi Apply invokes talosctl apply-config exactly once

- **WHEN** `nostos node install tp1` completes the tpi flow
- **THEN** captured subprocess invocations include exactly one
  `talosctl apply-config -i -n <ip> --file <path>` call where `<path>` is
  the 0600 temp file owned by the orchestrator

#### Scenario: Rendered config file is unlinked after Apply

- **WHEN** `Apply` returns (success or error)
- **THEN** the rendered config temp file no longer exists on disk

### Requirement: WaitMaintenance verifies Talos identity

`WaitMaintenance` SHALL succeed only when `talosctl --insecure -n <ip>
version` returns a parseable Talos version response within the orchestrator-
supplied deadline. A raw TCP connect to port 50000 SHALL NOT be sufficient.

#### Scenario: TCP-only listener does not satisfy WaitMaintenance

- **WHEN** the target IP accepts TCP on 50000 but does not return a valid
  Talos version response
- **THEN** `WaitMaintenance` continues polling until the deadline expires
  and returns `provisioner.ErrTimeout`

#### Scenario: Authenticated maintenance probe succeeds

- **WHEN** `talosctl --insecure -n <ip> version` returns a parseable
  response
- **THEN** `WaitMaintenance` returns nil

### Requirement: Resolved-secret values never reach emitted events

The orchestrator SHALL wrap the raw event channel with a `Scrubber` seeded
once per run with all resolved-secret values. Every provider emit and every
captured subprocess stdout/stderr SHALL pass through the Scrubber before
reaching the event sink. Provider-side discretion is not part of the
redaction story.

#### Scenario: Planted secret is redacted

- **WHEN** a resolved secret value is `S3CR3T-DO-NOT-LEAK` and a provider
  emits a message containing that substring
- **THEN** an observer of the event channel never sees the substring; the
  emitted message contains a redaction marker (e.g. `[REDACTED]`) in its
  place

#### Scenario: Subprocess output is scrubbed

- **WHEN** a child process (e.g. `tpi`) writes a resolved secret value to
  its stdout
- **THEN** the captured chunk passes through the Scrubber before `emit()`
  is called; the substring does not appear in any emitted event

### Requirement: Per-node flock serializes concurrent local invocations

The orchestrator SHALL acquire an exclusive non-blocking flock on
`nostos/state/configs/<name>.lock` (created mode 0600) at the top of
`Install`, held until return. A second invocation for the same node from
the same workstation SHALL fail fast with a typed `ErrLocked` whose message
names the lockfile path. Cross-host concurrent operators are out of scope;
the symptom (last writer wins; potentially destructive race) is documented
as a known gap.

#### Scenario: Second concurrent install of same node aborts fast

- **WHEN** one `nostos node install tp1` is already running and holds the
  flock
- **THEN** a second `nostos node install tp1` returns
  `errors.Is(err, ErrLocked)` within 100ms; stderr cites the lockfile path

### Requirement: Live-node reinstall is opt-in

Before `Prepare`, the orchestrator SHALL probe the target IP with
`talosctl version`. If the node responds as Ready (apid responds over the
secured listener; for controlplane: + etcd healthy), `Install` SHALL return
`ErrNodeAlreadyReady` unless `--reinstall` was passed. The default
`nostos node install` invocation requires interactive confirmation OR the
explicit `--yes` flag for any destructive provider (e.g. tpi); the
confirmation prompt SHALL show a 5-second cancellable banner naming the
node, IP, and boot method.

#### Scenario: Healthy node refuses reinstall by default

- **WHEN** `nostos node install tp1` is invoked and tp1 is currently a
  healthy cluster member
- **THEN** `Install` returns `errors.Is(err, ErrNodeAlreadyReady)`; no
  provider hook beyond `Preflight` is invoked

#### Scenario: --reinstall bypasses the live-node guard

- **WHEN** `nostos node install tp1 --reinstall --yes` is invoked
- **THEN** the orchestrator proceeds through the full lifecycle without
  prompting

### Requirement: NodeView is not a separate type; providers consume `*config.Node`

Provisioners SHALL accept `*config.Node` directly. The package
`internal/provisioner/` MAY import `internal/config` (read-only); it MUST
NOT import `internal/cluster`.

#### Scenario: Providers compile without orchestrator imports

- **WHEN** `go list -deps ./.submodules/nostos/internal/provisioner/tpi` is
  run
- **THEN** the dependency graph contains `internal/config` and
  `internal/provisioner` but NOT `internal/cluster`
