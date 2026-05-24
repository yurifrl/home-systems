## Context

nostos v0.1 (openspec/changes/nostos-v01/) shipped a single-method install
flow: PXE-boot a Dell OptiPlex from UEFI, render a per-MAC machineconfig,
serve over HTTP+dnsmasq, watch for the config fetch in the iPXE chain
(in-band delivery), wait for apid, bootstrap etcd. End-to-end install lives
in `.submodules/nostos/internal/cluster/orchestrate.go::Install` (lines
67-235). PXE is the only supported install path.

The lab has tp1 / tp4 (Turing Pi RK1, arm64) sitting unjoined, plus
vm-pc01 (Proxmox VM) and pc01 (x86 + NVIDIA) on manual Taskfile flows.
v0.2 closes the **tp1/tp4 gap only**. vm-pc01 and pc01 wait for v0.3
providers.

## Goals / Non-Goals

**Goals (v0.2):**
- Single `Provisioner` interface with pinned hook contracts.
- `internal/cluster/orchestrate.go::Install` is provisioner-agnostic.
- `tpi` provider end-to-end so `task nostos:install NODE=tp1` replaces
  `taskfiles/turing.yml`.
- dell01's behavior preserved bit-for-bit by promoting v0.1 PXE code into
  a `pxe` provider whose `Apply` is a no-op (config is delivered in-band
  via the iPXE chain).
- Three test seams (`Commander`, `Secrets`, `Clock`) and a provider
  compliance suite, so ~90% of spec scenarios are unit-testable.

**Non-Goals (v0.2 — deferred with reason in proposal.md):**
- `redfish`, `proxmox`, `usb`, `rpi-imager` providers and enum entries.
- JSONL run log, `inventory.db`, drift detection, `nostos diff`.
- `nostos doctor` (catalog or stub).
- `--parallel` flag (locks ship internally; flag does not).
- `--resume` and any persistent run history.
- Tailscale authkey auto-rotation.
- Vendored iPXE / Homebrew / Talos system extension.

## Definitions (glossary)

These terms are load-bearing and pinned here; specs cite this section.

- **RK1**: Turing Pi RK1 compute module (Rockchip RK3588, arm64). The
  `tpi` provider is not RK1-specific — it works for any module hosted on
  a Turing Pi 2 board — but the v0.2 hardware targets are RK1.
- **Maintenance mode**: Talos boot state where apid is exposed on TCP
  50000 without TLS client-cert authentication, prior to first
  machineconfig apply. Observable via `talosctl --insecure -n <ip>
  version` returning a parseable response. TCP-listening alone does
  NOT imply maintenance-mode-ready.
- **Ready**: For a worker, apid responds to `talosctl version` over the
  secured listener after Apply (i.e. `WaitApid` succeeds). For a
  controlplane, additionally etcd is healthy and `talos/kubeconfig` has
  been fetched. The orchestrator emits `Kind=ready` only when this
  condition holds.
- **Boot method**: the value of `Node.Boot.Method` in `nostos/config.yaml`.
  v0.2 enum is `{pxe, tpi}`.
- **Provisioner**: Go interface in `internal/provisioner/`. One method
  per boot method; one implementation per provider package.
- **Idempotent (per hook)**:
  - `Preflight`: no observable side effect; safe to re-run.
  - `Prepare`: converges to the same on-disk state across re-runs (cache
    hits short-circuit work).
  - `Boot`: NOT idempotent in the general case (a `tpi flash` is
    destructive every run; PXE Boot starts a server). Re-entry guarded
    by the orchestrator's per-node flock and the live-node reinstall
    guard.
  - `WaitMaintenance`: pure poll; safe to re-run.
  - `Apply`: each provider documents whether re-running is safe. PXE's
    no-op is trivially safe. tpi's `talosctl apply-config -i` succeeds
    once per maintenance-mode window; second call returns a typed
    error from talosctl which the provider must surface.
  - `Cleanup`: idempotent; safe to call twice.
- **ContentionKey**: string returned by a Provisioner for a given node
  identifying a shared scarce resource. Two installs with the same
  non-empty key serialize at the Apply boundary. Replaces v0.2-draft
  `BMCKey`; same shape, more honest name (PXE server contention is also
  modeled this way).
- **Boundary "in-band" vs "out-of-band" config delivery**: in-band =
  rendered config is fetched by the firmware/bootloader during the boot
  chain (PXE: `talos.config=http://...` in iPXE script). Out-of-band =
  delivered via `talosctl apply-config -i` after the node reaches
  maintenance mode (tpi). Each provider declares which by what it does
  in `Apply`.

