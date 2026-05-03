## ADDED Requirements

### Requirement: Invoked via `go run`
The system SHALL be invoked exclusively via `go run ./.submodules/nostos/cmd/nostos ...`. The repository SHALL NOT ship compiled binaries nor require `go install`.

#### Scenario: Fresh clone invocation works
- **WHEN** a fresh `git clone` of the consumer repo is followed by `go run ./.submodules/nostos/cmd/nostos --version`
- **THEN** the command prints `nostos, version 0.1.0` after Go downloads module dependencies (first-run cost) and future invocations complete in sub-second wallclock

#### Scenario: Taskfile wrappers use go run
- **WHEN** `task nostos:up NODE=dell01` runs
- **THEN** the underlying invocation is `go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml up dell01`

#### Scenario: No binary artifacts committed
- **WHEN** any commit touches `.submodules/nostos/`
- **THEN** the commit does not include compiled binaries (enforced via `.gitignore` for `bin/`, `dist/`, and equivalents)

### Requirement: Cobra subcommand tree
The system SHALL expose subcommands via a [cobra](https://github.com/spf13/cobra) command tree with a root `nostos` group. The top-level commands SHALL be: `init`, `node`, `build`, `render`, `serve`, `up`, `wipe`, `bootstrap`, `config`, `status`, `kubeconfig`, `nuke`.

#### Scenario: Help lists all commands
- **WHEN** `nostos --help` runs
- **THEN** every command above appears with a one-line description

#### Scenario: Nested subcommands
- **WHEN** `nostos node --help` runs
- **THEN** the output lists `add`, `list`, `remove`

#### Scenario: Shell completion generation
- **WHEN** `nostos completion bash` (or zsh/fish/powershell) runs
- **THEN** the command emits valid shell completion script for the selected shell

### Requirement: Config discovery
The system SHALL discover `config.yaml` in the following order: (1) `--config` flag, (2) `$NOSTOS_CONFIG`, (3) `config.yaml` in current working directory, (4) walk parent directories looking for `nostos/config.yaml`.

#### Scenario: Explicit --config wins
- **WHEN** `nostos --config /abs/path/other.yaml status` runs
- **THEN** the explicit file is loaded even if a `config.yaml` exists in the current directory

#### Scenario: Discovery walks up directories
- **WHEN** `nostos status` runs from a subdirectory of a repo that has `nostos/config.yaml` at its root
- **THEN** that file is discovered automatically

#### Scenario: Absent config fails with guidance
- **WHEN** no config is discoverable and `nostos node list` runs
- **THEN** the command exits non-zero suggesting `nostos init`

### Requirement: Structured output modes
Commands that list or query state SHALL support both `--output text` (Lipgloss-rendered, human-readable, default) and `--output json` (machine-readable via a consistent `Response{Ok, Error, Data}` envelope).

#### Scenario: JSON output is valid
- **WHEN** `nostos node list --output json` runs
- **THEN** stdout contains a single well-formed JSON document parseable by `jq`

#### Scenario: Text output uses Lipgloss styling on TTY
- **WHEN** `nostos node list` runs on a TTY
- **THEN** the output uses color-coded status indicators (green for up, red for down, etc.)

#### Scenario: Auto-detect non-TTY
- **WHEN** `nostos node list` runs with stdout piped to a file
- **THEN** Lipgloss styling is stripped and output is plain text

### Requirement: Error handling
Errors SHALL render through a consistent formatter: a one-line summary, the failing operation, a remediation hint, and the exit code. Go stack traces SHALL be hidden unless `--verbose` is passed.

#### Scenario: Default error output is user-friendly
- **WHEN** a command fails because op is not signed in
- **THEN** the output is a single line like `Error: 1Password session not active. Run: op signin --account <name>`

#### Scenario: Verbose mode shows stack
- **WHEN** the same command runs with `--verbose` and fails
- **THEN** the output includes the Go stack trace in addition to the user-friendly message

### Requirement: Initialize a new consumer project
The system SHALL provide `nostos init [<directory>]` that scaffolds `config.yaml`, `templates/` (empty dir), and `state/` (with `.gitignore`).

#### Scenario: Init creates canonical layout
- **WHEN** `nostos init ./newproj` runs in an empty directory
- **THEN** `newproj/config.yaml` contains a commented schema example, `newproj/templates/` exists, and `newproj/state/.gitignore` wildcard-ignores everything inside

#### Scenario: Init refuses to overwrite
- **WHEN** `nostos init .` runs in a directory that already contains `config.yaml`
- **THEN** the command exits non-zero unless `--force` is passed
