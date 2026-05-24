## 1. Test seams and provisioner package skeleton

- [ ] 1.1 Create `.submodules/nostos/internal/execx/commander.go` with
  `Commander` interface (`Run(ctx, name, args, env, stdin, stdout, stderr) error`)
  and a default `OSCommander` that wraps `exec.CommandContext`. Acceptance:
  unit test injects `FakeCommander`, captures argv + env + stdin, returns
  scripted exit codes; `OSCommander` runs `/bin/echo hi` and captures stdout.
- [ ] 1.2 Create `internal/clockx/clock.go` with `Clock` interface
  (`Now`, `Sleep`, `NewTimer`) and `FakeClock`. Acceptance: FakeClock
  advances deterministically in unit tests; no test in
  `internal/provisioner/...` calls `time.Now` directly (`grep -R
  'time\.Now\|time\.Sleep' internal/provisioner` returns zero hits).
- [ ] 1.3 Create `internal/provisioner/provisioner.go` with `Provisioner`
  interface, `Phase` enum, `Event` struct, `EventEmitter` type, `Deps`,
  and the registry (`Register` / `New`). Method enum is **{pxe, tpi}**.
  Acceptance: `go vet ./.submodules/nostos/internal/provisioner/...`
  clean; `Register` panics with the method name in the recover-message on
  duplicate; `New("unknown")` returns an error matching
  `errors.Is(err, ErrNotRegistered)`.
- [ ] 1.4 Create `internal/provisioner/errors.go` with sentinels
  (`ErrPreflight`, `ErrBoot`, `ErrTimeout`, `ErrLocked`,
  `ErrNodeAlreadyReady`, `ErrNotRegistered`). Acceptance:
  `errors.Is` round-trip test for each sentinel.
- [ ] 1.5 Create `internal/provisioner/redact/scrubber.go` with
  `Scrubber.AddAll([]string)` and `Scrubber.Scrub(string) string`, plus
  `WrapEmitter(EventEmitter, *Scrubber) EventEmitter` that scrubs every
  `Event.Message` field. Acceptance: property test (rapid-style) — for
  any string `s` and secret table `T`, `Scrub(s, T)` contains no `t ∈ T`
  as substring; counterexamples include overlapping prefixes
  (`secret`, `secretX`).
- [ ] 1.6 Create `internal/provisioner/contention/map.go` with `Acquire(key
  string) func()` (release fn). Acceptance: barrier-based concurrency
  test — same key serializes (no overlap in observed enter/exit pairs);
  distinct keys overlap (both enter before either exits).
- [ ] 1.7 Create `internal/provisioner/flock/node.go` with
  `AcquireNode(name string) (release func(), err error)` using
  `golang.org/x/sys/unix` `LOCK_EX|LOCK_NB` on
  `nostos/state/configs/<name>.lock`. Acceptance: spawn a child process
  holding the lock; parent's `AcquireNode` returns `ErrLocked` within
  100ms; child exit releases.
- [ ] 1.8 Create `internal/provisioner/provisionertest/compliance.go`
  exposing `RunComplianceSuite(t, factory func() Provisioner)`.
  Acceptance: table-driven invariants from design D-Tests run; both
  `pxe` and `tpi` pass with no skips at end of this change.

## 2. Config schema (additive)

- [ ] 2.1 Edit `internal/config/config.go`: add optional `Boot` struct on
  `Node` with `Method` enum **{pxe, tpi}** and `*TPIBoot` sub-block.
  Default `Method=pxe` when block omitted. Acceptance: existing
  `nostos/config.yaml` (only dell01) loads with `Boot.Method=="pxe"`;
  literal-bytes round-trip for the dell01 entry is preserved modulo
  whitespace.
- [ ] 2.2 Add a `Ref` typed-string with a custom `UnmarshalYAML` that
  requires a known URI prefix (`op://`, `sops://`, `file://`).
  `env://` is **rejected** for BMC creds. Anything else fails to
  unmarshal with a typed error citing the field path. Acceptance:
  fixture `config-inline-secret.yaml` (literal password under
  `boot.tpi.password`) fails YAML unmarshal with field path
  `nodes[tp1].boot.tpi.password`; `env://op` fails with
  `env:// scheme not allowed for credential refs`.
