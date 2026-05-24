## ADDED Requirements

### Requirement: Provision Talos onto a Turing-Pi-hosted module via the BMC

The system SHALL provide a `tpi` provisioner that installs Talos onto a
Turing-Pi-hosted compute module by flashing the rendered metal image to the
module's eMMC over the board's BMC and then powering the slot on. The
provisioner SHALL be selected by setting `boot.method: tpi` on the node
entry in `nostos/config.yaml`. The provider is not RK1-specific in
principle, but v0.2's hardware targets are Turing Pi RK1 modules
(Rockchip RK3588 SoM, arm64).

#### Scenario: Happy-path install of a Turing-Pi-hosted module

- **WHEN** `nostos node install <name>` runs and the node is configured with
  `boot.method: tpi`, `boot.tpi.host: <BMC>`, `boot.tpi.slot: <1..4>`
- **THEN** the provisioner downloads the `metal-<arch>.raw.xz` for the
  configured `cluster.schematic_id` + `cluster.talos_version`, verifies its
  sha256 against `cluster.image_digests`, decompresses it, calls
  `tpi power off` / `flash` / `power on` against the configured slot, waits
  until `talosctl --insecure -n <node-ip> version` parses, then runs
  `talosctl apply-config -i --file <rendered-config>` against the node IP
- **AND** the orchestrator emits a final event with `Phase=ready,
  Kind=ready` (worker: after `WaitApid`; controlplane: after Bootstrap +
  kubeconfig fetch)

#### Scenario: Install replaces taskfiles/turing.yml

- **WHEN** the operator runs `task nostos:install NODE=tp1`
- **THEN** no command from `taskfiles/turing.yml::flash` /
  `download` / `install-talos` is invoked; `tpi flash` is invoked
  exclusively from inside the `tpi` provisioner; the deprecated Taskfile
  recipes exit 1 with a deprecation message

### Requirement: Image cache is keyed by schematic + version + arch and verified against pinned digests

The provisioner SHALL cache downloaded Talos metal images under
`~/.cache/nostos/images/<schematic_id>/<talos_version>/metal-<arch>.raw.xz`
(directory mode 0700, file mode 0600). Each cached file SHALL be verified
against `cluster.image_digests[<schematic_id>/<version>/<arch>]` in
`nostos/config.yaml`. **Trust-on-first-use is NOT acceptable**: a missing
digest entry SHALL cause `Preflight` to fail closed with a typed error
naming the exact key the operator must add.

#### Scenario: Missing digest fails closed

- **WHEN** `cluster.image_digests` does not contain a key for the install's
  `(schematic_id, version, arch)` tuple
- **THEN** `Preflight` returns an error whose message contains the literal
  required key (e.g. `<schematic>/<version>/arm64`); no HTTP fetch is
  attempted

#### Scenario: First install downloads and verifies

- **WHEN** the cache is empty, the digest is pinned, and `nostos node
  install tp1` runs
- **THEN** the cached file exists at the expected path with mode 0600 and
  matches the pinned sha256

#### Scenario: Second install reuses the cache

- **WHEN** an install runs and the cached file's sha256 matches the pinned
  digest
- **THEN** zero HTTP GETs are issued to factory.talos.dev for that file

#### Scenario: Bad bytes are deleted, not retained

- **WHEN** a download produces a file whose sha256 does not match the
  pinned digest
- **THEN** the file is unlinked; the call returns a typed error citing both
  expected and actual hashes; no retry is attempted in v0.2

### Requirement: Decompression uses a Go-native xz library

The provisioner SHALL decompress `.raw.xz` to `.raw` using
`github.com/ulikunitz/xz`. It MUST NOT shell out to `xz` (no
attacker-controlled filename in argv to a child process).

#### Scenario: No xz subprocess is invoked

- **WHEN** `Prepare` decompresses the cached image
- **THEN** captured subprocess invocations (via the test `Commander`)
  include zero invocations of `xz`

### Requirement: BMC credentials use _ref typed schema and never reach argv or events

The `tpi` provisioner SHALL accept credentials only via `_ref` fields:
`boot.tpi.username_ref`, `boot.tpi.password_ref`,
`boot.tpi.identity_file_ref`. `_ref` fields are typed (`Ref` Go type) with
a custom YAML unmarshaller that REQUIRES one of the URI prefixes `op://`,
`sops://`, or `file://`. The `env://` scheme is **prohibited** for BMC
credentials (process-environment exposure). Resolved password values SHALL
be passed to the `tpi` subprocess via environment variables
(`TPI_USERNAME`, `TPI_PASSWORD`) on `Cmd.Env`, never via argv. Resolved
secret values SHALL never appear in any emitted event.

#### Scenario: Inline password rejected at YAML unmarshal

- **WHEN** `nostos/config.yaml` declares `boot.tpi.password: literal-string`
- **THEN** YAML unmarshal fails with a typed error citing the field path
  `nodes[<name>].boot.tpi.password` and the allowed URI prefixes; no
  validator pass is reached

#### Scenario: env:// scheme rejected for BMC creds

- **WHEN** a `_ref` field is set to `env://SOMEVAR`
- **THEN** YAML unmarshal fails with a typed error stating that `env://` is
  not allowed for credential refs

#### Scenario: Resolved password not present in argv

- **WHEN** the `tpi` subprocess is invoked during `Boot`
- **THEN** the captured argv contains no occurrence of the resolved password
  value; the value appears only in `Cmd.Env`

#### Scenario: Resolved password not present in any emitted event

