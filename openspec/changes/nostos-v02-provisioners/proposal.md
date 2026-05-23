## Why

`nostos` v0.1 (see `openspec/changes/nostos-v01/`) shipped a working bare-metal flow for **one** node — `dell01`, a Dell OptiPlex that PXE-boots from UEFI. The rest of this lab's nodes do not PXE:

- **tp1** (`192.168.68.107`) and **tp4** (`192.168.68.114`) — Turing Pi RK1 modules. They install via `tpi flash` over the BMC; PXE is not an option. The current flow is a hand-curated shell list in `taskfiles/turing.yml` that downloads a `metal-arm64.raw.xz`, calls `tpi flash`, then `talosctl apply-config -i`. Authkeys expired and they are sitting unjoined (see `nostos/README.md`).
- **vm-pc01** (`192.168.68.102`) — a Proxmox VM. Its install lives in `taskfiles/talos.yml` as a `talosctl apply-config -i` line with no orchestration around image creation, VM lifecycle, or maintenance-mode wait.
- **pc01** (`192.168.68.104`) — x86 box flashed manually via `dd`. The same `talos.yml::pc01-install` task hard-codes a `/dev/disk5` USB stick path and assumes the operator is at the keyboard.

`nostos` v0.1 is therefore a one-trick pony. The product vision was always *one tool, every node*, and `nostos/config.yaml` only listing `dell01` is the most visible symptom of the gap (`nostos/config.yaml:11-17`).

This change introduces a **Provisioner abstraction** so that `nostos node install <name>` works regardless of how the node actually boots. v0.2 ships the abstraction plus the **tpi** provider — the smallest slice that closes the most painful gap (RK1 reset). v0.3 / v0.4 / v1.0 extend the same interface to Redfish, Proxmox, USB, rpi-imager, drift detection, and a comprehensive `doctor`.

## What Changes

- Define a `Provisioner` interface in `.submodules/nostos/internal/provisioner/` with five lifecycle hooks: `Preflight`, `Prepare`, `Boot`, `WaitMaintenance`, `Cleanup`. The orchestrator loop in `.submodules/nostos/internal/cluster/orchestrate.go::Install` becomes provisioner-agnostic — it owns wipe queueing, config render, maintenance-mode handoff to `talosctl apply-config -i`, bootstrap, kubeconfig fetch — and delegates "how does this hardware get into maintenance mode" to a provider.
- Extend the `Node` struct (`.submodules/nostos/internal/config/config.go:43-51`) with an additive `Boot` block carrying `method` (one of `pxe|tpi|redfish|proxmox|usb|rpi-imager`) plus method-specific config. Backwards-compat: a node with no `boot` block defaults to `method: pxe`, so `nostos/config.yaml`'s existing `dell01` entry keeps working unchanged.
- Promote the existing PXE flow into a `pxe` provider that satisfies the same interface. v0.1 behavior — assets build, `state/pending-wipes.json`, `${next-server}` boot.ipxe, dnsmasq subprocess — is preserved verbatim, just behind the interface.
- Implement the **tpi** provider (v0.2 deliverable): given a node with `boot.method: tpi`, the provider downloads/builds the right Talos `metal-arm64.raw.xz` for the node's schematic + version, calls `tpi flash -n <slot>` and `tpi power on -n <slot>` against the configured BMC, and waits for apid on the node's static IP. This replaces `taskfiles/turing.yml` end to end.
- `nostos node install <name>` becomes the single user-facing install command (replacing v0.1's `nostos up <node>`; v0.1's name stays as an alias for one release for backwards-compat). Method dispatch happens inside the orchestrator.
- Add a JSONL run log at `~/.local/state/nostos/runs/<run-id>.jsonl` capturing every Event emitted during an install. v0.2 writes the log; v0.3 reads it for resume.
- Lay groundwork for `--parallel`: the provisioner registry exposes a `BMCKey()` so the orchestrator can serialize installs that hit the same BMC (a Turing Pi board has 4 slots behind one BMC; flashing two slots simultaneously is unsupported), while still parallelizing across distinct BMCs.

### v0.3 / v0.4 / v1.0 (sketched here, not implemented in this change)
- v0.3: `redfish` and `proxmox` providers; `~/.local/state/nostos/inventory.db` (SQLite) recording each install + drift snapshots; `nostos diff <node>` comparing rendered config vs. live machineconfig.
- v0.4: `cluster upgrade --to <ver>`, `secrets rotate`, comprehensive `nostos doctor` (BMC reachability, secret validity, disk size, MAC collision, version match).
- v1.0: stable CLI, man pages, Homebrew tap, container image, vendored iPXE binaries (kill the Docker requirement from v0.1), optional Talos system extension, hardware test matrix.

## Capabilities

