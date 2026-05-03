## ADDED Requirements

### Requirement: Declare nodes in config.yaml with validated schema
The system SHALL read node definitions from a single `config.yaml`, each node declaring MAC, IP, role, arch, install disk, and template. Validation SHALL fail loudly on shape errors (malformed MAC, duplicate MAC, unknown role, invalid IP, missing template).

#### Scenario: Valid config loads and validates
- **WHEN** `nostos node list` runs against a valid `config.yaml`
- **THEN** all declared nodes are displayed and no validation errors are printed

#### Scenario: Invalid MAC rejected
- **WHEN** a node has a MAC not matching `^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`
- **THEN** any nostos command that reads the registry exits non-zero with the invalid field identified

#### Scenario: Duplicate MAC rejected
- **WHEN** two nodes in `config.yaml` share the same MAC
- **THEN** any nostos command that reads the registry exits non-zero and lists both offending node names

#### Scenario: Role restricted to controlplane or worker
- **WHEN** a node has `role: something-else`
- **THEN** validation fails identifying the offending node

### Requirement: Add a node interactively via Huh form
The system SHALL provide `nostos node add <name>` that launches a Huh form prompting for MAC, IP, role, arch, install disk, and template; writes the result to `config.yaml` atomically; and scaffolds an empty template file if none exists.

#### Scenario: Interactive add succeeds
- **WHEN** `nostos node add dell02` runs and the operator completes the Huh form
- **THEN** `config.yaml` contains a new `dell02` entry AND `templates/dell02.yaml` exists (either scaffolded or already present)

#### Scenario: Duplicate name rejected
- **WHEN** `nostos node add dell01` runs and `dell01` already exists
- **THEN** the command exits non-zero without modifying `config.yaml`

#### Scenario: Non-TTY fails with clear guidance
- **WHEN** `nostos node add dell02 < /dev/null` runs (piped input, no TTY)
- **THEN** the command exits non-zero with a message suggesting the operator run interactively or pass `--mac --ip --role ...` flags

### Requirement: List nodes with reachability
The system SHALL provide `nostos node list` that displays registered nodes in a Lipgloss-rendered table including name, IP, role, ping, apid status, and detected Talos version when reachable.

#### Scenario: Shows per-node status
- **WHEN** `nostos node list` runs against a cluster where one node is up and one is off
- **THEN** the output shows each node's name, IP, role, reachability pill, apid state, and (for reachable) Talos version

#### Scenario: JSON output mode
- **WHEN** `nostos node list --output json` runs
- **THEN** stdout contains a single JSON document parseable by `jq`

### Requirement: Remove a node with confirmation
The system SHALL provide `nostos node remove <name>` that deletes the entry from `config.yaml` only with `--yes` or an interactive confirmation.

#### Scenario: Removal requires confirmation
- **WHEN** `nostos node remove dell01` runs non-interactively without `--yes`
- **THEN** the command exits non-zero without modifying `config.yaml`

#### Scenario: Template file preserved on removal
- **WHEN** `nostos node remove dell01 --yes` completes
- **THEN** `config.yaml` no longer contains `dell01` AND `templates/dell01.yaml` is NOT deleted

### Requirement: Render per-node machineconfig with MAC-hyphen filename
The system SHALL render a Talos machineconfig for a named node by applying the node's template, injecting secrets via the configured backend, and writing to `state/configs/<mac-hyphenated>.yaml` (lowercase, matching iPXE's `${mac:hexhyp}`).

#### Scenario: Rendered filename matches iPXE expectation
- **WHEN** `nostos render dell01` runs for MAC `d0:94:66:d9:eb:a5`
- **THEN** the output file is `state/configs/d0-94-66-d9-eb-a5.yaml`

#### Scenario: Render byte-identical to op inject
- **WHEN** `nostos render dell01` runs with secrets backend set to `onepassword`
- **THEN** the output file is byte-identical to the result of running `op inject -i templates/dell01.yaml -o /tmp/out.yaml` on the same template

#### Scenario: Render validates output via talosctl
- **WHEN** `nostos render dell01` completes successfully
- **THEN** `talosctl validate --config <output> --mode metal` exits zero when invoked on the rendered file (skipped with a warning if talosctl is absent)

#### Scenario: Missing template fails loud
- **WHEN** a node's `template:` references a file that does not exist
- **THEN** `nostos render` exits non-zero identifying the missing file
