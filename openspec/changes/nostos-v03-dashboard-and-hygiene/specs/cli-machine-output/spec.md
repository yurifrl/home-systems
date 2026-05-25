## ADDED Requirements

### Requirement: Every command SHALL accept `--output {text,json,ndjson}`

The system SHALL provide a global `--output` flag on the root command
with the values `text` (default), `json`, and `ndjson`. Every leaf
command SHALL honor the flag. List commands SHALL emit NDJSON; show
commands SHALL emit a single JSON object; event-stream commands
(`node install`) SHALL emit NDJSON one event per line.

The `text` mode is the canonical human surface and remains the default.
JSON modes are the canonical agent surface; agents SHALL NOT parse
text output.

#### Scenario: `nostos status` emits JSON

- **WHEN** `nostos status --output json` is invoked
- **THEN** stdout is a single, well-formed JSON object
- **AND** stderr is empty under success
- **AND** the schema (per the `nostos schema` requirement) lists every
  field of the object

#### Scenario: `nostos node install` emits NDJSON event stream

- **WHEN** `nostos node install tp1 --output ndjson` runs to success
- **THEN** stdout contains one JSON object per line, each with
  `{phase, kind, message, at}`
- **AND** the final line is `{phase:"ready",kind:"ready",...}`
- **AND** no JSON object spans multiple lines

### Requirement: `nostos schema` SHALL enumerate every command, flag, arg, and exit code

The system SHALL provide a `nostos schema [<command-path>]` subcommand
that returns a stable JSON description of the CLI surface. With no
argument, it emits the entire tree as a JSON array (or NDJSON with
`--output ndjson`). With a command path, it emits a single object.

#### Scenario: Schema entries cover every leaf command

- **WHEN** `nostos schema --output json` is invoked
- **THEN** every cobra leaf command appears in the output exactly once
- **AND** each entry has non-empty `summary`, `flags`, `exit_codes`
- **AND** the exit-code catalogue matches the documented set in
  `AGENTS.md`

#### Scenario: Enum and validation annotations are exposed

- **WHEN** a flag or arg has an enum or validation rule
- **THEN** the schema entry exposes `enum: [...]` or
  `validation: "<rule-name>"`
- **AND** an agent that filters on `enum` finds every method choice
  for `--method`, every output choice for `--output`, etc.

### Requirement: Every mutation SHALL accept `--dry-run` and emit a typed `Plan` with ZERO subprocess invocations

The system SHALL expose `--dry-run` on every mutating command (`node
install`, future `cluster cleanup`, future `cluster upgrade`, future
`secrets keys revoke`). When set, the command SHALL populate a typed
`Plan` value, emit it to stdout (JSON when `--output json`, prose
otherwise), and exit with code **0** carrying payload
`"status":"preview"`. Under `--dry-run`:

- The subprocess seam (`Commander`) SHALL record **ZERO** invocations.
- No file SHALL be written and no remote SHALL be mutated.
- The Plan JSON SHALL contain a `would_execute: [...]` array listing
  every subprocess that the live run would invoke (argv template +
  env keys, never env values).
- Re-running the same command without `--dry-run` SHALL produce an
  actual execution sequence that is a (sub)sequence of the planned
  `would_execute`. The live run MUST NOT invoke any subprocess that
  was not listed in the plan.

Dry-run does NOT occupy a dedicated exit code (was 8 in earlier
drafts; that conflicted with shell-reserved low exits per tests
review). Agents detect dry-run by parsing `status:"preview"` in the
JSON payload, not by exit code.

#### Scenario: Dry-run install plans every phase

- **WHEN** `nostos node install tp1 --dry-run --output json` runs
- **THEN** the emitted Plan contains an entry for each Provisioner
  phase (`preflight`, `prepare`, `boot`, `wait`, `apply`, `cleanup`)
  in `would_execute`
- **AND** each entry names the subprocess that would run (argv
  template, env keys; never env values)
- **AND** the subprocess seam (`Commander`) records ZERO invocations
- **AND** the process exits 0 with `status:"preview"`

#### Scenario: Live execution is a subsequence of the plan

- **WHEN** `nostos node install tp1 --dry-run --output json` is
  captured as plan `P`
- **AND THEN** `nostos node install tp1` is run live with
  `FakeCommander{ScriptedSuccess: true}` and its invocation list is
  captured as `L`
- **THEN** `L` is a (sub)sequence of `P.would_execute`
- **AND** no element of `L` is absent from `P.would_execute`

#### Scenario: Dry-run cleanup never deletes (when v0.4 ships B4)

- **WHEN** `nostos cluster cleanup --dry-run` runs (v0.4)
- **THEN** the Tailscale OAuth client SHALL NOT issue any DELETE
- **AND** the emitted Plan lists every device that would be deleted
  with `device_id`, `hostname`, `last_seen`, `age_days`
- (v0.3: `cluster cleanup` is deferred per the proposal Stream B4.)