### New Capabilities
- `provisioner` — Interface + registry for "how does this node get into Talos maintenance mode." Five lifecycle hooks. `pxe` and `tpi` providers ship in v0.2; `redfish`, `proxmox`, `usb`, `rpi-imager` in later releases.
- `tpi-provisioning` — Turing Pi BMC provider for RK1 (and any other Turing-Pi-hosted) modules. Wraps `tpi flash` + `tpi power` + image fetch. Replaces `taskfiles/turing.yml`.

### Modified Capabilities
- `pxe-provisioning` (from `nostos-v01/specs/pxe-provisioning/`) — Refactored to satisfy the `Provisioner` interface. **No external behavior change** for `dell01`. Internal seam moves; tests must keep passing.
- `cluster-control` — `nostos up <node>` is renamed to `nostos node install <name>` (alias kept for one release). The `Install` function in `internal/cluster/orchestrate.go` becomes provisioner-agnostic; the v0.1 PXE-specific steps (`pxe.RenderBootIpxe`, `pxe.NewServer`, HTTP-request tap) move into the `pxe` provider's `Boot`.
- `nostos-cli` — New verbs `nostos node install`, `nostos diff` (v0.3 stub in v0.2), `nostos doctor` (stub). Existing `nostos up`/`nostos wipe`/`nostos status` keep working.

## Impact

- **Code added:**
  - `.submodules/nostos/internal/provisioner/` — interface, registry, `NodeView` value type, shared error types.
  - `.submodules/nostos/internal/provisioner/pxe/` — wraps existing `internal/pxe/` calls behind the interface.
  - `.submodules/nostos/internal/provisioner/tpi/` — calls `tpi` CLI subprocess; downloads + caches per-schematic `metal-arm64.raw.xz`.
  - `.submodules/nostos/internal/runlog/` — JSONL writer at `~/.local/state/nostos/runs/`.
- **Code changed:**
  - `internal/config/config.go` — add optional `Boot` struct on `Node`. Validation ensures `method` is in the supported enum and method-specific block is present.
  - `internal/cluster/orchestrate.go` — `Install` is restructured around the provisioner lifecycle. PXE-specific code moves out.
  - `internal/cli/up.go` — kept as alias; new `internal/cli/node_install.go` is the canonical entry.
- **Code unchanged:** `internal/secrets/`, `internal/registry/render.go`, `internal/cluster/bootstrap.go`, `internal/cluster/cert.go`. These are already provisioner-agnostic.
- **Consumer config:** `nostos/config.yaml` gains optional `boot:` blocks for tp1, tp4, vm-pc01 (added under section 2 of tasks.md). `dell01` entry stays as-is.
- **Replaced flows:** `taskfiles/turing.yml::flash`, `taskfiles/turing.yml::download`, `taskfiles/turing.yml::install-talos` and `taskfiles/talos.yml::apply` (worker rows) are deprecated in favor of `task nostos:install NODE=<name>`. The Taskfile entries remain as one-line wrappers calling `go run ./.submodules/nostos/cmd/nostos node install <name>`.
- **Runtime externals (new):** `tpi` CLI (already on operator's laptop per `taskfiles/turing.yml`) for v0.2. `gofish` (Redfish) and Proxmox API client land in v0.3 as Go libraries — no new CLI dependencies.
- **State:** New paths under `~/.local/state/nostos/` — `runs/<id>.jsonl` (v0.2), `inventory.db` (v0.3). Per the v0.1 invariant, these are caches: deletable, rebuildable from `config.yaml` + secrets backend.
- **Backwards-compat:** Operators with a v0.1 `config.yaml` and v0.1 `nostos up dell01` muscle memory keep working. `boot.method` defaults to `pxe`. `nostos up` aliases to `nostos node install`.

## What This Is Not (Non-Goals)

- **Not phone-home / SaaS.** Inventory.db is local. No metrics shipped anywhere.
- **Not multi-operator.** The SQLite inventory assumes one operator; concurrent writers from two laptops are out of scope.
- **Not a BMC manager.** nostos calls BMCs to install Talos; it does not own iLO/iDRAC user management, Turing Pi cluster admin, or Proxmox cluster operations.
- **Not a Talos config DSL.** Templates remain plain Talos machineconfig YAML with `op://` refs. The Provisioner interface only governs *how the node receives* the rendered config, not *what the config says*.
- **Not parallel-everywhere.** v0.2 ships the BMC contention model but `--parallel` itself is gated behind a flag and disabled by default until v0.3 has tested it on real hardware.
- **Not a credential vault.** BMC creds live behind the existing secrets backend (`op://`, `sops`, etc.) — never inline in `config.yaml`, never in the JSONL run log.
- **Not a replacement for `talosctl`.** Maintenance-mode handoff (`talosctl apply-config -i`) and `talosctl bootstrap` remain the cluster-control primitives. The provisioner only delivers the node *to* maintenance mode.
