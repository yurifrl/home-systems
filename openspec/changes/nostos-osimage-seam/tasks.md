# Implementation Tasks

## 0. Pre-flight: stabilize the tree

- [x] 0.1 Resolve the dangling `internal/cli/flash.go` state (references undefined `runFlashProxmox`/`emitFlashProxmoxDryRun`): revert the half-applied proxmox branch so `go build ./...` is green before refactoring
- [x] 0.2 Confirm baseline: `go test ./...` passes in `.submodules/nostos` (this suite is the regression net for the refactor)

## 1. P1 — Introduce the seam (no behavior change)

- [x] 1.1 Create `internal/osimage/osimage.go`: `OSImage` interface (`Name`, `Resolve`, `NodeConfig`, `NetbootScript`, `FlashPlan`), `Ref` and `FlashPlan`/`FilePart` value types, and a `Register`/`New`/`For` registry mirroring `internal/provisioner`
- [x] 1.2 Add `osimage.For(node)` selection defaulting to `talos`; unknown OS returns a not-found error
- [x] 1.3 Create `internal/osimage/talos` impl that wraps existing code: `Resolve` (factory schematic/version/arch), `NodeConfig` (delegate to `registry.Render`), `NetbootScript` (existing iPXE template), `FlashPlan` (raw image + machineconfig sidecar + RPi EEPROM); register in `init()`
- [x] 1.4 Route `render` through `osimage` (done in P2 §2.3 alongside dropping the registry guard): render now calls `osimage.For(node).NodeConfig`; talos returns bytes (+ path via `registry.ConfigPath`), proxmox returns nil → "no machineconfig" message. `--no-validate` preserved via a `validate` param on `NodeConfig`.
- [x] 1.5 Route the talos `pxe serve` script path through `osimage` (done in P2 §2.4): unified per-MAC override map built via `osimage`; talos intentionally keeps the build-time boot.ipxe to preserve the wipe re-render.
- [x] 1.6 Unit tests for the registry (default talos, explicit select, unknown rejected, new-OS-registers-without-callsite-change); confirm existing Talos tests still pass

## 2. P2 — Move Proxmox behind the seam

- [x] 2.1 Create `internal/osimage/proxmox` impl: `Resolve` (latest|pinned via the existing resolver), `NetbootScript` (sanboot the ISO), `FlashPlan` (single ISO part), `NodeConfig` returns nil; register in `init()`
- [x] 2.2 Have it consume (or absorb) `internal/netboot/proxmox` resolver + `DownloadISO`; keep the offline fixture tests passing
- [x] 2.3 Delete the six OS conditionals: `config` `PXETarget`/`EffectiveTarget` branch, `cli/flash.go` (×2), `cli/commands.go` proxmox-script build, `registry/registry.go` render refusal — replaced by `osimage` routing
- [x] 2.4 `pxe serve` proxmox dispatch now flows entirely through `osimage.For(node).NetbootScript`; serve handler has no `if proxmox`
- [x] 2.5 Serve-path tests updated: proxmox MAC gets sanboot script, talos MAC unchanged, both via `osimage`

## 3. P3 — OS-agnostic writer

- [x] 3.1 Refactor `internal/image.Builder` into a writer that consumes a `FlashPlan` + destination (file|device), preserving ModeFile/ModeDevice, xz-decompress, and the compress-incompatible-with-device check
- [x] 3.2 Move the Talos machineconfig sidecar + RPi EEPROM emission out of the writer into `osimage/talos`'s `FlashPlan` parts
- [x] 3.3 Wire `cli/flash.go` to: `img := osimage.For(node)` → `Resolve`/`Ensure` → `FlashPlan` → `imagewriter.Write(plan, dest)`; no OS branch (this replaces the reverted 0.1 code)
- [x] 3.4 Verify `flash` for a talos node still produces image + sidecar (+ EEPROM for rpi); add a proxmox-node flash test asserting a single ISO part, no sidecar
- [x] 3.5 Update `flash` dry-run to render the plan from `osimage` (talos and proxmox), no per-OS branch

## 4. P4 — Config migration + docs

- [x] 4.1 Add node-level `os: {name, version}` to `internal/config`; default `name: talos`; require `version` when non-talos; validate proxmox version (`latest` | `^\d+\.\d+-\d+$`); reject removed `boot.pxe.target`/`boot.pxe.version`
- [x] 4.2 Update config tests: absent os → talos; proxmox+latest; proxmox+pinned; proxmox missing version (fails); malformed version (fails); legacy `boot.pxe.target` rejected
- [x] 4.3 Migrate pc01 in `nostos/config.yaml` from `boot.pxe.*` to `os: {name: proxmox, version: latest}`
- [x] 4.4 Update `internal/cli/schema/schema.go` (document the `os:` block) and the nostos README (`os:` schema, latest behavior, pc01 example); confirm via `nostos schema`
- [x] 4.5 Full suite green: `go test ./...` and `go vet ./...` in `.submodules/nostos`
