## Context

nostos v0.1 (openspec/changes/nostos-v01/) shipped a single-method install
flow: PXE-boot a Dell OptiPlex from UEFI, render a per-MAC machineconfig,
serve over HTTP+dnsmasq, watch for the config fetch, wait for apid, bootstrap
etcd. End-to-end install lives in
.submodules/nostos/internal/cluster/orchestrate.go::Install (lines 67-235);
that function references pxe.BuildAll, pxe.RenderBootIpxe, pxe.NewServer
and the HTTP request tap directly. PXE is the only supported install path.

The lab has four other nodes that do not PXE:

- tp1, tp4 - Turing Pi RK1 ARM modules. Todays flow lives in
  taskfiles/turing.yml (flash, download, install-talos): download
  metal-arm64.raw.xz from factory.talos.dev, tpi flash -i ... -n <slot>,
  tpi power on -n <slot>, then talosctl apply-config -i. None of this is
  in nostos.
- vm-pc01 - Proxmox VM. taskfiles/talos.yml::apply does
  talosctl apply-config -f against an already-running VM with a
  pre-installed Talos disk image. VM lifecycle (create, attach ISO, boot) is
  manual.
- pc01 - x86 with NVIDIA GPU. taskfiles/talos.yml::pc01-install flashes
  a USB stick by dd to /dev/disk5 and then talosctl apply-config -i.

nostos/config.yaml only contains dell01 (lines 11-17); the workers are
not declared there because the v0.1 schema has no concept of a non-PXE node.
This is the gap v0.2 closes.

## Goals / Non-Goals

**Goals (v0.2):**
- Define a single Provisioner interface that all install methods satisfy.
- Refactor internal/cluster/orchestrate.go::Install to be provisioner-agnostic.
- Ship the tpi provider end-to-end so `task nostos:install NODE=tp1` replaces
  the entire taskfiles/turing.yml flow.
- Preserve dell01s behavior bit-for-bit by promoting v0.1s PXE code into a
  pxe provider.
- Lay groundwork (interface shape, run-log JSONL, BMC contention key) for
  v0.3 providers without implementing them.

**Non-Goals (v0.2 - deferred):**
- redfish, proxmox, usb, rpi-imager providers.
- inventory.db, drift detection, nostos diff.
- cluster upgrade, secrets rotate, comprehensive doctor.
- --parallel actually running parallel; the contention key ships, the flag
  defaults to off.
- Vendored iPXE / Homebrew / Talos system extension - v1.0 work.

## Decisions

### D1. Provisioner interface

Defined in internal/provisioner/provisioner.go:

    package provisioner

    import (
        "context"
        "time"
    )

    // NodeView is the immutable subset of a Node a provisioner needs.
    // Decouples providers from internal/config to keep the package importable
    // from tests without dragging in yaml + validator.
    type NodeView struct {
        Name        string
        MAC         string
        IP          string
        Role        string // "controlplane" | "worker"
        Arch        string // "amd64" | "arm64"
        InstallDisk string // /dev/...
        Template    string // path under templates/
        Boot        BootConfig
    }

    type BootConfig struct {
        Method string // pxe | tpi | redfish | proxmox | usb | rpi-imager
        TPI      *TPIBoot
        Redfish  *RedfishBoot
        Proxmox  *ProxmoxBoot
        USB      *USBBoot
        RPi      *RPiBoot
        // PXE has no extra fields - cluster-wide settings live in cfg.Cluster.
    }

    // Provisioner is one boot method.
    type Provisioner interface {
        // Method returns the boot.method string this provider handles.
        Method() string

        // BMCKey returns a string identifying the shared BMC for contention
        // purposes. Two installs with the same BMCKey are serialized.
        // Empty string means "no shared BMC" (PXE; USB; rpi-imager).
        BMCKey(NodeView) string

        // Preflight runs cheap checks: BMC reachable, image cached,
        // creds resolvable. Called before any side effects.
        Preflight(ctx context.Context, n NodeView) error

        // Prepare does idempotent prep work: download/build images, render
        // boot artifacts, queue any one-shot wipe flags. Safe to re-run.
        Prepare(ctx context.Context, n NodeView, emit EventEmitter) error

        // Boot kicks the node toward Talos maintenance mode (tpi flash +
        // power-on; redfish virtual-media + reboot; pxe.Server.Start; etc).
        // Returns when the boot signal has been sent, NOT when the node is
        // up. Long-running boot servers (PXE) are kept alive via a context
        // owned by the orchestrator and torn down by Cleanup.
        Boot(ctx context.Context, n NodeView, emit EventEmitter) error

        // WaitMaintenance blocks until the node is reachable in maintenance
        // mode (apid up at NodeView.IP, no machineconfig applied yet) or
        // the deadline expires.
        WaitMaintenance(ctx context.Context, n NodeView, emit EventEmitter) error

        // Cleanup tears down provider resources (PXE server stop; restore
        // boot.ipxe; clear pending wipe entry; close BMC sessions).
        // Always called, even on error in earlier phases.
        Cleanup(ctx context.Context, n NodeView, emit EventEmitter) error
    }

    // EventEmitter is the same channel used by the orchestrator today.
    // Aliased here so providers do not import internal/cluster.
    type EventEmitter func(kind, message string)

