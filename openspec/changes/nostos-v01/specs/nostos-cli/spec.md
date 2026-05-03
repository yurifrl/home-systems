## ADDED Requirements

### Requirement: Single binary entrypoint
The system SHALL provide a `nostos` command installable via `uv tool install nostos` (or `pipx install nostos`) that exposes all capabilities through a nested `click`-based CLI.

#### Scenario: Installation and invocation
- **WHEN** `uv tool install nostos` is run from a clean environment
- **THEN** the `nostos` command is on PATH and `nostos --help` prints the top-level command list

#### Scenario: Nested subcommands
- **WHEN** `nostos node --help` is run
- **THEN** the output lists `add`, `list`, `remove` subcommands with their summaries

### Requirement: Config file auto-discovery
The system SHALL discover `config.yaml` by searching (a) path given to `--config`, (b) `$NOSTOS_CONFIG` environment variable, (c) the current directory, (d) parent directories up to the filesystem root.

#### Scenario: Explicit --config wins
- **WHEN** `nostos --config /path/to/other.yaml status` is run
- **THEN** the file at `/path/to/other.yaml` is loaded even if a `config.yaml` exists in the current directory

#### Scenario: Discovery walks up directories
- **WHEN** `nostos status` is run from a subdirectory of a repo that has `nostos/config.yaml` at its root
- **THEN** that root `nostos/config.yaml` is discovered and used

#### Scenario: No config found fails clearly
- **WHEN** no `config.yaml` is discoverable and `nostos node list` is run
- **THEN** the command exits non-zero with a message suggesting `nostos init`

### Requirement: Initialize a new consumer project
The system SHALL provide `nostos init` that scaffolds `config.yaml`, `templates/`, `state/` (with `.gitignore`), and an empty node registry.

#### Scenario: Init creates canonical layout
- **WHEN** `nostos init` is run in an empty directory
- **THEN** the directory contains `config.yaml` (with a commented example), `templates/` (empty), and `state/.gitignore`

#### Scenario: Init refuses to overwrite
- **WHEN** `nostos init` is run in a directory that already contains `config.yaml`
- **THEN** the command exits non-zero without modifying anything, unless `--force` is passed

### Requirement: Structured output modes
The system SHALL support `--output text` (default, human-readable via `rich`) and `--output json` (machine-readable) for commands that list or query state.

#### Scenario: JSON output is valid
- **WHEN** `nostos node list --output json` is run
- **THEN** the output is a single valid JSON document, writable to a file, and parseable by `jq`

#### Scenario: Text output uses rich formatting
- **WHEN** `nostos node list` is run on a terminal that supports color
- **THEN** the output uses color-coded status pills (e.g. green for `up`, red for `down`)

### Requirement: Error handling with actionable messages
The system SHALL render errors through a consistent formatter that includes: a one-line summary, the failing operation, the remediation hint, and the exit code. Internal stack traces are hidden behind `--debug`.

#### Scenario: Default error output is user-friendly
- **WHEN** a command fails because 1Password is not signed in
- **THEN** the output is `Error: 1Password session not active. Run: op signin` (or similar) without a Python traceback

#### Scenario: Debug mode shows traceback
- **WHEN** the same command is run with `--debug` and fails
- **THEN** the output includes the full Python traceback in addition to the user-friendly message

### Requirement: Top-level commands
The system SHALL expose the following commands at the top level: `init`, `node`, `build`, `render`, `serve`, `install`, `wipe`, `bootstrap`, `config`, `status`, `kubeconfig`, `nuke`, `web`.

#### Scenario: All commands are listed in help
- **WHEN** `nostos --help` is run
- **THEN** the output lists each command above with a one-line description

#### Scenario: Unknown commands error clearly
- **WHEN** `nostos foobar` is run with no such command
- **THEN** the command exits non-zero with `Unknown command: foobar`; Did you mean: <closest match>?
