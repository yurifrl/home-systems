# guest-iso

## ADDED Requirements

### Requirement: Config-driven guest ISO entries

The system SHALL define guest install media as named entries in nostos `config.yaml` under an `images` section. Each entry SHALL carry everything needed to build, publish, and sign its ISO: build source (e.g. UUP build id, edition), input assets (driver source, answer-file path), object-store target (bucket, object name, project), and a credentials reference expressed as `op://`. The Go code SHALL NOT hardcode any machine name, bucket name, object name, or project id; all such values SHALL be resolved from the named config entry.

#### Scenario: Operate on a named entry

- **WHEN** a user runs `nostos iso <verb> <name>`
- **THEN** the system resolves all parameters from the `images` entry keyed by `<name>` in config, and fails with a clear error if no such entry exists

#### Scenario: No hardcoded machine identity

- **WHEN** the source is reviewed for a given verb
- **THEN** no bucket name, object name, GCP project id, or machine/node name appears as a literal in Go code; each originates from config

### Requirement: Build guest ISO

The system SHALL build the combined guest install ISO for a named entry from the entry's declared inputs. The build MAY shell out to a container runtime when privileged operations (loop-mounts) are required. The build SHALL produce a single ISO artifact at a deterministic local path and SHALL fail with a clear error if a required input or the container runtime is unavailable.

#### Scenario: Build produces the artifact

- **WHEN** `nostos iso build <name>` completes successfully
- **THEN** the combined ISO exists at the configured/derived local output path and includes the entry's declared driver and answer-file inputs

#### Scenario: Transient build-source failures are retried

- **WHEN** a build-source fetch fails transiently (e.g. upstream 5xx)
- **THEN** the build retries before failing, and surfaces the underlying error if retries are exhausted

### Requirement: Publish guest ISO

The system SHALL upload a built ISO to the entry's configured object store using credentials resolved from the entry's `op://` reference. The system SHALL NOT make the object or bucket public.

#### Scenario: Upload to the configured target

- **WHEN** `nostos iso publish <name>` runs after a successful build
- **THEN** the ISO is uploaded to the configured bucket/object using the resolved service-account credentials, and the command reports the stored object location

#### Scenario: Credentials come from the secrets backend

- **WHEN** publish needs object-store credentials
- **THEN** they are resolved through the existing `op://` secrets backend, never read from a literal in code

### Requirement: Sign a download URL

The system SHALL mint a short-lived V4 signed GET URL for the entry's published object, with a configurable duration not exceeding the provider maximum. It SHALL emit a paste-ready snippet for the consuming configuration (e.g. the crossplane-proxmox private values).

#### Scenario: Produce a usable signed URL

- **WHEN** `nostos iso url <name> [duration]` runs and the object exists
- **THEN** the command prints a valid time-limited signed URL and a paste-ready config snippet, without exposing the object publicly

### Requirement: End-to-end prepare

The system SHALL provide a single verb that runs build, publish, and sign in sequence for a named entry.

#### Scenario: One command does the whole pipeline

- **WHEN** `nostos iso prepare <name>` runs
- **THEN** the ISO is built, uploaded to the configured private object store, and a signed URL snippet is printed, stopping with a clear error at the first failing stage
