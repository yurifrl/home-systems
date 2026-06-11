# nostos — the `osimage` seam: making "which OS" a first-class, pluggable axis

Status: draft (architecture / design thinking)
Date: 2026-06-08
Owner: Yuri
Supersedes the scattered `boot.pxe.target` branching introduced by the
`nostos-pxe-generic-iso` change.

> One line: nostos already abstracts **how** a machine boots (`provisioner`:
> pxe, tpi). It has never abstracted **what** it installs — "Talos" was an
> unnamed, hardcoded assumption. Adding Proxmox exposed that gap as a rash of
> `if target == "proxmox"` branches. This design introduces the missing axis as
> a peer registry (`osimage`) so Talos and Proxmox are siblings and future OSes
> are drop-in.

---

## 1. The problem, precisely

Proxmox support (the `nostos-pxe-generic-iso` change) shipped working code but
smeared the OS choice across six conditional sites:

```
config/config.go     EffectiveTarget / PXETarget / validate branch
cli/flash.go         if PXETarget == "proxmox"  (×2)
cli/commands.go      if PXETarget != "proxmox"  (proxmox script build)
registry/registry.go if PXETarget != "talos"    (render refusal)
```

Each `if` is the same missing concept leaking: **no type owns "the OS."** Worse,
the OS lived under `boot.pxe.target`, implying it's a PXE concern — but `flash`
needs it too, so the field is mis-located as well as under-abstracted.

### The two axes

nostos installs **an OS** onto **a machine** using **a boot method**. That's two
orthogonal axes:

```
              payload: talos              payload: proxmox
         ┌────────────────────────┬────────────────────────┐
  pxe    │ kernel+initrd+config   │ sanboot the ISO         │
         │ (factory.talos.dev)    │ (resolve latest/pinned) │
         ├────────────────────────┼────────────────────────┤
  flash  │ raw image + sidecar    │ ISO written raw         │
         │ cfg + (rpi) EEPROM     │ (isohybrid)             │
         ├────────────────────────┼────────────────────────┤
  tpi    │ talos via BMC          │ (n/a today)             │
         └────────────────────────┴────────────────────────┘
   ▲                  ▲
   │                  └ PAYLOAD axis — MISSING abstraction (this design)
   └ TRANSPORT axis — already abstracted as internal/provisioner
```

Every cell is a *composition* (transport × payload), not a special case. The
`if`s exist only because the column ("payload") has no owning type.

## 2. Decisions (locked)

1. **Name the axis `osimage`** — package `internal/osimage`, interface
   `OSImage`. Reads in plain English next to the existing axis: *provisioner =
   how it boots; osimage = what it installs.* (`target` too abstract; `image`
   collides with the writer package; `payload`/`distro` are jargon.)

2. **`internal/image.Builder` becomes a pure writer.** It currently hardcodes
   Talos specifics (sidecar machineconfig, RPi EEPROM). Invert that: an
   `OSImage` produces a **FlashPlan**; the writer just streams the main image to
   the device/file and drops any sidecars beside it. The writer stops knowing
   any OS.

3. **`osimage` owns *packaging*; `provisioner` keeps *lifecycle*.** Smallest
   honest blast radius. Lifecycle (apply-config, bootstrap) is genuinely
   Talos-specific and already lives in `provisioner`; it stays there. Proxmox
   has no orchestrated lifecycle in v1 — it is served or flashed and boots
   itself.

## 3. The seam

Mirror the `provisioner` registry exactly — same `Register`/`New`/`Factory`
shape the codebase already uses, so there's nothing new to learn:

```go
// internal/osimage/osimage.go
package osimage

// Ref is a concrete, resolved image identity (post "latest"/schematic
// resolution). Opaque to callers; each OSImage defines its own.
type Ref struct {
    OS       string // "talos" | "proxmox"
    Version  string // concrete, e.g. "v1.13.3" or "8.10-1"
    SHA256   string // when known (auditability for "latest")
    // ... os-specific fields as needed (schematic, arch, iso url)
}

// FilePart is one artifact written during a flash (main image or a sidecar).
type FilePart struct {
    Name        string // suggested filename / suffix
    Path        string // local source path
    IsMainImage bool   // exactly one part is the bootable image
}

// FlashPlan is what an OSImage hands the (OS-agnostic) writer.
type FlashPlan struct {
    Parts []FilePart // [0]=main image; rest are sidecars (config, eeprom…)
    Notes []string   // operator next-steps (e.g. "apply-config …")
}

// OSImage is the contract every installable OS implements.
type OSImage interface {
    Name() string                                       // "talos" | "proxmox"

    // Resolve turns node config (version selector etc.) into a concrete Ref,
    // downloading/caching the artifact as a side effect when needed.
    Resolve(ctx context.Context, node *config.Node) (Ref, error)

    // NodeConfig renders any config this OS needs applied after boot.
    // Talos: the machineconfig. Proxmox: returns (nil, nil) — no config.
    NodeConfig(ctx context.Context, node *config.Node) ([]byte, error)

    // NetbootScript renders the per-MAC stage-2 iPXE script for this OS.
    // Talos: kernel+initrd+talos.config. Proxmox: sanboot the ISO.
    NetbootScript(ctx context.Context, node *config.Node, ref Ref) (string, error)

    // FlashPlan returns the parts to write to a USB/device for this OS.
    FlashPlan(ctx context.Context, node *config.Node, ref Ref) (FlashPlan, error)
}

// Register / New / For — identical pattern to internal/provisioner.
func Register(name string, f Factory) { /* … */ }
func For(node *config.Node) (OSImage, error) { /* defaults to "talos" */ }
```