A registry maps method strings to constructors:

    type Factory func(deps Deps) Provisioner
    type Deps struct {
        Cfg     *config.Config
        Paths   paths.Paths
        Secrets secrets.Resolver
        Cmd     execx.Commander // mockable subprocess interface
    }
    var registry = map[string]Factory{}
    func Register(method string, f Factory) { registry[method] = f }
    func New(method string, deps Deps) (Provisioner, error) { ... }

Each provider package (provisioner/pxe, provisioner/tpi) calls Register in
its init(). cmd/nostos/main.go blank-imports the providers it wants to ship
in this build, mirroring how database/sql drivers register.

### D2. Orchestrator reshape

internal/cluster/orchestrate.go::Install today is ~170 lines of linear PXE
plumbing (orchestrate.go:67-235). The reshaped function becomes:

    func Install(ctx, cfg, p, node, name, opts, events) error {
        emit := makeEmitter(events)
        nv := provisioner.ViewFrom(cfg, node, name)

        prov, err := provisioner.New(nv.Boot.Method, deps)
        if err != nil { return err }

        runID := runlog.NewID()
        rlog, _ := runlog.Open(runID)
        defer rlog.Close()
        // Tee every emit into the JSONL run log.
        emit = runlog.Tee(emit, rlog)

        // Always render machineconfig - every provisioner needs it
        // because maintenance-mode handoff is `talosctl apply-config -i`.
        if _, err := registry.Render(cfg, p, name, true); err != nil { ... }

        if err := prov.Preflight(ctx, nv); err != nil { return err }

        // Cleanup runs no matter what. Defer order: cleanup last (LIFO).
        defer prov.Cleanup(context.Background(), nv, emit)

        if err := prov.Prepare(ctx, nv, emit); err != nil { return err }
        if err := prov.Boot(ctx, nv, emit); err != nil { return err }
        if err := prov.WaitMaintenance(ctx, nv, opts.bootDeadline(), emit); err != nil { return err }

        // Maintenance-mode handoff: identical for every provider.
        if err := ApplyConfigInsecure(ctx, cfg, p, nv); err != nil { return err }

        // Wait for apid on the static IP after reboot into installed mode.
        if err := WaitApid(ctx, nv, opts.apidDeadline()); err != nil { return err }

        if nv.Role == "controlplane" {
            if err := Bootstrap(ctx, cfg, p, node, opts.BootstrapTimeout); err != nil { return err }
            _ = FetchKubeconfig(ctx, p, node)
        }
        emit("ready", "...")
        return nil
    }

