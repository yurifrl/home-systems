## ADDED Requirements

### Requirement: Declare nodes in config.yaml
The system SHALL read node definitions from a single `config.yaml` file, each node identified by name and carrying MAC address, IP address, role, architecture, install disk, and template reference.

#### Scenario: Config loads and validates
- **WHEN** `nostos node list` is run against a valid `config.yaml`
- **THEN** all declared nodes are displayed with their declared fields, and no validation errors are printed

#### Scenario: Invalid MAC is rejected
- **WHEN** a node has a MAC that is not six hex octets separated by colons
- **THEN** `nostos node list` exits non-zero with a message identifying the invalid field and node name

#### Scenario: Duplicate MAC is rejected
- **WHEN** two nodes in `config.yaml` share the same MAC
- **THEN** any nostos command that reads the registry exits non-zero with a message listing both offending node names

### Requirement: Add a node interactively
The system SHALL provide `nostos node add <name>` that prompts for MAC, IP, role, architecture, install disk, and template; writes the result to `config.yaml` atomically; and creates a template stub if none exists.

#### Scenario: Interactive add succeeds
- **WHEN** `nostos node add dell02` is run and the operator answers all prompts
- **THEN** `config.yaml` contains a new `dell02` entry, and `templates/dell02.yaml` exists (either as a copy of the nearest-matching template or as a scaffolded empty template)

#### Scenario: Duplicate name is rejected
- **WHEN** `nostos node add dell01` is run and a `dell01` entry already exists
- **THEN** the command exits non-zero without modifying `config.yaml`

### Requirement: List nodes with live reachability
The system SHALL provide `nostos node list` that displays registered nodes and their current reachability state (`up`/`down`/`unknown`) plus detected Talos version when reachable.

#### Scenario: Shows per-node status
- **WHEN** `nostos node list` is run against a cluster where one node is up and one is off
- **THEN** the output shows each node's name, IP, role, reachability, and — for reachable nodes — the detected Talos version

### Requirement: Remove a node
The system SHALL provide `nostos node remove <name>` that deletes the node's entry from `config.yaml` after confirmation.

#### Scenario: Removal requires confirmation
- **WHEN** `nostos node remove dell01` is run non-interactively without `--yes`
- **THEN** the command exits non-zero without modifying `config.yaml`

#### Scenario: Removal preserves template file
- **WHEN** `nostos node remove dell01 --yes` completes
- **THEN** `config.yaml` no longer contains the `dell01` entry AND `templates/dell01.yaml` is NOT deleted (templates are shared potential resources)

### Requirement: Render per-node machineconfig
The system SHALL render a Talos machineconfig for a named node by applying the node's template, injecting secret values via the configured secrets backend, and writing the output to `state/configs/<mac-hyphenated>.yaml` (lowercase hex-hyphen form, matching iPXE's `${mac:hexhyp}` substitution).

#### Scenario: Rendered filename matches iPXE expectation
- **WHEN** `nostos render dell01` is run and `dell01`'s MAC is `d0:94:66:d9:eb:a5`
- **THEN** the output file is `state/configs/d0-94-66-d9-eb-a5.yaml`

#### Scenario: Render validates output
- **WHEN** `nostos render dell01` completes successfully
- **THEN** `talosctl validate --config <output> --mode metal` exits zero on the rendered file

#### Scenario: Missing template fails loud
- **WHEN** a node's `template:` field references a file that does not exist under `templates/`
- **THEN** `nostos render` exits non-zero with a message naming the missing file