## Decisions

### D1. Provisioner interface

Defined in `internal/provisioner/provisioner.go`:

    package provisioner

    import (
        "context"
        "time"
    )

    // Phase is one of these constants; orchestrator emits the phase
    // marker on transition.
    type Phase string
    const (
        PhasePreflight Phase = "preflight"
        PhasePrepare   Phase = "prepare"
        PhaseBoot      Phase = "boot"
        PhaseWait      Phase = "wait"
        PhaseApply     Phase = "apply"
        PhaseBootstrap Phase = "bootstrap"
        PhaseReady     Phase = "ready"
        PhaseError     Phase = "error"
        PhaseCleanup   Phase = "cleanup"
    )

    type Event struct {
        Phase   Phase
        Kind    string  // "info" | "progress" | "download" | "apid-up" | ...
        Message string
        At      time.Time
    }

    // EventEmitter is wrapped by the orchestrator with a Scrubber sink
    // before being passed to providers. Providers MUST NOT construct
    // their own emitters; they receive one.
    type EventEmitter func(Event)

    type Provisioner interface {
        Method() string

        // ContentionKey returns "" if no shared resource, else a stable
        // string. Two installs with the same non-empty key serialize
        // at the Apply boundary (acquired before Boot, released after
        // Apply or in Cleanup, whichever runs last).
        ContentionKey(node *config.Node) string

        // Preflight: cheap checks. No side effects. ctx is the run ctx.
        Preflight(ctx context.Context, node *config.Node, emit EventEmitter) error

        // Prepare: idempotent prep (image fetch, decompress, render
        // boot artifacts, queue wipe). Side effects allowed; safe to
        // re-run. ctx is the run ctx.
        Prepare(ctx context.Context, node *config.Node, emit EventEmitter) error

        // Boot: kick the node toward maintenance mode (tpi flash + power
        // on; pxe.Server.Start). Returns when the boot signal has been
        // sent, NOT when the node is up. Long-running servers (PXE) keep
        // running; their goroutines are owned by the provider and joined
        // in Cleanup. ctx is the run ctx.
        Boot(ctx context.Context, node *config.Node, emit EventEmitter) error

        // WaitMaintenance: blocks until the node is reachable in
        // maintenance mode (talosctl --insecure version parses) or ctx
        // deadline expires. ctx carries the deadline.
        WaitMaintenance(ctx context.Context, node *config.Node, emit EventEmitter) error

        // Apply: deliver the rendered machineconfig. PXE: no-op (config
        // already delivered in iPXE chain). tpi/redfish/proxmox/usb:
        // talosctl apply-config -i. configPath is a 0600 temp file owned
        // by the orchestrator and unlinked after Apply returns.
        Apply(ctx context.Context, node *config.Node, configPath string, emit EventEmitter) error

        // Cleanup: ALWAYS called. Receives a fresh ctx derived from
        // context.Background with a 60s deadline. May re-acquire
        // ContentionKey if non-empty to issue destructive teardown.
        // Idempotent.
        Cleanup(ctx context.Context, node *config.Node, emit EventEmitter) error
    }

A registry maps method strings to constructors:

    type Factory func(deps Deps) Provisioner
    type Deps struct {
        Cfg     *config.Config
        Paths   paths.Paths
        Secrets secrets.Resolver
        Cmd     execx.Commander // mockable subprocess seam
        Clock   clockx.Clock    // mockable time seam
    }
    var registry = map[string]Factory{}
    func Register(method string, f Factory) { ... } // panics on dup
    func New(method string, deps Deps) (Provisioner, error) { ... }

Method enum (validator-enforced): `{pxe, tpi}`. Providers register in
`init()`. `cmd/nostos/main.go` blank-imports the providers it ships;
unimplemented methods are not in the enum and fail validation closed.

**No `NodeView` value type.** Providers import `internal/config`
read-only and accept `*config.Node`. Decoupling-via-extra-type was a
duplication anti-pattern; the interface signatures themselves are the
seam.

### D2. Orchestrator reshape