### Requirement: Errors SHALL be structured with stable codes

The system SHALL emit errors as structured values with the shape
`{error: true, code: string, message: string, details: object?,
hint: string?}`. Under `--output json`, the structured error SHALL be
written to stdout AND any hint SHALL ride in the JSON `hint` field on
stdout (stderr empty). Under `--output text`, the message goes to
stderr and the hint goes to stderr. The two streams SHALL never
overlap for a single invocation.

The exit-code catalogue is **6 entries**, with nostos-specific codes
in the **10-19** range to avoid POSIX / shell-reserved collisions:

| Code | Meaning             | Notes                                         |
|------|---------------------|-----------------------------------------------|
| 0    | success             | dry-run returns 0 with payload `status:"preview"` |
| 1    | generic error       | reserved fallback; prefer a specific code     |
| 10   | validation          | input rejected, schema mismatch               |
| 11   | network             | unreachable host, DNS, TLS handshake          |
| 12   | auth                | BMC auth, OAuth, kubeconfig context wrong     |
| 13   | conflict            | flock held, node-already-ready, digest mismatch |
| 64   | usage               | cobra default; preserved                      |

Sub-causes (BMC unreachable vs auth vs version, digest mismatch vs
unpinned, node-already-ready vs flock-held) live in `details.code` of
the structured error, NOT in unique top-level exit numbers.

#### Scenario: Validation error returns code 10 with details

- **WHEN** `nostos node install nonexistent --output json` is invoked
  and `nonexistent` is not in `nostos/config.yaml`
- **THEN** stdout contains `{"error":true,"code":"validation",
  "message":"node not found: nonexistent","details":{"node":"nonexistent",
  "available":["dell01","tp1","tp4"]}}`
- **AND** the process exits 10

#### Scenario: BMC errors expose the sub-cause

- **WHEN** `nostos node install tp1 --output json` is invoked against
  a misconfigured BMC
- **THEN** stdout contains `code:"auth"` (when 401/403) or
  `code:"network"` (when unreachable) with `details.code` one of
  `bmc_unreachable` / `bmc_auth` / `bmc_version`
- **AND** the process exits 12 (auth) or 11 (network) accordingly

### Requirement: List and show commands SHALL accept `--fields=a,b,c`

The system SHALL implement a field-mask projection on every list and
show command, including `nostos dashboard --once --output json`.
Unknown fields SHALL fail with structured error code `validation`
(exit 10). Field names SHALL support dot-notation for nested objects
and arrays (e.g. `nodes.name`).

#### Scenario: Field projection reduces output

- **WHEN** `nostos node list --output json --fields=name,ip`
- **THEN** every emitted object contains exactly the keys `name`
  and `ip`

### Requirement: Inputs SHALL be hardened against agent / fuzzed misuse

The system SHALL reject:

- node names not matching `^[a-z0-9][a-z0-9-]{0,62}$`
- **any user-supplied string containing an ASCII control character
  in the range `0x00-0x1F` or `0x7F`** (node names, field-mask
  names, `op://` ref strings, free-form labels)
- `--config` paths containing `..` segments after lex-cleaning, OR
  resolving (after symlink resolution) outside the operator's home
  directory or the repo root
- `op://` refs containing query parameters or fragments
- YAML inputs with embedded anchors that resolve to filesystem paths

Each rejection SHALL surface as a structured validation error (code
`validation`, exit 10) with a `details.field` naming the offending
input.

#### Scenario: Control characters rejected in node names

- **WHEN** an agent invokes `nostos node install $'tp1\x00'`
- **THEN** the command fails with `code:"validation"`,
  `details.field:"name"`, exit 10

#### Scenario: Path traversal in `--config` rejected

- **WHEN** `nostos --config ../../etc/passwd status` is invoked
- **THEN** the command fails with `code:"validation"`, exit 10
- **AND** no read of the offending path occurs

#### Scenario: Symlink-resolved escape from repo root rejected

- **WHEN** `nostos --config ./symlink-to-etc-passwd status` is
  invoked AND the symlink target resolves outside `$HOME` and the
  repo root
- **THEN** the command fails with `code:"validation"`, exit 10
- **AND** the rejection happens AFTER symlink resolution, not before

### Requirement: AGENTS.md SHALL document non-obvious invariants

The system SHALL ship `.submodules/nostos/AGENTS.md` with at minimum:
required command sequences, exit-code catalogue (mirroring the table
above), idempotency guarantees per command, and explicit warnings
("Always pass `--reinstall` when re-flashing a live node — never
delete the rendered config first"; "Always run `nostos secrets test
tailscale` before `node install` after editing the secrets config").

#### Scenario: Every documented exit code matches the schema

- **WHEN** the AGENTS.md exit-code table is parsed
- **THEN** every code appears in `nostos schema --output json` under
  at least one command's `exit_codes`
- **AND** no code in the schema is missing from AGENTS.md
