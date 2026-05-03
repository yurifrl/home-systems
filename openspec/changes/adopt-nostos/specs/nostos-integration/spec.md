## ADDED Requirements

### Requirement: Canonical repo layout for nostos data
The home-systems repo SHALL store all nostos-consumer data under `nostos/` at the repo root: `nostos/config.yaml` (registry), `nostos/templates/*.yaml` (machineconfigs), and `nostos/state/` (gitignored cache).

#### Scenario: Single directory holds all nostos data
- **WHEN** an operator inspects the repo after this change
- **THEN** `ls nostos/` shows exactly `config.yaml`, `templates/`, and (if any nostos command has been run) `state/`; no nostos data lives outside this directory

#### Scenario: state is gitignored
- **WHEN** `nostos build` runs and creates files under `nostos/state/`
- **THEN** `git status` does not list `nostos/state/` contents as untracked changes

### Requirement: Tool vendored under .submodules
The home-systems repo SHALL vendor the nostos tool at `.submodules/nostos/` and invoke it via `go run ./.submodules/nostos/cmd/nostos`.

#### Scenario: Tool invocation path
- **WHEN** any nostos command is run through the Taskfile wrappers
- **THEN** the underlying invocation is `go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml <subcommand>`

### Requirement: Taskfile wrappers for nostos commands
The home-systems repo SHALL expose Taskfile wrappers named `task nostos:<cmd>` for the core operator commands (`build`, `render`, `up`, `bootstrap`, `status`), each passing `--config nostos/config.yaml`.

#### Scenario: Task wrapper invokes nostos
- **WHEN** `task nostos:up` is run from the repo root
- **THEN** it executes `go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml serve`

#### Scenario: Wrappers pass NODE variable
- **WHEN** `task nostos:render NODE=dell01` is run
- **THEN** the underlying invocation passes `dell01` as the node argument to `nostos render`

### Requirement: Legacy pxe/ removal
The home-systems repo SHALL NOT contain `pxe/` directory, `taskfiles/pxe.yml`, or any `task pxe:*` command after this change.

#### Scenario: pxe directory gone
- **WHEN** an operator runs `ls pxe/` after this change
- **THEN** the command reports the directory does not exist

#### Scenario: pxe task namespace gone
- **WHEN** an operator runs `task --list` after this change
- **THEN** no tasks with the `pxe:` prefix appear

### Requirement: talos.yml pruned of pxe entries
The `taskfiles/talos.yml` file SHALL NOT contain any task that depends on the removed `pxe/` directory or references `talos/op/nodes/` files only used for PXE-driven installs (`apply:dell01` specifically).

#### Scenario: apply:dell01 removed
- **WHEN** `task talos:apply:dell01` is run after this change
- **THEN** task reports the task does not exist (or the task is no longer declared)

### Requirement: CLAUDE.md reflects nostos workflow
The repo's `CLAUDE.md` SHALL describe the nostos provisioning workflow (not RPi-era PXE scripts) under its provisioning section, and SHALL link to `nostos/README.md` for consumer-specific details.

#### Scenario: Agent guidance is current
- **WHEN** an AI coding agent reads `CLAUDE.md` after this change
- **THEN** the provisioning section mentions `task nostos:*` commands, references `.submodules/nostos/`, and does NOT mention `task pxe:*` or the decommissioned RPi controlplane

### Requirement: Cluster unaffected during migration
The home-systems cluster (Dell controlplane + running workloads) SHALL remain operational throughout this change. No `talosctl apply-config`, `kubectl` mutations, or node restarts are performed by this change.

#### Scenario: Cluster health before and after is identical
- **WHEN** `kubectl get nodes` is run before and after this change
- **THEN** the list of Ready nodes is identical and no restart events occurred as a result of this change