- [ ] 2.3 Add validation rules: if `Method==tpi`, `Boot.TPI` non-nil with
  `Host`, `Slot ∈ [1..4]`, and either (`UsernameRef` + `PasswordRef`) or
  `IdentityFileRef`. Across all `tpi`-method nodes, `(Host, Slot)` MUST
  be unique. Acceptance: unit tests for valid + each invalid permutation
  including a `(host, slot)` collision fixture (`config-invalid-collision.yaml`)
  that fails with both colliding node names in the error message.
- [ ] 2.4 Add `Cluster.ImageDigests map[string]string` with key
  `<schematic>/<version>/<arch>` and value `sha256:<hex>`. Validator
  warns (does not fail) on missing entries; tpi Preflight fails closed
  on the missing entry. Acceptance: schema doc test; nostos refuses tpi
  install when digest is unpinned and prints the exact key the operator
  must add.
- [ ] 2.5 Add tp1 (192.168.68.107, arm64, worker, slot 1) and tp4
  (192.168.68.114, arm64, worker, slot 4) entries to
  `nostos/config.yaml` under `nodes:` with `boot.method: tpi` and
  `boot.tpi` block. **Do not add vm-pc01 or pc01** in v0.2 (no provider
  exists). Acceptance: `task nostos:status` (or equivalent list command)
  prints all three nodes (dell01, tp1, tp4); `nostos node show tp1`
  prints `host:` value but NOT the resolved password (FakeSecrets
  sentinel `fake-bmc-password-do-not-rotate` MUST NOT appear in
  stdout/stderr).
- [ ] 2.6 Document the new schema in `.submodules/nostos/README.md` and
  `nostos/README.md`. Acceptance: a copy-pasteable `boot.tpi` block plus
  a copy-pasteable `cluster.image_digests` entry; doctest extracts the
  block and runs it through the validator successfully.

## 3. tpi provider

- [ ] 3.1 Create `internal/provisioner/tpi/tpi.go` implementing
  `Provisioner`. `init()` calls `Register("tpi", New)`. `Method() ==
  "tpi"`; `ContentionKey(node) == "tpi:" + node.Boot.TPI.Host`.
  Acceptance: compliance suite (`provisionertest.RunComplianceSuite`)
  passes with no skips.
- [ ] 3.2 Implement image fetch + verify in `tpi/image.go`. URL =
  `https://factory.talos.dev/image/<schematic>/<version>/metal-<arch>.raw.xz`.
  Cache path =
  `~/.cache/nostos/images/<schematic>/<version>/metal-<arch>.raw.xz`.
  Verify against `cfg.Cluster.ImageDigests`. Idempotent (cache-hit
  short-circuit). Acceptance: `httptest` server; first call hits HTTP
  exactly once and writes file with mode 0600; second call hits HTTP
  zero times; bad-digest fixture causes the cached file to be deleted
  and the call to return a typed error citing both expected and actual
  hashes.
- [ ] 3.3 Decompress xz to sibling `.raw` using
  `github.com/ulikunitz/xz` (Go-native, **no** shell-out). Acceptance:
  unit test with a tiny synthetic `.xz` fixture in `testdata/images/`
  produces byte-identical output to `xz -d` reference.
- [ ] 3.4 Implement `Preflight`: `tpi --version` parses to >= minimum
  (placeholder `1.0.0`; revisit per D-Open Q1); TCP connect to
  `host:443` within 2s using `Clock` + `Commander`-mockable dialer;
  every `_ref` resolves; cache root has
  `>= max(image_size_compressed * 3, 8 GiB)` free. Acceptance: unit
  test with FakeCommander scripts each step exactly once; old-version
  case (`tpi --version` returns `0.4.0`) fails with `ErrPreflight`.
- [ ] 3.5 Implement `Boot`: materialize `IdentityFileRef` (if set) at
  `~/.cache/nostos/secrets/<run-id>/tpi-key` with `O_CREAT|O_EXCL`
  mode 0600 inside a 0700 dir; `lstat` to refuse symlinks. Set
  `TPI_USERNAME` / `TPI_PASSWORD` on `Cmd.Env` (never argv). Run
  `power off` (treat "already off" stderr as non-fatal — pin the
  matched substring), then `flash`, then `power on`. Acceptance:
  FakeCommander captures all three calls in order; argv contains NO
  occurrence of the resolved password value (regex assertion on full
  argv string); env contains `TPI_PASSWORD=<value>`; key file at
  expected path with mode 0600 and parent dir 0700; "already off" path
  proceeds; symlink-at-target rejects.
