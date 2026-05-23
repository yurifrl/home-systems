## 1. Provisioner package skeleton

- [ ] 1.1 Create .submodules/nostos/internal/provisioner/provisioner.go with the Provisioner interface, NodeView, BootConfig, EventEmitter type alias. Acceptance: package compiles with go vet ./.submodules/nostos/internal/provisioner/... clean.
- [ ] 1.2 Create internal/provisioner/registry.go with Register / New / Method() lookup, plus a duplicate-method panic in Register. Acceptance: unit test asserts Register panics on duplicates and New returns a typed not-registered error for unknown methods.
- [ ] 1.3 Create internal/provisioner/view.go with ViewFrom(cfg, node, name) returning a NodeView. Acceptance: round-trip test - building a NodeView from a fixture Config matches expected fields.
- [ ] 1.4 Create internal/provisioner/errors.go with sentinel errors (ErrPreflight, ErrBoot, ErrTimeout) so callers can errors.Is them. Acceptance: errors.Is round-trip test passes.
- [ ] 1.5 Create internal/runlog/runlog.go (JSONL writer at ~/.local/state/nostos/runs/<run-id>.jsonl) + Tee(emit, log). Acceptance: writing N events produces N JSONL lines, last line is the most recent.
- [ ] 1.6 Add a redaction lint test in internal/runlog/redact_test.go that scans every emitted message for op://, sops://, and a list of known secret-shaped substrings. Acceptance: test fails when a planted secret value lands in a log; passes on clean inputs.

## 2. Config schema v2 (additive)

- [ ] 2.1 Edit internal/config/config.go: add optional Boot struct on Node with Method enum (pxe|tpi|redfish|proxmox|usb|rpi-imager) and pointer sub-blocks (TPI, Redfish, Proxmox, USB, RPi). Default Method=pxe when block is omitted. Acceptance: existing nostos/config.yaml (only dell01) loads without error.
- [ ] 2.2 Add validation rules: Method enum required; if Method==tpi, Boot.TPI must be non-nil and contain Host + Slot + (UsernameRef + PasswordRef) | IdentityFileRef. Acceptance: unit tests cover valid and each invalid permutation; error messages cite the failing field path.
- [ ] 2.3 Add nostos/config.yaml entries for tp1 (192.168.68.107, arm64, worker, slot 1), tp4 (192.168.68.114, arm64, worker, slot 4), and vm-pc01 (192.168.68.102, amd64, worker) with boot.method=tpi (tp1, tp4) and boot.method=proxmox (vm-pc01, install deferred to v0.3). Acceptance: nostos node list prints all four nodes; nostos node show tp1 shows the BMC host (no password value).
- [ ] 2.4 Reject inline credential values (e.g. boot.tpi.password literal). Validator MUST fail on any field whose name does not end in _ref but contains a credential-shaped value. Acceptance: unit test pins this guard.
- [ ] 2.5 Document the new schema in .submodules/nostos/README.md and in nostos/README.md (consumer copy). Acceptance: README includes a copy-pasteable boot.tpi block.

## 3. tpi provider (v0.2 deliverable)

- [ ] 3.1 Create internal/provisioner/tpi/tpi.go implementing the Provisioner interface. Register in init() with method=tpi. Acceptance: registry test sees tpi after blank-import.
- [ ] 3.2 Create internal/provisioner/tpi/image.go that derives the factory.talos.dev URL from cluster.SchematicID + TalosVersion + arch and downloads to ~/.cache/nostos/images/<schematic>/<version>/<file>. Idempotent: skip when sha256 matches cached. Acceptance: unit test with httptest server confirms download + skip-on-cached behavior.
- [ ] 3.3 Decompress xz to sibling .raw file using a streaming decoder (xz Go module or shelled-out xz -d). Acceptance: test with a small xz fixture produces matching bytes.
- [ ] 3.4 Implement Preflight: tpi --version succeeds; TCP-connect to tpi.Host:443 within 2s; secrets refs resolve; cache dir has > 4 GiB free. Acceptance: unit test with mocked Commander + secrets resolver.
- [ ] 3.5 Implement Boot: power off slot, flash image, power on slot. Use TPI_USERNAME / TPI_PASSWORD env (set on the Cmd, not in argv). Acceptance: captured argv contains no resolved password value; env contains the value.
- [ ] 3.6 Implement WaitMaintenance: poll TCP nv.IP:50000 every 5s up to deadline. Acceptance: timeout test returns provisioner.ErrTimeout.
- [ ] 3.7 Implement Cleanup: on prior error, tpi power off slot. Always close any open log/file handles. Acceptance: orchestrator-side test asserts Cleanup runs even when Boot returns an error.
- [ ] 3.8 Stream tpi stdout into emit() with a 200ms throttle to avoid event flood. Acceptance: a 30s synthetic stdout stream produces fewer than ~150 emits.