The PXE-specific blocks at orchestrate.go:103-112 (BuildAll), 114-119 (wipe
re-render), 122-135 (server start + Preflight + HTTPRequests tap), 137-160
(GET /configs/<mac>.yaml watch), 195-205 (consume wipe + restore boot.ipxe)
all migrate to provisioner/pxe/*.go. The wipe queue itself
(internal/cluster/wipe.go) stays put because v0.3 may want it for redfish
too.

A new function `cluster.ApplyConfigInsecure` formalizes the existing
`talosctl apply-config -i` step that the workers do today via
taskfiles/talos.yml::apply. v0.1 PXE happened to include the config inline
in iPXE boot, so this step was implicit; in v0.2 every provider funnels
through it for symmetry.

### D3. Per-method designs

#### pxe (v0.2 - refactor only)

Implementation: provisioner/pxe/pxe.go. Wraps the existing internal/pxe code
1:1.

- Method: "pxe"
- BMCKey: "" (no BMC; PXE boot is initiated by the node, not by nostos)
- Preflight: pxe.NewServer().Preflight() (current orchestrate.go:124-129);
  detects ethernet interface, port 9080 free, dnsmasq binary present.
- Prepare: pxe.BuildAll (download kernel/initramfs, build iPXE), then
  registry.Render is called by the orchestrator. If the node has a wipe
  queued, re-render boot.ipxe with talos.experimental.wipe=system.
- Boot: srv.Start(serveCtx, ni). The HTTP-request tap (srv.HTTPRequests())
  becomes a goroutine inside the provider that emits download events.
- WaitMaintenance: poll registry.Probe for apid up at nv.IP.
- Cleanup: srv.Stop, ConsumeWipe on success, restore clean boot.ipxe.

#### tpi (v0.2 - new provider, key deliverable)

Implementation: provisioner/tpi/tpi.go. Replaces taskfiles/turing.yml.

Config block on Node:

    boot:
      method: tpi
      tpi:
        host: "192.168.68.10"        # Turing Pi BMC LAN IP
        slot: 1                       # 1..4
        # Authentication source. Either:
        username_ref: "op://kubernetes/turingpi/username"
        password_ref: "op://kubernetes/turingpi/password"
        # ...or a key file:
        identity_file_ref: "op://kubernetes/turingpi/ssh_key"

- Method: "tpi"
- BMCKey: tpi.Host (every slot on the same board shares one BMC; serialize).
- Preflight:
  - tpi --version succeeds (binary present on operator laptop).
  - TCP connect to tpi.Host:443 within 2s.
  - Secrets backend resolves credential refs (no value logged).
  - Image cache directory has > 4 GiB free.
- Prepare:
  - Compute image URL from cfg.Cluster.SchematicID + TalosVersion + arch:
    https://factory.talos.dev/image/<schematic>/<version>/metal-arm64.raw.xz
  - Download to ~/.cache/nostos/images/<schematic>/<version>/metal-arm64.raw.xz
    (idempotent: skip if size + sha match).
  - xz -d to a sibling .raw file (sparse-aware).
  - Render machineconfig is done by the orchestrator already.
- Boot:
  - Set TPI_USERNAME / TPI_PASSWORD env (or --user/--password flags) from the
    resolved secrets, never inline in argv.
  - tpi --host <h> power off -n <slot>
  - tpi --host <h> flash -i <local.raw> -n <slot>     (this can take 5+ min)
  - tpi --host <h> power on -n <slot>
  - Stream tpi stdout; emit progress events.
- WaitMaintenance:
  - Poll TCP nv.IP:50000 (apid maintenance-mode listener) every 5s up to
    opts.bootDeadline (default 10 min).
- Cleanup:
  - On error: tpi power off -n <slot> (operator can retry).
  - Always: redact creds from emitted events; close any tpi log file handles.

#### redfish (v0.3 - sketch)

Use github.com/stmcginnis/gofish. Mount rendered machineconfig via virtual
media (Talos supports config injection through metadata server; for redfish
flow, ship a small one-shot ISO with the machineconfig embedded, attached as
virtual CD). Power-cycle. Wait for apid on static IP.

QUESTION: do we ship our own ISO builder or rely on talos-image factory? See
Open Questions Q1.

#### proxmox (v0.3 - sketch)

API client: github.com/luthermonson/go-proxmox. Provisioner manages the
full VM lifecycle for boot.method=proxmox: create or reuse VM, attach Talos
ISO from PVE storage, set cloud-init or smbios with the machineconfig
metadata URL, start VM. Wait for apid.

#### usb (v0.3 - sketch)

Operator-driven. nostos generates the right metal-<arch>.raw.xz, decompresses,
then prompts the operator (huh form) to insert a USB stick, picks the
device with diskutil/lsblk, runs dd with progress, and waits for the
operator to physically boot the target machine. WaitMaintenance is the same
TCP poll. No BMC.

#### rpi-imager (v0.4 - sketch)

For Raspberry Pi nodes: write the Talos ARM image plus boot config via the
rpi-imager headless API. Mostly a special-case of usb with Pi-specific
boot-firmware setup.

### D4. Concurrency and BMC contention

`nostos node install --parallel <n1> <n2> <n3>` runs in v0.3+. The shape that
ships in v0.2:

- The orchestrator owns a `BMCSemaphore` map keyed by Provisioner.BMCKey(nv).
  Two installs whose providers return the same non-empty key block on a
  shared sync.Mutex (or weighted semaphore of size 1). Empty keys mean no
  contention.
- For tpi specifically: a single Turing Pi board (one BMC) serializes its
  four slots. Two boards in the same lab parallelize freely.
- For pxe: BMCKey returns "" so multiple PXE installs could in principle
  run in parallel, BUT only one PXE server can bind dnsmasq + port 9080 +
  the tftp dir at a time. We model this as a separate keyed lock
  ("pxe:server"), held by the pxe providers Boot phase. So in v0.2 PXE
  installs are effectively serial; v0.3 may add a multiplexing PXE server.
- --parallel ships disabled in v0.2 (default 1). The locks ship; the user-
  facing flag does not. This avoids shipping untested concurrency on real
  hardware.

### D5. Resumability via JSONL run logs

internal/runlog/runlog.go writes one event per line to
~/.local/state/nostos/runs/<run-id>.jsonl. Format:

    {"run_id":"...","ts":"2026-05-23T08:14:01Z","node":"tp1","kind":"info","message":"queued ..."}
    {"run_id":"...","ts":"2026-05-23T08:14:02Z","node":"tp1","kind":"progress","phase":"prepare","message":"..."}

- Phase tag (preflight|prepare|boot|wait|apply|bootstrap|ready|error) is
  emitted at the start of each orchestrator phase so resume logic in v0.3
  can know where the previous run died.
- Secrets are NEVER written to the log. Provider implementations call
  emit() with already-redacted strings; the runlog package does no extra
  redaction (defence-in-depth via static lint test, see Testing Strategy).
- Logs are kept; rotation policy (delete > 30 days, > 100 runs) lands in
  v0.4 as part of a `nostos run gc` subcommand.
- v0.2 deliverable: write the log and tail it for `nostos run logs <id>`.
- v0.3 deliverable: `nostos node install --resume <run-id>` reads the log,
  determines the last completed phase, and re-enters the lifecycle from
  there. Provisioner methods are already idempotent so this requires no
  new interface surface.

### D6. Inventory schema (v0.3 deliverable, reserved here)

SQLite at ~/.local/state/nostos/inventory.db. modernc.org/sqlite (pure-Go,
no cgo) keeps `go run` cold-start fast.

Tables (DDL is illustrative; final shape lives in the v0.3 change):

    CREATE TABLE nodes (
      name        TEXT PRIMARY KEY,
      mac         TEXT NOT NULL,
      ip          TEXT NOT NULL,
      role        TEXT NOT NULL,
      arch        TEXT NOT NULL,
      boot_method TEXT NOT NULL,
      first_seen  TIMESTAMP,
      last_seen   TIMESTAMP,
      last_run_id TEXT
    );

    CREATE TABLE installs (
      run_id     TEXT PRIMARY KEY,
      node       TEXT REFERENCES nodes(name),
      started_at TIMESTAMP,
      ended_at   TIMESTAMP,
      result     TEXT, -- "ok" | "error" | "interrupted"
      message    TEXT
    );

    CREATE TABLE drift_snapshots (
      id          INTEGER PRIMARY KEY,
      node        TEXT REFERENCES nodes(name),
      taken_at    TIMESTAMP,
      rendered_sha256 TEXT,
      live_sha256     TEXT,
      diff_text   TEXT  -- empty when no drift
    );

Drift snapshot algorithm (v0.3): render machineconfig in-memory, fetch live
machineconfig via talosctl get mc -o yaml, normalize (sort keys, strip
volatile fields like timestamps), sha256 each, capture diff if differs.
Surfaced via `nostos diff <node>` and `nostos doctor`.

### D7. Security model

Trust boundaries:

- Operator laptop is trusted (already runs `op signin`).
- BMC LAN is **semi-trusted** at best. Turing Pi BMC ships with a default
  password; iLO/iDRAC are HTTPS-only but often with self-signed certs.
- Maintenance-mode Talos (apid pre-config) is **insecure** by design - this
  is the existing "first-boot insecure window" v0.1 already accepts. v0.2
  does not narrow it.

Rules:

- BMC creds are always resolved through `internal/secrets`, never read from
  config.yaml inline. Validation rejects any `boot.tpi.password:` literal.
- `_ref` suffix on every credential field is a static rule; the validator
  ensures the value matches the URI regex (op:// | sops:// | env:// | file://).
- Subprocess argv is constructed without secrets where the tool supports
  env vars (tpi accepts TPI_USERNAME / TPI_PASSWORD env). Where argv is
  unavoidable (e.g. some redfish CLI flows), we use stdin.
- Run logs and Event emissions go through redact.Strings() which scrubs any
  substring matching the resolved-secret values for the current run.
- machineconfig contents (which already contain decrypted secrets after
  rendering) live in nostos/state/configs/ at 0600 - same rule v0.1
  enforces. v0.2 ships a startup check that asserts perms on render and
  refuses to start serve if anything is world-readable.

### D8. Doctor checks catalog (v0.4 deliverable, sketched)

`nostos doctor` runs all of:

- BMC reachability for every node with boot.method in {tpi, redfish, proxmox}.
- Secret URI validity: every ref in config + templates resolves without value
  leakage.
- Disk size: install_disk exists on the node (talosctl get disks) and is at
  least 32 GiB.
- MAC collision: already in v0.1 config validation; doctor re-runs at
  cluster scope and checks against the live ARP table on the operators LAN.
- Version match: every node reports the cluster.talos_version.
- Time skew: < 60s drift on every node.

`--json` output is consumed by `nostos cluster status`.

### D9. Backwards-compat and migration

- Node gets an optional Boot block. Absent means method: pxe. dell01 existing
  entry (nostos/config.yaml:11-17) parses unchanged.
- nostos up <node> keeps working for one release as an alias for
  nostos node install <node>. After v0.3 we drop the alias and surface a
  one-line deprecation when invoked.
- taskfiles/turing.yml::flash, download, install-talos are rewritten as a
  single task nostos:install NODE=<name> wrapper. The old keys remain as
  deprecation aliases for one minor release; their cmds invoke the new task
  and print deprecated; use task nostos:install NODE=...
- taskfiles/talos.yml::apply (the workers row) follows the same pattern.
- taskfiles/talos.yml::pc01-install is NOT auto-migrated in v0.2 - pc01
  needs the usb provider which lands in v0.3. The task stays manual with a
  TODO referencing the v0.3 spec.
- v0.1 state directory layout is unchanged. Provisioner artifacts (image
  cache for tpi) live under ~/.cache/nostos/, not under the consumers
  state/, because they are not config-derived.
- v0.1 secrets backend interface is unchanged. New _ref fields plug in
  without backend changes.

Migration steps for the operator:

1. Update .submodules/nostos/ (this change adds files; no existing files
   change schema).
2. Edit nostos/config.yaml to add the workers under nodes: with a boot:
   block each. Reference template additions in tasks.md sec 2.
3. Run task nostos:install NODE=tp1. Operator confirms the destructive flash
   via interactive prompt (or --yes for scripted use).
4. Repeat for tp4 and (when v0.3 ships proxmox) vm-pc01.

### D10. Testing strategy

- Unit:
  - internal/provisioner/registry_test.go - Register/New/duplicate detection.
  - internal/provisioner/pxe/pxe_test.go - lifecycle calls thin wrappers
    around existing pxe code; mostly compile-time guarantees + a single
    integration-shaped test gated on Docker.
  - internal/provisioner/tpi/tpi_test.go - subprocess interface mocked via
    execx.Commander (already in v0.1 design D9). Cases:
      - happy flash + power-on emits expected event sequence
      - tpi flash failure surfaces error and triggers Cleanup power-off
      - secrets are not present in argv (assert via captured argv)
      - image already cached - skip download
  - internal/runlog/runlog_test.go - JSONL append + tail; redaction lint
    test that fails the build if any secret-shaped string lands in a log.
- Orchestrator:
  - internal/cluster/orchestrate_test.go (new) uses a fake Provisioner that
    records every method called. Asserts:
      - Cleanup runs on every error path (including ctx cancel)
      - Render happens before Boot
      - ApplyConfigInsecure runs once per install
      - On controlplane role, Bootstrap runs after WaitMaintenance returns
- Regression:
  - dell01 install path: a golden test running the orchestrator with the
    pxe provider against fake commands must produce the same Event sequence
    (same Kinds in same order) as the v0.1 implementation. Pin via
    testdata/golden/dell01-install.events.json.
- Integration:
  - //go:build integration && tpi: real tpi CLI against a lab Turing Pi.
    Manual evidence; not in CI.

### D11. Roadmap

- v0.2 (this change): Provisioner interface + registry. pxe provider
  (refactor). tpi provider (new). Orchestrator reshape. Run-log JSONL. BMC
  contention key. nostos node install command. Backwards-compat alias for
  nostos up. Workers added to nostos/config.yaml.
- v0.3: redfish + proxmox providers. inventory.db (SQLite). Drift detection
  + nostos diff. --parallel real implementation. usb provider for pc01.
  nostos node install --resume.
- v0.4: cluster upgrade --to <ver> (controlled rolling reboot via
  provisioner Boot calls). secrets rotate. Comprehensive doctor. nostos-pxe
  and nostos-bmc daemon split.
- v1.0: Stable CLI surface, man pages, Homebrew tap, container image,
  vendored iPXE binaries (kill Docker requirement), optional Talos system
  extension, hardware test matrix (RK1, RPi4/5, Dell, Proxmox, generic
  Redfish).

### D12. Open Questions

- QUESTION Q1. Redfish path for delivering machineconfig in maintenance
  mode: do we (a) build a one-shot ISO embedding the config and attach via
  virtual media, (b) rely on Talos metadata-server URL and host the
  rendered config at HTTP from nostos, or (c) post-boot via
  talosctl apply-config -i? Option (c) matches every other provider but
  requires the firmware to PXE-or-disk boot a generic Talos image first.
  Decide in v0.3.
- QUESTION Q2. Is BMCKey enough to model contention, or do we need a
  finer-grained resource concept? E.g. a Proxmox host with 12 VMs is
  effectively unlimited for VM-create but capped at the storage backend
  IO. Lean toward BMCKey is enough for v0.3; revisit if a real bottleneck
  surfaces.
- QUESTION Q3. Should the run-log JSONL be append-only across runs in one
  file (nostos.jsonl) or one file per run-id? Plan: one file per run-id
  for easy nostos run logs <id> and trivial cleanup. Confirm.
- QUESTION Q4. TPI image cache path: ~/.cache/nostos/images/ vs. consumer
  state/assets/. Lean cache (operator-wide), but if the operator manages
  multiple clusters with different schematics, cache key must include
  schematic_id. Confirmed in tasks.md sec 3.
- QUESTION Q5. Do we want a provisioner.Capabilities() method now (so the
  orchestrator can ask: does this provider support resume? does it require
  config render? does it leave artifacts in state/?) or wait until v0.3
  forces it? Lean: defer; YAGNI.
- QUESTION Q6. First-boot insecure window: tpi flash + reboot lands the
  node in maintenance mode with apid listening on every interface. Should
  we recommend a temporary firewall rule on the operator laptop side, or
  accept this is the same window v0.1 already has? Lean: same as v0.1,
  document in security spec.
