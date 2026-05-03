## ADDED Requirements

### Requirement: Pluggable backend selection
The system SHALL select the active secrets backend from `config.yaml` under `secrets.backend` and initialize it on first use.

#### Scenario: Backend selection from config
- **WHEN** `config.yaml` sets `secrets.backend: onepassword` and nostos needs to resolve a secret
- **THEN** the 1Password backend is used and no other backend is initialized

#### Scenario: Unknown backend rejected
- **WHEN** `config.yaml` sets `secrets.backend` to a value not in the known set (`onepassword`, `sops`, `env`, `file`)
- **THEN** any nostos command that would touch secrets exits non-zero with a message listing supported backends

### Requirement: Resolve URIs by scheme
The system SHALL resolve secret URIs of the form `<scheme>://<authority>/<path>` where the scheme maps to a registered backend: `op://` → 1Password, `sops://` → sops-encrypted file, `env://` → environment variable, `file://` → filesystem path.

#### Scenario: 1Password URI resolves
- **WHEN** a template contains `TS_AUTHKEY=op://kubernetes/talos/TS_AUTHKEY` and the 1Password CLI is signed in
- **THEN** the rendered config contains the actual secret value in place of the URI, unquoted

#### Scenario: Env URI resolves
- **WHEN** a template contains `KEY=env://MY_SECRET` and the environment has `MY_SECRET=hunter2`
- **THEN** the rendered config contains `KEY=hunter2`

#### Scenario: File URI resolves
- **WHEN** a template contains `CA=file:///tmp/ca.crt` and that file exists
- **THEN** the rendered config contains the file's contents (whitespace-trimmed) in place of the URI

#### Scenario: Missing value fails loud
- **WHEN** a URI cannot be resolved (op item not found, env var unset, file missing)
- **THEN** the render command exits non-zero with a message naming the failing URI and the backend's specific error

### Requirement: Validate backend availability before render
The system SHALL validate that the configured backend is usable before beginning any render, providing actionable remediation when it isn't.

#### Scenario: 1Password not signed in
- **WHEN** `nostos render <node>` is run and `op whoami` indicates no signed-in session
- **THEN** the command exits non-zero with a message suggesting `op signin`

#### Scenario: 1Password interactive prompt on TTY
- **WHEN** `nostos render <node>` is run on an interactive terminal and the 1Password CLI needs biometric approval
- **THEN** the command does NOT exit immediately but allows the CLI's prompt to succeed before continuing

### Requirement: Never log or persist resolved secrets
The system SHALL never log resolved secret values, never write them outside `state/configs/*.yaml` (the rendered machineconfigs), and never echo them to stdout/stderr in error messages.

#### Scenario: Error messages reference URIs, not values
- **WHEN** rendering fails after a secret was resolved
- **THEN** the error message contains the URI that was being resolved (e.g. `op://kubernetes/talos/TS_AUTHKEY`) but NOT the resolved value

#### Scenario: Logs redact resolved values
- **WHEN** nostos logs a render operation at any verbosity level
- **THEN** resolved secret values do not appear in the log

### Requirement: In-memory-only secret caching
The system SHALL cache resolved secrets only for the duration of a single nostos invocation, never persisting them to disk outside the rendered machineconfigs, and never across process boundaries.

#### Scenario: Cache lifetime bounded to process
- **WHEN** one `nostos render` completes, resolving 10 URIs
- **THEN** no cache file is written to `state/` and a subsequent `nostos render` re-resolves all 10 URIs from the backend

### Requirement: Extensibility via Protocol
The system SHALL expose a `SecretsBackend` Protocol with `resolve(uri)` and `validate()` methods, enabling third-party backends via Python import path configuration.

#### Scenario: Protocol contract
- **WHEN** a new backend class is registered implementing `SecretsBackend` with a `scheme` class attribute
- **THEN** templates using that scheme's URIs are resolved by the new backend without changes to templates or core nostos code