`internal/cluster/orchestrate.go::Install` becomes:

    func Install(ctx, cfg, p, node, name, opts, events) error {
        // Wrap the raw event channel with a Scrubber seeded with the
        // resolved-secret table for THIS run. Providers cannot leak
        // secrets via emit() even if they try.
        scrub := redact.NewScrubber()
        emit := redact.WrapEmitter(events, scrub)

        prov, err := provisioner.New(node.Boot.Method, deps)
        if err != nil { return err }

        // Per-node flock for cross-process safety. Held Render -> Apply.
        unlock, err := flock.AcquireNode(name)
        if err != nil { return ErrLocked }
        defer unlock()

        // Phase: preflight (BEFORE any disk/secret side effect).
        if err := prov.Preflight(ctx, node, emit); err != nil { return err }

        // Resolve secrets ONCE; feed Scrubber.
        resolved, err := secrets.ResolveAll(ctx, node)
        if err != nil { return err }
        scrub.AddAll(resolved.Values())
        defer resolved.Zero()

        // Optional: live-node reinstall guard.
        if !opts.Reinstall {
            if alive, _ := cluster.ProbeReady(ctx, node.IP); alive {
                return ErrNodeAlreadyReady // require --reinstall
            }
        }

        // Render machineconfig to a 0600 temp file under
        // ~/.cache/nostos/secrets/<run-id>/. Unlinked at end.
        configPath, err := registry.RenderTo(cfg, p, name, secretsTempDir)
        if err != nil { return err }
        defer os.Remove(configPath)

        // Cleanup runs no matter what (LIFO defer).
        defer func() {
            cctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
            defer cancel()
            _ = prov.Cleanup(cctx, node, emit)
        }()

        // ContentionKey acquisition spans Boot..Apply.
        if key := prov.ContentionKey(node); key != "" {
            release := contention.Acquire(key)
            defer release()
        }

        if err := prov.Prepare(ctx, node, emit); err != nil { return err }
        if err := prov.Boot(ctx, node, emit); err != nil { return err }

        waitCtx, cancel := context.WithDeadline(ctx, opts.WaitMaintenanceDeadline())
        defer cancel()
        if err := prov.WaitMaintenance(waitCtx, node, emit); err != nil { return err }

        if err := prov.Apply(ctx, node, configPath, emit); err != nil { return err }
        if err := WaitApid(ctx, node, opts.ApidDeadline()); err != nil { return err }

        if node.Role == "controlplane" {
            if err := Bootstrap(ctx, cfg, p, node, opts.BootstrapTimeout); err != nil { return err }
            _ = FetchKubeconfig(ctx, p, node)
        }
        emit(Event{Phase: PhaseReady, Kind: "ready", Message: "...", At: time.Now()})
        return nil
    }

Key contracts:

- **No double-apply.** PXE's `Apply` is a no-op; tpi's `Apply` runs
  `talosctl apply-config -i`. The orchestrator does not call
  `apply-config -i` itself — that decision lives in the provider.
- **Render after Preflight, after secret resolution, before Prepare.**
  No resolved secrets touch disk before cheap checks pass.
- **Cleanup gets a fresh context** so it survives Ctrl-C of the run
  context. May re-acquire its `ContentionKey` to issue destructive
  teardown.
- **Scrubber is seeded once per run** with all resolved-secret values.
  Every provider emit and every captured child stdout/stderr passes
  through `scrub.Scrub(string)` before reaching events. Provider
  discretion is not part of the redaction story.

### D3. Per-method designs

#### pxe (refactor only)

Implementation: `internal/provisioner/pxe/pxe.go`. Wraps existing
`internal/pxe/` 1:1.

- `Method() = "pxe"`
- `ContentionKey() = "pxe:server"` (only one PXE server can bind
  dnsmasq + port 9080 at a time).
- `Preflight`: detect ethernet interface, port 9080 free, dnsmasq
  binary present.
- `Prepare`: `pxe.BuildAll`. Wipe re-render lives here (read pending
  wipes, re-render boot.ipxe with `talos.experimental.wipe=system`).
- `Boot`: `srv.Start(serveCtx, ni)`. The HTTP-request tap goroutine is
  owned by the provider; tied to a context that is cancelled in
  `Cleanup`. `Boot` returns when the server is listening.
- `WaitMaintenance`: poll `talosctl --insecure -n nv.IP version` until
  it parses or ctx deadline.
- `Apply`: **no-op** (config delivered in-band via iPXE chain). Returns
  nil immediately.
- `Cleanup`: `srv.Stop`, cancel HTTP tap goroutine and wait, ConsumeWipe
  on success, restore clean boot.ipxe.

#### tpi (new — key v0.2 deliverable)

Implementation: `internal/provisioner/tpi/tpi.go`. Replaces
`taskfiles/turing.yml`.

Config block on Node:

    boot:
      method: tpi
      tpi:
        host: "192.168.68.10"
        slot: 1                        # 1..4
        username_ref: "op://kubernetes/turingpi/username"
        password_ref: "op://kubernetes/turingpi/password"
        # OR identity_file_ref alone:
        # identity_file_ref: "op://kubernetes/turingpi/ssh_key"

