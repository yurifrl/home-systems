## ADDED Requirements

### Requirement: Pluggable backend selection
The system SHALL select the active secrets backend from `config.yaml` under `secrets.backend` and initialize it on first use.

#### Scenario: Backend selection from config
- **WHEN** `config.yaml` sets `secrets.backend: onepassword` and nostos needs to resolve a secret
- **THEN** the 1Password backend is used and no other backend is initialized beyond the always-available `env` and `file`

#### Scenario: Unknown backend rejected
- **WHEN** `config.yaml` sets `secrets.backend` to a value not in `{onepassword, sops, env, file}`
- **THEN** any nostos command that reads secrets exits non-zero listing supported backends

### Requirement: Resolve URIs by scheme via Go interface
The system SHALL expose a `Backend` interface:

```go
type Backend interface {
    Scheme() string
    Resolve(uri string) (string, error)
    Validate() error
}
```

Schemes SHALL be dispatched: `op://` → 1Password, `sops://` → sops, `env://` → environment, `file://` → filesystem.

#### Scenario: 1Password URI resolves
- **WHEN** a template contains `TS_AUTHKEY=op://kubernetes/talos/TS_AUTHKEY` and `op` is signed in
- **THEN** the rendered config contains the resolved tskey string in place of the URI, unquoted and without surrounding whitespace

#### Scenario: Env URI resolves
- **WHEN** a template contains `KEY=env://MY_SECRET` and the env has `MY_SECRET=hunter2`
- **THEN** the rendered config contains `KEY=hunter2`

#### Scenario: File URI resolves
- **WHEN** a template contains `CA=file:///tmp/ca.crt` and that file exists
- **THEN** the rendered config contains the trimmed file contents

#### Scenario: Unresolvable URI fails loud
- **WHEN** a URI cannot be resolved (op item missing, env var unset, file missing)
- **THEN** rendering exits non-zero naming the failing URI and the backend's specific error

### Requirement: Pre-flight backend validation before render
The system SHALL call `Validate()` on the selected backend before beginning any render, producing actionable remediation on failure.

#### Scenario: 1Password not signed in
- **WHEN** `nostos render <node>` runs and `op whoami` indicates no active session
- **THEN** the command exits non-zero with a message suggesting `op signin --account <account>`

#### Scenario: Interactive 1Password prompt preserved
- **WHEN** `nostos render <node>` runs on a TTY and the `op` CLI requires biometric approval
- **THEN** the command does NOT exit immediately but allows `op read` to complete after operator approval

### Requirement: Never log or persist resolved secrets
The system SHALL never log resolved secret values, never write them outside `state/configs/*.yaml`, and never echo them in error messages.

#### Scenario: Error messages reference URIs, not values
- **WHEN** rendering fails after secrets were partially resolved
- **THEN** the error message contains the failing URI but NOT any resolved secret value

#### Scenario: Verbose logging redacts values
- **WHEN** nostos runs with `--verbose` and logs render progress
- **THEN** resolved secret values do not appear in the log stream

### Requirement: In-memory-only secret caching
The system SHALL cache resolved secrets only for the duration of a single invocation, never writing them anywhere outside the rendered machineconfigs, and never persisting across process boundaries.

#### Scenario: Cache lifetime bounded to process
- **WHEN** one `nostos render` completes resolving N URIs
- **THEN** no cache file appears under `state/` and a subsequent `nostos render` re-resolves all N URIs from the backend
