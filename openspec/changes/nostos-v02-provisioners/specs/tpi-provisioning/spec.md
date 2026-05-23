## ADDED Requirements

### Requirement: Provision Talos onto a Turing Pi RK1 module via the BMC
The system SHALL provide a tpi provisioner that installs Talos onto a Turing Pi-hosted compute module by flashing the rendered metal image to the modules eMMC over the boards BMC and then powering the slot on. The provisioner SHALL be selected by setting boot.method=tpi on the node entry in nostos/config.yaml.

#### Scenario: Happy-path install of an RK1 module
- **WHEN** nostos node install tp1 runs and tp1 is configured with boot.method=tpi, boot.tpi.host=192.168.68.10, boot.tpi.slot=1
- **THEN** the provisioner downloads the metal-arm64.raw.xz for the configured schematic_id + talos_version, decompresses it, calls tpi power off / flash / power on against slot 1, waits for apid at 192.168.68.107:50000, hands off to talosctl apply-config -i, and the orchestrator emits a final ready event

#### Scenario: Install replaces taskfiles/turing.yml
- **WHEN** the operator runs task nostos:install NODE=tp1
- **THEN** no command from taskfiles/turing.yml is invoked directly; tpi flash is invoked exclusively from inside the tpi provisioner

### Requirement: Image cache is keyed by schematic + version + arch
The provisioner SHALL cache downloaded Talos metal images under ~/.cache/nostos/images/<schematic_id>/<talos_version>/metal-<arch>.raw.xz and skip re-download when the cached file matches the expected size and sha256.

#### Scenario: First install downloads
- **WHEN** the cache is empty and nostos node install tp1 runs
- **THEN** ~/.cache/nostos/images/<schematic>/<version>/metal-arm64.raw.xz is created and matches the upstream factory.talos.dev sha256

#### Scenario: Second install reuses the cache
- **WHEN** an install runs and the cached image matches expected sha256
- **THEN** no HTTP GET is issued to factory.talos.dev and the Prepare phase completes in under 2 seconds (excluding decompression)

#### Scenario: Schematic change invalidates cache
- **WHEN** cluster.schematic_id changes in nostos/config.yaml
- **THEN** the next install downloads a fresh image under the new schematic directory; old schematic images are not deleted automatically

### Requirement: BMC credentials are resolved through the secrets backend, never inline
The tpi provisioner SHALL accept credentials only via _ref fields (boot.tpi.username_ref, boot.tpi.password_ref, boot.tpi.identity_file_ref). The validator SHALL reject inline values. Resolved secrets SHALL be passed to the tpi subprocess via environment variables (TPI_USERNAME, TPI_PASSWORD), not via argv.

#### Scenario: Inline password rejected
- **WHEN** nostos/config.yaml declares boot.tpi.password: literal-string
- **THEN** nostos node list / install / show fails validation with an error referencing the field name and recommending the _ref form

#### Scenario: Resolved password not present in argv
- **WHEN** the tpi subprocess is invoked during Boot
- **THEN** the captured argv (recordable via the Commander mock in tests) contains no occurrence of the resolved password value; the value appears only in the subprocess environment

#### Scenario: Resolved password not present in run log
- **WHEN** an install completes
- **THEN** ~/.local/state/nostos/runs/<run-id>.jsonl contains no occurrence of the resolved password value

### Requirement: BMC contention is serialized per board
Two installs targeting different slots on the same Turing Pi BMC (same boot.tpi.host) SHALL be serialized; concurrent flashes of two slots on the same board are not supported. Installs on distinct boards SHALL run concurrently when --parallel is in effect.

#### Scenario: Same-board installs serialize
- **WHEN** nostos node install --parallel 2 tp1 tp4 runs and both share boot.tpi.host
- **THEN** the tpi provisioners Boot phase for tp4 blocks until tp1 releases the BMC key

### Requirement: Cleanup powers the slot off after a failed flash
On failure during Boot or WaitMaintenance, the provisioner SHALL attempt tpi power off -n <slot> during Cleanup so the operator can retry from a known state.

#### Scenario: Failed flash leaves slot powered off
- **WHEN** tpi flash returns a non-zero exit code
- **THEN** Cleanup invokes tpi power off -n <slot>, emits a single error event with a redacted message, and Install returns the underlying error

### Requirement: Preflight checks BMC reachability before any side effect
The provisioner SHALL run Preflight before Prepare or Boot. Preflight SHALL verify: tpi --version succeeds, TCP connect to boot.tpi.host:443 succeeds within 2s, every credential _ref resolves successfully, and the cache directory has at least 4 GiB free.

#### Scenario: BMC unreachable fails fast
- **WHEN** boot.tpi.host is set to an unreachable address
- **THEN** Preflight returns provisioner.ErrPreflight with a message naming the host; no image download or flash is attempted

#### Scenario: Missing tpi binary fails clearly
- **WHEN** the tpi binary is not on PATH
- **THEN** Preflight returns an error explaining tpi must be installed and pointing to the upstream install docs