`internal/osimage/talos` and `internal/osimage/proxmox` each `Register()`
themselves in `init()`, exactly like `provisioner/pxe` and `provisioner/tpi`.

## 4. Call sites collapse (the `if`s evaporate)

```
flash:      img := osimage.For(node)
            ref, _ := img.Resolve(ctx, node)
            plan, _ := img.FlashPlan(ctx, node, ref)
            imagewriter.Write(plan, dest)              // no OS knowledge

pxe serve:  img := osimage.For(node)
            ref, _ := img.Resolve(ctx, node)
            script, _ := img.NetbootScript(ctx, node, ref)   // no `if proxmox`

render:     cfg, _ := osimage.For(node).NodeConfig(ctx, node)
            if cfg == nil { return "this OS installs no machineconfig" }
```

A future `debian` OS is a new file that `Register`s itself — **zero edits** to
flash / serve / render / writer. That is the extensibility test, and it passes.

## 5. Config migration: OS moves up to the node

The OS choice is orthogonal to transport, so it must not live under `boot.pxe`:

```yaml
# BEFORE (the mistake): OS buried under one transport
pc01:
  boot:
    pxe:
      target: proxmox
      version: latest

# AFTER (obvious): OS is a node-level axis; every transport reads it
pc01:
  os:
    name: proxmox          # talos (default when block absent) | proxmox
    version: latest        # proxmox: latest | "8.3-1"; talos: cluster-implied
  boot:
    method: pxe            # pxe | flash | tpi — all consult node.os
```

Backward compatibility: a node with no `os:` block defaults to `name: talos`,
preserving today's behavior for dell01/tp1/tp4/rpi01 with no YAML edits. The
short-lived `boot.pxe.target` field (only pc01 used it, never shipped to a real
boot) is removed, not deprecated.

## 6. What `internal/image` becomes

`Builder` (Talos-flavored) → `imagewriter.Write(plan FlashPlan, dest)`:

- Opens the destination (file or block device; existing ModeFile/ModeDevice).
- Streams the **main image** part (decompress on `.xz`, as today).
- In file mode, writes each **sidecar** part beside the output (the Talos
  machineconfig sidecar + RPi EEPROM become *Talos-provided parts*, not writer
  built-ins).
- Knows nothing about Talos, Proxmox, configs, or EEPROM semantics.

This is a net simplification of `internal/image`, not just a move.

## 7. Phased plan (talos tests are the safety net)

The existing Talos path works and is tested; treat those tests as the harness
the refactor must keep green at every step.

- **P1 — introduce the seam, no behavior change.** Add `internal/osimage` with
  the registry + a `talos` impl that wraps *today's* code paths (call into the
  existing factory download, `registry.Render`, the existing ipxe template).
  Route `render` + `pxe serve` (talos) through `osimage.For`. All existing
  tests stay green. No proxmox yet.
- **P2 — move Proxmox behind the seam.** Reimplement the proxmox resolver +
  sanboot script + ISO download as `osimage/proxmox`. Delete the six `if`
  branches. `pxe serve` proxmox path now flows through `NetbootScript`.
- **P3 — pure writer.** Refactor `internal/image.Builder` → `imagewriter`
  taking a `FlashPlan`; move Talos sidecar + EEPROM into `osimage/talos`'s
  `FlashPlan`. Wire `flash` through `osimage.For(node).FlashPlan`. This is where
  the dangling `flash.go` proxmox branch gets *replaced* (not finished) by the
  seam.
- **P4 — config migration.** Add node-level `os:` block + validation; default to
  talos; remove `boot.pxe.target`. Update pc01 in `config.yaml`. Update
  `nostos schema` + README.

Each phase is independently shippable and leaves the tree green.

## 8. Boundary with `provisioner` (avoid the obvious trap)

`osimage` must **not** absorb lifecycle. The tempting-but-wrong move is to give
`OSImage` `Preflight/Boot/Apply` methods — that re-implements `provisioner` and
re-tangles the axes. Keep it disciplined:

```
provisioner.Provisioner  → transport + lifecycle (preflight, boot, wait, apply)
osimage.OSImage          → packaging (resolve, config bytes, ipxe script, flash plan)
orchestrator (node install) wires the two; for talos it always has.
```

For proxmox, `node install` is simply unsupported in v1 (no lifecycle to drive);
the supported paths are `flash` and `pxe serve`. We do **not** invent a fake
lifecycle to force proxmox through `node install`.

## 9. Risks / trade-offs

- **[Moving working Talos code]** → P1 wraps existing code without changing it;
  the existing Talos tests gate every phase. No big-bang rewrite.
- **[`Ref` becoming a god-struct]** → keep it minimal; OS-specific fields stay
  internal to each impl where possible (the registry hands back an `OSImage`
  that already closed over its resolution).
- **[Two registries (provisioner + osimage) look similar]** → that's a feature:
  identical shape = one mental model. Document the boundary (§8) prominently.
- **[Scope creep into lifecycle]** → explicitly out (§8). If/when Proxmox needs
  orchestration, it becomes a `provisioner` method, not an `osimage` concern.

## 10. Open questions

- `Resolve` doing I/O (download/cache) vs. a separate `Fetch` step — folding it
  in keeps call sites short; splitting it makes dry-run/caching boundaries
  crisper. Lean: `Resolve` is pure-ish (returns Ref + URLs), a separate
  `Ensure(ref)` does the download/cache. (Reconcile with the existing
  `proxmox.DownloadISO`.)
- Does `tpi` ever need a non-talos osimage? Probably never; fine — the matrix is
  allowed to have empty cells.
- Should `pxe serve` and the `provisioner/pxe` install path share the
  netboot-script rendering via `osimage` (they currently duplicate ipxe
  knowledge)? Likely yes in P2; worth confirming.
