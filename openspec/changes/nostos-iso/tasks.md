# nostos-iso — Tasks

## 1. Config schema

- [x] 1.1 Add an `images` section to the nostos config model (`internal/config`): named entries with `build` (uup build id, edition, driverSource, answerFile), `store` (bucket, object, project), and `credentialsRef` (`op://`).
- [x] 1.2 Add validation: unknown `<name>` errors clearly; required fields enforced; no defaults that embed machine identity.
- [x] 1.3 Update the JSON schema / `schema` command output and add a config fixture for tests.
- [x] 1.4 Unit-test config load + lookup-by-name (present/absent entry).

## 2. Build verb

- [x] 2.1 Create the guest-iso package (e.g. `internal/guestiso`) with a `Build(ctx, entry) (isoPath, error)` that shells to a privileged container via `internal/execx`.
- [x] 2.2 Embed the container build script (`go:embed`) — port `hack/win11-iso/build.sh` (UUP base + virtio + answer file + `efisys_noprompt.bin`, with transient-retry on the UUP fetch).
- [x] 2.3 Wire inputs from config (no literals): UUP id, edition, driver source, answer-file path, output path under the nostos state dir.
- [x] 2.4 Fail fast with a clear error when the container runtime is unavailable.
- [x] 2.5 Test: build invocation builds the correct container command from a config entry (no network/loop-mount in unit test; assert command + args).

## 3. Publish verb

- [x] 3.1 Add the GCS SDK (`cloud.google.com/go/storage`) to `go.mod`; isolate imports to the guest-iso package.
- [x] 3.2 Implement `Publish(ctx, entry, isoPath)`: resolve the SA key via the `op://` secrets backend, upload to `store.bucket/store.object`, report the stored location.
- [x] 3.3 Guard against making the object/bucket public (never set public ACLs).
- [x] 3.4 Test publish against an httptest/fake storage server (mirror the `osimage/proxmox` fixture pattern).

## 4. Sign-URL verb

- [x] 4.1 Implement `SignURL(ctx, entry, duration)`: mint a V4 signed GET URL (cap at provider max 7d) using the resolved SA key.
- [x] 4.2 Emit a paste-ready snippet for the consuming config (crossplane-proxmox private values `isos.win11.url`).
- [x] 4.3 Test signing produces a well-formed, time-limited URL from a fixture key.

## 5. CLI wiring

- [x] 5.1 Add `internal/cli/iso.go`: `nostos iso` parent with `build`, `publish`, `url`, `prepare` subcommands, each taking `<name>` (+ optional duration for `url`).
- [x] 5.2 Register `newISOCmd()` in `internal/cli/root.go`.
- [x] 5.3 Implement `prepare` as build → publish → url, stopping at first failure.
- [x] 5.4 Honor global flags (`--config`, `--output text|json`); add `iso` command help/usage.
- [x] 5.5 CLI smoke test (mirror `flash_test.go`/`cli_test.go`): command tree, arg validation, missing-entry error.

## 6. Integration & cutover

- [x] 6.1 Add the Win11 `images` entry to nostos config (replacing the scripts' hardcoded bucket/project/object).
- [ ] 6.2 Verify `nostos iso prepare <name>` reaches parity: ISO present in the private bucket + valid signed URL.
- [ ] 6.3 Remove `hack/win11-iso/` and repoint `crossplane-proxmox` README/values comments at `nostos iso`.
- [x] 6.4 Update nostos `README.md`/`AGENTS.md` with the `iso` command + the Docker/`op` prerequisites.