- [ ] 3.6 Implement `WaitMaintenance`: poll `talosctl --insecure -n
  <ip> version` every 5s (FakeClock) until parseable response or ctx
  deadline. Default deadline 20 min. Acceptance: timeout test returns
  `ErrTimeout`; success test parses a fixture `talosctl version`
  output; uses Clock seam (no `time.Sleep`).
- [ ] 3.7 Implement `Apply`: `talosctl apply-config -i -n <ip> --file
  <configPath>`. Acceptance: FakeCommander records exact argv;
  `configPath` exists at mode 0600 when subprocess is invoked; orchestrator
  unlinks it after Apply returns (test asserts file is gone after
  `Install`).
- [ ] 3.8 Implement `Cleanup`: on prior error, `tpi power off -n
  <slot>` with 60s deadline (single try). Always: unlink any
  materialized key file under `~/.cache/nostos/secrets/<run-id>/`;
  `os.RemoveAll` of that dir. Idempotent (second call is a no-op).
  Acceptance: orchestrator-side test injects an error in `Boot`;
  `Cleanup` is called with a non-cancelled context (parent ctx is
  cancelled); FakeCommander records `tpi power off`; key file is
  unlinked; second `Cleanup` returns nil.
- [ ] 3.9 Stream `tpi` stdout into emit() with a 200ms coalescing
  window using `Clock`. Acceptance: scripted 30s synthetic stdout at
  1ms tick produces ≤ 151 emits (pin the ceiling, drop the tilde);
  every emitted message passes through the Scrubber before reaching
  `EventEmitter`.

## 4. Orchestrator refactor

- [ ] 4.1 Move PXE-specific code from
  `internal/cluster/orchestrate.go` (lines 103-205) into
  `internal/provisioner/pxe/pxe.go`. Define `Apply` as a no-op (PXE
  delivers config in-band via the iPXE chain). HTTP-request tap is a
  goroutine owned by the provider, joined in `Cleanup`. Acceptance:
  dell01 install through `pxe` provider produces the **topologically
  ordered subsequence** `[info, progress, download (kernel),
  download (initramfs), config-fetched, apid-up, ready]` with no
  `KindError`; assertion uses `assertSubsequence`, NOT byte-equal
  golden.
- [ ] 4.2 Rewrite `cluster.Install` per design D2: Preflight,
  ResolveSecrets (feed Scrubber), live-node reinstall guard, Render
  to 0600 temp file, ContentionKey acquire, Prepare, Boot,
  WaitMaintenance, Apply, WaitApid, Bootstrap (controlplane).
  `Cleanup` runs in a deferred block with a fresh
  `context.Background`-derived 60s context. Acceptance: orchestrator
  unit test with `FakeProvisioner` asserts each phase ran in order;
  Cleanup runs on every error path including ctx cancel and panic;
  Render-to-temp file is mode 0600 and unlinked after Install.
- [ ] 4.3 Remove the v0.1 inline `talosctl apply-config -i` from PXE
  path. PXE `Apply` is no-op. tpi `Apply` invokes it. Acceptance:
  FakeCommander watching the PXE install records ZERO `talosctl
  apply-config -i` invocations; same install via tpi records exactly
  one; argv passed to talosctl includes `--file <path>` where path is
  the orchestrator's 0600 temp file.
- [ ] 4.4 Wire `contention.Acquire(prov.ContentionKey(node))` between
  Boot and Apply (released after Apply or in Cleanup). Acceptance:
  barrier-based concurrency test — two installs sharing
  `tpi:192.168.68.10` serialize; two installs with distinct keys
  overlap. No sleeps in the test.
- [ ] 4.5 Wire `flock.AcquireNode(name)` at the top of `Install`.
  Acceptance: spawn child holding the lock; parent invocation returns
  `ErrLocked` within 100ms with a message naming the lockfile path.
- [ ] 4.6 Live-node reinstall guard: probe `talosctl --insecure -n
  <ip> version` (then secured `talosctl version`); if Ready, return
  `ErrNodeAlreadyReady` unless `opts.Reinstall` is true. Acceptance:
  FakeCommander returns a healthy response → install fails fast
  without invoking any provider hook except `Preflight`.
- [ ] 4.7 Wrap `events` channel with `redact.WrapEmitter(events,
  scrub)` after secret resolution; provider emits all flow through
  Scrubber. Acceptance: planted-secret integration test — resolved
  secret value is `S3CR3T-DO-NOT-LEAK`; provider emits a message
  containing that substring; the events channel observer never sees
  the substring (sees `[REDACTED]` or equivalent).

## 5. CLI