`_ref` fields are `Ref` typed strings (custom YAML unmarshaller). Allowed
URI prefixes: `op://`, `sops://`, `file://`. `env://` is **prohibited**
for BMC creds (process-environment exposure). Anything else fails to
unmarshal with a typed error. No "credential-shaped value" heuristics.

- `Method() = "tpi"`
- `ContentionKey(node) = "tpi:" + node.Boot.TPI.Host` (every slot on a
  board shares one BMC).
- `Preflight`:
  - `tpi --version` succeeds AND parses to >= minimum (TBD;
    placeholder until we benchmark current operator binary).
  - TCP-connect to `host:443` within 2s.
  - All `_ref` fields resolve via `secrets.Resolver` (values consumed,
    never logged, fed to Scrubber).
  - Cache root has `>= max(image_size_compressed * 3, 8 GiB)` free
    (compressed + decompressed + headroom). Replaces fixed 4 GiB.
  - `(host, slot)` is unique across all `tpi`-method nodes in
    `cfg.Nodes` (validator runs at config load; Preflight re-checks
    defensively).
- `Prepare`:
  - Compute image URL from `cfg.Cluster.SchematicID` + `cfg.Cluster.TalosVersion`
    + `node.Arch`:
    `https://factory.talos.dev/image/<schematic>/<version>/metal-<arch>.raw.xz`
  - Look up expected sha256 in `cfg.Cluster.ImageDigests[<schematic>/<version>/<arch>]`.
    If not pinned, fail with a typed error pointing the operator to add
    the digest. **No TOFU on first download.**
  - Download to
    `~/.cache/nostos/images/<schematic>/<version>/metal-<arch>.raw.xz`
    via temp-file + atomic rename. Verify sha256 matches the pinned
    digest; on mismatch, delete and fail.
  - Decompress to sibling `.raw` using `github.com/ulikunitz/xz`
    (Go-native; no shell out).
  - Idempotent: if cached file exists and sha256 matches, skip download.
- `Boot`:
  - Materialize `identity_file_ref` (if set) to
    `~/.cache/nostos/secrets/<run-id>/tpi-key` with `O_CREAT|O_EXCL`
    mode 0600 inside a 0700 dir. `lstat` to refuse symlinks. Path is
    in argv; key bytes are not.
  - Set `TPI_USERNAME` / `TPI_PASSWORD` on the `Cmd.Env` (never argv).
  - `tpi --host <h> power off -n <slot>` — exit code != 0 is non-fatal
    if stderr indicates "already off" (provider documents the matched
    pattern).
  - `tpi --host <h> flash -i <local.raw> -n <slot>` — destructive.
  - `tpi --host <h> power on -n <slot>`.
  - Stream `tpi` stdout into emit() through Scrubber, with a 200ms
    coalescing window (assert ≤ 151 emits over a 30s synthetic stream).
- `WaitMaintenance`: poll `talosctl --insecure -n nv.IP version` every
  5s; success = parseable response. Default deadline 20 min (RK1 cold
  boots are slow). Override via `--wait-deadline`.
- `Apply`: `talosctl apply-config -i -n <ip> --file <configPath>`. The
  rendered config file is a 0600 temp owned by the orchestrator;
  provider does not move or copy it.
- `Cleanup`: on prior error, `tpi power off -n <slot>` (best-effort,
  60s deadline). Always: unlink any materialized key file in
  `~/.cache/nostos/secrets/<run-id>/`; remove that dir.

### D4. Concurrency and contention

- Single `contention.Map[string]*sync.Mutex` keyed by
  `Provisioner.ContentionKey(node)`. Held from before `Boot` to after
  `Apply` (or to Cleanup, whichever exits last). Empty key bypasses.
- `tpi` returns `"tpi:<host>"` → slots on one board serialize, distinct
  boards parallelize.
- `pxe` returns `"pxe:server"` → multiple PXE installs serialize on the
  single dnsmasq+9080 binding. v0.3 may add multiplexing.
- `--parallel` is **not** wired in v0.2. The locks ship; the flag does
  not. v0.2 is effectively serial.
- **Cross-process safety:** per-node flock at
  `nostos/state/configs/<name>.lock` (created 0600). Held across Render
  → Apply. A second `nostos node install <name>` from another laptop
  or shell on the same workstation fails fast with a typed
  `ErrLocked`. Cross-host concurrent operators are out of scope; spec
  scenario records the expected symptom.

