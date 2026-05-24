## Why

`nostos` v0.1 (see `openspec/changes/nostos-v01/`) shipped a working
bare-metal flow for **one** node — `dell01`, a Dell OptiPlex that PXE-boots
from UEFI. The rest of this lab's nodes do not PXE:

- **tp1** (`192.168.68.107`) and **tp4** (`192.168.68.114`) — Turing Pi
  RK1 modules (Rockchip RK3588 SoM, arm64). They install via `tpi flash`
  over the Turing Pi BMC; PXE is not an option. The current flow is a
  hand-curated shell list in `taskfiles/turing.yml` that downloads a
  `metal-arm64.raw.xz`, calls `tpi flash`, then `talosctl apply-config -i`.
  Authkeys expired and they sit unjoined (see `nostos/README.md`).
- **vm-pc01** (`192.168.68.102`) — a Proxmox VM. Out of scope until v0.3
  ships the `proxmox` provider.
- **pc01** (`192.168.68.104`) — x86 box flashed manually via `dd`. Out
  of scope until v0.3 ships the `usb` provider.

`nostos/config.yaml` only listing `dell01` is the most visible symptom of
the gap (`nostos/config.yaml:11-17`).

This change introduces a **Provisioner abstraction** so that
`nostos node install <name>` works regardless of how the node actually
boots. **v0.2 ships exactly: the Provisioner interface + tpi provider +
PXE rehome.** No more, no less. v0.3+ extends the same interface to other
methods.

## What Changes

- Define a `Provisioner` interface in `.submodules/nostos/internal/provisioner/`
  with five lifecycle hooks (`Preflight`, `Prepare`, `Boot`, `WaitMaintenance`,
  `Apply`) plus an always-called `Cleanup`. The orchestrator loop in
  `.submodules/nostos/internal/cluster/orchestrate.go::Install` becomes
  provisioner-agnostic and delegates "how does this hardware get into Talos
  maintenance mode" and "how is the rendered machineconfig delivered" to a
  provider.
- Extend the `Node` struct (`.submodules/nostos/internal/config/config.go:43-51`)
  with an additive `Boot` block carrying `method` (one of `pxe|tpi`) plus
  method-specific config. Backwards-compat: a node with no `boot` block
  defaults to `method: pxe`, so `nostos/config.yaml`'s existing `dell01`
  entry parses unchanged.
- Promote the existing PXE flow into a `pxe` provider that satisfies the
  same interface. v0.1 behavior — assets build, `state/pending-wipes.json`,
  `${next-server}` boot.ipxe, dnsmasq subprocess, in-band config delivery
  via the iPXE chain — is preserved verbatim. The PXE provider's `Apply`
  hook is a no-op because PXE delivers the rendered config in-band.
- Implement the **tpi** provider (key v0.2 deliverable): given a node with
  `boot.method: tpi`, the provider downloads/verifies/decompresses the
  Talos `metal-arm64.raw.xz` for the node's schematic + version, calls
  `tpi flash -n <slot>` and `tpi power on -n <slot>` against the
  configured BMC, waits for an authenticated `talosctl --insecure version`
  on the node's static IP, then runs `talosctl apply-config -i` (out-of-band
  delivery). This replaces `taskfiles/turing.yml` end to end.
- `nostos node install <name>` becomes the single user-facing install
  command. `nostos up` becomes a thin alias that calls the same code path
  (deprecation note printed); slated for removal in v0.3.
- Generalize hardware-resource contention behind `Provisioner.ContentionKey()`.
  Two installs whose providers return the same non-empty key serialize at
  the Apply boundary. Used now to prevent two slots on the same Turing Pi
  board flashing simultaneously and to single-thread PXE server startup.
  `--parallel` itself is **not** added in v0.2; the locks ship internally.
- Add a per-node flock at `nostos/state/configs/<name>.lock` held across
  Render → Apply, so two `nostos node install <name>` invocations on one
  workstation fail fast with a typed lockfile error.

### v0.3+ (sketched, NOT implemented)

- v0.3: `redfish`, `proxmox`, `usb` providers; method enum extends.
  `inventory.db` (SQLite) + `nostos diff <node>` (drift detection).
  JSONL run log + `nostos node install --resume`. `--parallel` flag.
- v0.4: `cluster upgrade --to <ver>`, `secrets rotate`, comprehensive
  `nostos doctor`. `rpi-imager` provider.
- v1.0: stable CLI, vendored iPXE binaries (kill Docker dep), Homebrew tap.

## Capabilities

### New Capabilities

- `provisioner` — Interface + registry for "how does this node get into
  Talos maintenance mode and how is the machineconfig delivered." Five
  lifecycle hooks plus `Cleanup`. Method enum is **{pxe, tpi}** in v0.2;
  added methods land in their own openspec changes.
- `tpi-provisioning` — Turing Pi BMC provider for RK1 (and any other
  Turing-Pi-hosted) modules. Wraps `tpi flash` + `tpi power` + image fetch.
  Replaces `taskfiles/turing.yml`.

### Modified Capabilities