## 4. Orchestrator refactor to use Provisioner

- [ ] 4.1 Move PXE-specific code from internal/cluster/orchestrate.go (lines 103-205, see design D2) into a new internal/provisioner/pxe/pxe.go that satisfies the Provisioner interface. Acceptance: dell01 golden Event sequence matches pre-refactor (testdata/golden/dell01-install.events.json).
- [ ] 4.2 Rewrite cluster.Install per design D2: Preflight, Prepare, Boot, WaitMaintenance, ApplyConfigInsecure, WaitApid, Bootstrap, Cleanup (deferred). Acceptance: orchestrator unit test with FakeProvisioner asserts each phase ran in order; Cleanup runs on error.
- [ ] 4.3 Add cluster.ApplyConfigInsecure wrapping talosctl apply-config -i with the rendered config path. Acceptance: subprocess argv recorded by Commander mock matches expected; configurable timeout.
- [ ] 4.4 Wire BMCSemaphore keyed by Provisioner.BMCKey(). Empty key bypasses. Acceptance: unit test with two installs sharing a BMC key serializes (second blocks until first releases).
- [ ] 4.5 Tee every emit through runlog so every install produces ~/.local/state/nostos/runs/<id>.jsonl. Acceptance: integration-style test reads the file back and compares against captured emits.

## 5. CLI: nostos node install branches on boot.method

- [ ] 5.1 Add internal/cli/node_install.go. cobra command nostos node install <name> resolves the node, calls cluster.Install. Acceptance: nostos node install --help is wired; invoking against a fake registry calls Install once.
- [ ] 5.2 Keep internal/cli/up.go as an alias that calls the node_install runner; print a one-line deprecation note when invoked. Acceptance: nostos up dell01 still works; stderr includes the word deprecated.
- [ ] 5.3 Add --parallel <n> flag (defaults to 1; flag hidden if v0.2 keeps it disabled). Acceptance: --parallel 2 returns parallel installs land in v0.3 in v0.2 builds.
- [ ] 5.4 Add nostos run logs <id> tailing the JSONL file. Acceptance: tails a synthetic file and prints lines as they appear.
- [ ] 5.5 Add stub commands nostos diff and nostos doctor that exit 2 with not yet implemented; see openspec/changes/nostos-v02-provisioners. Acceptance: cobra root --help lists them; invoking returns exit code 2.
- [ ] 5.6 Update Taskfile wrappers: nostos:install NODE=<name> calls go run ./.submodules/nostos/cmd/nostos node install <name>. Old turing.yml::flash etc print deprecated and call the new wrapper. Acceptance: task nostos:install NODE=tp1 dispatches to the Go binary; task turing:flash prints the deprecation line.

## 6. Tests

- [ ] 6.1 Unit tests per the breakdown in design D10 sections (registry, pxe wrapper, tpi provider, runlog, redaction lint, orchestrator). Acceptance: go test ./.submodules/nostos/... passes.
- [ ] 6.2 Golden Event-sequence test for dell01 install via the pxe provider. Acceptance: testdata/golden/dell01-install.events.json matches the FakeProvisioner-recorded event Kinds in order.
- [ ] 6.3 Integration test (build tag integration && tpi) that flashes a real Turing Pi slot. Acceptance: manual evidence in PR description; not run in CI.
- [ ] 6.4 Backwards-compat test: load nostos/config.yaml from this repo (committed) and assert Boot defaults to method=pxe for dell01 and method=tpi for tp1/tp4. Acceptance: test pins the regression.

## 7. Deferred to v0.3+

These are listed for traceability; NOT implemented in this change. Each becomes its own openspec change.

- [ ] 7.1 redfish provider (gofish-based). v0.3.
- [ ] 7.2 proxmox provider (go-proxmox-based). v0.3.
- [ ] 7.3 usb provider (operator-driven dd flow). v0.3 (covers pc01).
- [ ] 7.4 rpi-imager provider. v0.4.
- [ ] 7.5 inventory.db (SQLite at ~/.local/state/nostos/inventory.db). v0.3.
- [ ] 7.6 Drift detection + nostos diff <node>. v0.3.
- [ ] 7.7 nostos node install --resume <run-id>. v0.3.
- [ ] 7.8 nostos doctor full check catalog (BMC, secrets, disk, MAC, version, time). v0.4.
- [ ] 7.9 cluster upgrade --to <ver> (rolling reboot driven by provisioner.Boot). v0.4.
- [ ] 7.10 secrets rotate / check. v0.4.
- [ ] 7.11 Long-running daemons nostos-pxe + nostos-bmc with gRPC. v0.4.
- [ ] 7.12 Vendored iPXE binaries (drop Docker), Homebrew tap, container image, optional Talos system extension. v1.0.