- **WHEN** an install completes (success or failure)
- **THEN** no event observed on the channel contains the resolved password
  value (Scrubber redaction; see `provisioner` capability)

### Requirement: Identity-file material is securely materialized and removed

If `boot.tpi.identity_file_ref` is set, the provisioner SHALL materialize
the resolved key bytes to a file at
`~/.cache/nostos/secrets/<run-id>/tpi-key` using `O_CREAT|O_EXCL` mode
0600 inside a 0700 dir. The provider SHALL `lstat` the path before opening
to refuse symlinks. The materialized file SHALL be unlinked in `Cleanup`,
including on Ctrl-C paths. Only the path appears in argv; key bytes never
appear in argv or events.

#### Scenario: Key file has correct modes

- **WHEN** Boot materializes the identity file
- **THEN** the file is mode 0600, the parent directory is mode 0700,
  ownership matches the operator uid

#### Scenario: Symlink at target is rejected

- **WHEN** the target path already exists as a symlink (e.g. attacker-
  planted)
- **THEN** materialization returns a typed error; no key bytes are written

#### Scenario: Cleanup unlinks the key file even on Ctrl-C

- **WHEN** the run context is cancelled mid-Boot
- **THEN** Cleanup runs with a fresh context and removes the materialized
  key file; the parent secrets directory is also removed

### Requirement: Contention is serialized per board

The `tpi` provisioner's `ContentionKey(node)` SHALL return
`"tpi:" + node.Boot.TPI.Host`. Two installs targeting different slots on
the same Turing Pi BMC SHALL be serialized; concurrent flashes of two
slots on the same board are not supported. Installs on distinct boards
parallelize when concurrency is enabled (v0.3+).

#### Scenario: Same-board installs serialize

- **WHEN** two `Provisioner` instances are asked for `ContentionKey` for
  nodes sharing `boot.tpi.host`
- **THEN** both return the same non-empty key, and the orchestrator's
  contention map blocks the second install's `Boot` until the first
  releases the key

### Requirement: (host, slot) uniqueness is enforced by the validator

For all nodes with `boot.method: tpi` in `nostos/config.yaml`, the tuple
`(boot.tpi.host, boot.tpi.slot)` SHALL be unique. Config validation rejects
duplicates with an error naming both colliding node entries.

#### Scenario: Duplicate (host, slot) rejected

- **WHEN** two nodes declare `host: 192.168.68.10` and `slot: 1`
- **THEN** config validation fails with an error naming both node names
  and the colliding `(host, slot)` pair

### Requirement: Cleanup powers the slot off after a failed flash

On failure during `Boot` or `WaitMaintenance`, the provisioner SHALL attempt
`tpi power off -n <slot>` during `Cleanup` (single try, 60-second context
deadline) so the operator can retry from a known state. The provisioner
MAY re-acquire its `ContentionKey` before issuing the power-off to avoid
racing a concurrent install on the same board.

#### Scenario: Failed flash leaves slot powered off

- **WHEN** `tpi flash` returns a non-zero exit code during `Boot`
- **THEN** `Cleanup` invokes `tpi power off -n <slot>`; `Install` returns
  the underlying error; the captured argv records the power-off call

#### Scenario: Already-off power-off is non-fatal

- **WHEN** `tpi power off` exits non-zero with stderr matching the pinned
  "already off" pattern (provider-documented)
- **THEN** the provisioner treats the result as success and continues

### Requirement: Preflight checks BMC and tpi version before any side effect

`Preflight` SHALL run before `Prepare` or `Boot` and SHALL verify all of:

- `tpi --version` succeeds AND parses to >= a pinned minimum version
  (placeholder; pinned at implementation time per design D-Open Q1).
- TCP connect to `boot.tpi.host:443` succeeds within 2 seconds.
- Every credential `_ref` resolves through the secrets backend (values
  consumed, never logged).
- The image cache root has at least
  `max(image_size_compressed * 3, 8 GiB)` free.

#### Scenario: BMC unreachable fails fast

- **WHEN** `boot.tpi.host` is unreachable
- **THEN** `Preflight` returns `errors.Is(err, ErrPreflight)` with a
  message naming the host; no image download or flash is attempted

#### Scenario: Missing tpi binary fails clearly

- **WHEN** the `tpi` binary is not on PATH
- **THEN** `Preflight` returns an error explaining `tpi must be installed`
  and pointing to the upstream install docs

#### Scenario: Old tpi version is rejected

- **WHEN** `tpi --version` returns a version below the pinned minimum
- **THEN** `Preflight` returns `ErrPreflight` with a message naming the
  observed version and the required minimum

### Requirement: Tailscale authkey policy

The Tailscale authkey carried by the rendered machineconfig SHALL be
ephemeral, single-use, with TTL <= 1 hour. The operator runbook for
`nostos node install` SHALL rotate the `op://` reference value before each
invocation. v0.2 does NOT automate rotation (deferred to v0.4
`secrets rotate`); the spec records the policy so reinstalls do not silently
re-use a key.

#### Scenario: Rendered machineconfig is unlinked after Apply

- **WHEN** `Apply` returns (success or failure)
- **THEN** the rendered config temp file under
  `~/.cache/nostos/secrets/<run-id>/` no longer exists; the parent secrets
  directory is removed in `Cleanup`

#### Scenario: Rendered machineconfig contents never reach events

- **WHEN** the orchestrator emits any event during the lifecycle
- **THEN** no event message contains the rendered machineconfig bytes
  (Scrubber redaction guarantees this for the resolved-secret subset; the
  provider SHALL NOT debug-print full machineconfig contents at any level)