- [ ] 5.1 Add `internal/cli/node_install.go`: `cobra` command `nostos
  node install <name> [--reinstall] [--yes]`. Acceptance: `nostos
  node install --help` lists the flags; against a fake registry,
  invoking calls `Install` exactly once with the expected
  `*config.Node`.
- [ ] 5.2 Keep `internal/cli/up.go` as a thin alias that calls the
  same runner. Acceptance: `nostos up dell01` invokes the same
  `Install` code path (asserted by a single counter incremented in
  the runner); stderr matches the regex
  `^deprecated: nostos up; use 'nostos node install <name>'$`.
- [ ] 5.3 Update Taskfile wrappers: `task nostos:install NODE=<name>`
  shells `go run ./.submodules/nostos/cmd/nostos node install <name>`.
  Old `taskfiles/turing.yml::flash`, `download`, `install-talos`
  recipes' bodies become `echo "deprecated: use 'task nostos:install
  NODE=<name>'" && exit 1` — **not** silent wrappers (old recipes do
  not go through the new secrets pipeline). Acceptance:
  `task nostos:install NODE=tp1` dispatches to the Go binary;
  `task turing:flash` exits 1 with the deprecation message.

## 6. Tests

- [ ] 6.1 Compliance suite (1.8) passes for `pxe` and `tpi` with no
  skips. Acceptance: `go test
  ./.submodules/nostos/internal/provisioner/...` green; `grep -R
  't.Skip' internal/provisioner` returns zero.
- [ ] 6.2 Subsequence-based regression for dell01 install via `pxe`
  provider. Acceptance: per 4.1 — required event-Kind subsequence
  observed; no `KindError`; the assertion is robust to extra
  intermediate progress events.
- [ ] 6.3 SIGKILL torture for any persistent-write paths in v0.2 (the
  per-node lockfile and the rendered-config temp file). Acceptance:
  property test (≥100 iterations) — child writes the lockfile,
  `SIGKILL` mid-run, parent observes lockfile then releases stale
  lock cleanly (or test is skipped with reason if v0.2 chooses
  PID-tagged locks instead of stale-lock recovery).
- [ ] 6.4 Backwards-compat fixture: pin the literal repo
  `nostos/config.yaml`. Acceptance: `dell01.Boot.Method == "pxe"`
  by default; `tp1.Boot.Method == "tpi"`; `tp1.Boot.TPI.Host ==
  "192.168.68.10"` (or as committed); load → no error.
- [ ] 6.5 Hardware test evidence template:
  `.github/PULL_REQUEST_TEMPLATE/hardware.md` with fields
  `Hardware`, `Slot`, `Image SHA`, `Run ID`, `Log attachment`.
  Acceptance: file exists; CI check warns (does not fail) if a PR
  with the `hardware-tested` label has no filled template.
- [ ] 6.6 Integration test (build tag `integration && pxe`) — QEMU
  end-to-end. Acceptance: nightly self-hosted job runs and uploads
  the run artifacts; CI marks "skipped: no KVM" when `kvm-ok` fails
  (never silent pass).
- [ ] 6.7 Real-hardware test (build tag `integration && tpi`) gated
  by env vars `NOSTOS_TPI_HOST` + `NOSTOS_TPI_SLOT`. Acceptance:
  manual evidence via the hardware PR template (6.5); not run in CI.

## 7. Out of scope for v0.2 (each is its own future openspec change)

Recorded for traceability. **NOT** implemented in this change. The
schema does not reserve enum entries or sub-blocks for these.

- 7.1 `redfish` provider — v0.3.
- 7.2 `proxmox` provider — v0.3 (covers vm-pc01).
- 7.3 `usb` provider — v0.3 (covers pc01).
- 7.4 `rpi-imager` provider — v0.4.
- 7.5 JSONL run log + `nostos run logs <id>` — v0.3 (lands with
  `--resume`).
- 7.6 `inventory.db` (SQLite) — v0.3.
- 7.7 Drift detection + `nostos diff <node>` — v0.3.
- 7.8 `nostos node install --resume <run-id>` — v0.3.
- 7.9 `nostos doctor` (catalog + stub) — v0.4.
- 7.10 `cluster upgrade --to <ver>` — v0.4.
- 7.11 `secrets rotate` (covers Tailscale authkey rotation) — v0.4.
- 7.12 `--parallel` flag — v0.3 (locks ship in v0.2; flag does not).
- 7.13 Vendored iPXE binaries, Homebrew tap, container image — v1.0.