- `pxe-provisioning` (from `nostos-v01/specs/pxe-provisioning/`) — Refactored
  to satisfy the `Provisioner` interface. **No external behavior change**
  for `dell01`: `Apply` is a no-op because PXE delivers config in-band via
  the iPXE chain. Internal seams move; the dell01 install must produce the
  same observable event-kind subsequence as v0.1.
- `cluster-control` — `nostos up <node>` is renamed to `nostos node install
  <name>` (alias kept until v0.3). The `Install` function in
  `internal/cluster/orchestrate.go` becomes provisioner-agnostic.
- `nostos-cli` — New verb `nostos node install`. Existing `nostos up`,
  `nostos wipe`, `nostos status` keep working.

## Impact

- **Code added:**
  - `.submodules/nostos/internal/provisioner/` — interface, registry,
    typed sentinel errors, `ContentionKey` semaphore, Scrubber sink for
    EventEmitter.
  - `.submodules/nostos/internal/provisioner/pxe/` — wraps existing
    `internal/pxe/` calls behind the interface. `Apply` is a no-op.
  - `.submodules/nostos/internal/provisioner/tpi/` — calls `tpi` CLI
    via the `Commander` seam; downloads + sha256-verifies + caches per
    `<schematic>/<version>/<arch>` Talos image.
  - `.submodules/nostos/internal/execx/` — `Commander` interface
    (mockable subprocess seam).
- **Code changed:**
  - `internal/config/config.go` — add `Boot` struct on `Node` with method
    enum **{pxe, tpi}** only. Add `cluster.image_digests` map for
    operator-pinned Talos image SHAs. Add `Ref` typed-string for `_ref`
    fields with a YAML unmarshaller that requires a known URI prefix.
  - `internal/cluster/orchestrate.go` — `Install` is restructured around
    the provisioner lifecycle. PXE-specific code moves out. Render runs
    after Preflight (no resolved secrets touch disk before cheap checks
    pass). Per-node flock guards Render → Apply.
  - `internal/cli/up.go` — kept as alias that calls the same runner;
    new `internal/cli/node_install.go` is the canonical entry.
- **Code unchanged:** `internal/secrets/`, `internal/registry/render.go`,
  `internal/cluster/bootstrap.go`, `internal/cluster/cert.go`.
- **Consumer config:** `nostos/config.yaml` gains optional `boot:` blocks
  for tp1, tp4 with `boot.method: tpi`. dell01 entry stays as-is. New
  `cluster.image_digests` map pins one digest per `(schematic, version,
  arch)` tuple in use.
- **Replaced flows:** `taskfiles/turing.yml::flash`, `download`,
  `install-talos` are deprecated. The old keys print
  `deprecated: use 'task nostos:install NODE=<name>'` and exit 1 (no
  silent wrapper-through; old recipes do not go through the new secrets
  pipeline). `taskfiles/talos.yml::pc01-install` is **not** changed in
  v0.2 (waits for v0.3 `usb` provider).
- **Runtime externals (new):** `tpi` CLI (already on operator laptop per
  `taskfiles/turing.yml`). `talosctl`, `op` (already required by v0.1).
  No new CLI deps.
- **State:** No new persistent state in v0.2. Image cache at
  `~/.cache/nostos/images/<schematic>/<version>/...` (deletable,
  rebuildable). Per-run secret materialization at
  `~/.cache/nostos/secrets/<run-id>/` (0700 dir, 0600 files), unlinked
  on Cleanup.
- **Backwards-compat:** Operators with a v0.1 `config.yaml` and v0.1
  `nostos up dell01` muscle memory keep working. `boot.method` defaults
  to `pxe`. `nostos up` aliases to `nostos node install` until v0.3.

## What This Is Not (Non-Goals for v0.2)

- **Not a multi-method shipping vehicle.** Method enum is `{pxe, tpi}`.
  Adding `redfish`/`proxmox`/`usb`/`rpi-imager` is each its own openspec
  change; v0.2 does NOT reserve schema bytes for them.
- **Not parallel.** No `--parallel` flag. Internal `ContentionKey` locks
  ship; concurrency surface stays serial.
- **Not a structured run log.** No JSONL. No SQLite. No `inventory.db`.
  No `--resume`. Operator can tee stderr if they want history. Run-log
  + resume land in v0.3 when `--resume` actually consumes them.
- **Not a doctor / diff catalog.** No `nostos doctor`, no `nostos diff`,
  no stub commands. Land them when implementations exist.
- **Not phone-home / SaaS / multi-operator.**
- **Not a BMC manager.** nostos calls BMCs to install Talos; it does not
  own iLO/iDRAC user management or Turing Pi cluster admin.
- **Not a Talos config DSL.** Templates remain plain Talos machineconfig
  YAML with `op://` refs.
- **Not a credential vault.** BMC creds live behind the existing secrets
  backend (`op://`, `sops://`, `file://`) — never inline, never in
  emitted events.
- **Not a Tailscale-authkey rotator.** v0.2 requires the operator to
  rotate the Tailscale authkey `op://` ref before invocation; the spec
  records the policy (single-use, TTL ≤ 1h) but the rotate hook lands
  in v0.4 alongside `secrets rotate`.
- **Not a replacement for `talosctl`.** `talosctl apply-config -i` and
  `talosctl bootstrap` remain primitives.