### D5. Security model

Trust boundaries:

- Operator laptop: trusted (already runs `op signin`).
- BMC LAN: semi-trusted. Turing Pi BMCs ship with default passwords.
- Maintenance-mode Talos (apid pre-config): insecure window — same as
  v0.1. v0.2 narrows it by:
  - Probing with authenticated `talosctl --insecure version` (not raw
    TCP) so we verify "this is actually Talos in maintenance mode."
  - Holding the per-node flock so two concurrent local invocations
    cannot race the apply-config.
- factory.talos.dev: external trust dependency. Mitigated by
  operator-pinned `cluster.image_digests`.
- `op` CLI: trusted as v0.1 already trusted it. `OP_SESSION_*` is
  stripped from child env before exec'ing `tpi`/`talosctl`.

Rules pinned in spec:

- BMC creds resolved through `internal/secrets`; inline values fail
  YAML unmarshal (typed `Ref`).
- `env://` scheme prohibited for BMC creds.
- Subprocess: never invoke a shell. `Cmd.Env` for secrets;
  `Cmd.Stdin` where the tool accepts it; argv only for non-secret
  paths and flags.
- Resolved-secret values never reach emitted events: orchestrator
  wraps the EventEmitter with a Scrubber seeded once per run with all
  resolved values. Captured child stdout/stderr passes through the
  same Scrubber before emit.
- Rendered machineconfig lives at 0600 in a 0700 per-run secrets
  directory, unlinked after Apply.
- Tailscale authkey: spec requires single-use, TTL ≤ 1h, rotated per
  install run. Operator rotates the `op://` ref before invocation.
  Auto-rotation lands in v0.4 alongside `secrets rotate`.
- Live-node reinstall guard: orchestrator probes `talosctl version`
  on `node.IP`; if Ready, refuses unless `--reinstall` is passed.
- `(host, slot)` uniqueness validation across all `tpi`-method nodes.
- `nostos/state/configs/` must be in `.gitignore`; nostos refuses to
  start an install if any rendered file is git-tracked.

### D-Open. Open questions

- **Q1.** Concrete minimum `tpi` version. Placeholder in spec; pin
  during implementation.
- **Q2.** Cleanup retry policy on flaky BMCs. v0.2 = single try, 60s
  timeout. Revisit if real flashes show >60s power-off latency.
- **Q3.** Reinstall live-node detection: prefer `talosctl version`
  over ARP/ICMP since it confirms "Talos at this IP" not just
  "host alive." Confirm during implementation.
- **Q4.** `nostos up` alias removal: pinned to v0.3 release; if v0.3
  slips past 90 days, alias stays.

### D-Tests. Testing strategy (summary)

Three seams land in PR #1 before any provider migration:

- `execx.Commander` (mockable subprocess).
- `secrets.Resolver` (already exists; widen with FakeSecrets fixture).
- `clockx.Clock` (FakeClock; no wall-clock dependence in tests).

Plus a `internal/provisioner/provisionertest/` compliance suite that
both `pxe` and `tpi` pass with no skips (table-driven invariants:
Method stable, Preflight idempotent, Boot ctx-cancellation, Cleanup
fresh-ctx + idempotent, ContentionKey purity, no resolved secret in
emits).

Tier 1 (every PR, ubuntu-latest, < 3 min): unit, race, vet, lint.
Tier 2 (nightly, self-hosted with KVM): QEMU PXE end-to-end against a
SHA-pinned Talos amd64 image. Skipped with reason if `kvm-ok` fails;
never silent pass.
Tier 3 (manual, `run-hardware-tests` PR label): real Turing Pi flash;
real dell01 reinstall. Evidence captured via
`.github/PULL_REQUEST_TEMPLATE/hardware.md` (hardware, slot, image
sha, run-id, log attachment).

dell01 regression check is a **topological subsequence** assertion on
the event-Kind stream (PXE boot-time download events appear before
`apid-up`, `ready` is last, no `KindError`), not a literal-list
golden — same evidence without brittle ordering.

### D-Roadmap

v0.3 (next change): redfish + proxmox + usb providers (each its own
openspec change). JSONL run log + `--resume`. `inventory.db`. Drift
detection + `nostos diff`. `--parallel` real implementation. `nostos up`
alias removed.

v0.4: `cluster upgrade --to <ver>`. `secrets rotate` (covers Tailscale
authkey). Comprehensive `nostos doctor`. `rpi-imager` provider.

v1.0: Stable CLI, vendored iPXE binaries, Homebrew tap, container image,
hardware test matrix.
