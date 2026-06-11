## Context

nostos abstracts **how** a machine boots via `internal/provisioner` (a registry
of `pxe`, `tpi` that self-register with `Register`/`New`/`Factory`). It has never
abstracted **what** it installs: "Talos" is hardcoded across `registry.Render`
(machineconfig), `internal/pxe/embed.go` (kernel/initrd iPXE template), the
factory.talos.dev download, and `internal/image.Builder` (which bakes in a Talos
machineconfig sidecar and RPi EEPROM).

The `nostos-pxe-generic-iso` change added Proxmox but, lacking a payload
abstraction, smeared the OS choice across six `if target/proxmox/talos` sites
(config, flash ×2, pxe-serve, render) and mis-located the field under
`boot.pxe.target`. `flash.go` is currently mid-refactor and references two
unwritten functions (it does not compile) — this change replaces that dangling
work with the proper seam.

The two axes are orthogonal: **transport** (pxe/tpi/flash) × **payload**
(talos/proxmox). Transport is abstracted; payload is not. Every `if` is the
missing payload type leaking.

## Goals / Non-Goals

**Goals:**
- Introduce `internal/osimage` as a peer registry to `internal/provisioner`,
  with an `OSImage` interface owning **packaging** (resolve, node config,
  netboot script, flash plan).
- Move Talos packaging behind `osimage/talos` (wrapping existing code) and
  Proxmox behind `osimage/proxmox` (reusing the resolver/sanboot/download).
- Delete all six OS conditionals; route `flash`, `pxe serve`, `render` through
  `osimage.For(node)`.
- Refactor `internal/image.Builder` into an OS-agnostic writer consuming a
  `FlashPlan`.
- Move the OS choice to a node-level `os: {name, version}` block; default talos.
- Keep every existing Talos test green at every phase.

**Non-Goals:**
- A Proxmox `node install` lifecycle (Proxmox is flashed or served; no
  orchestrated lifecycle in v1).
- Any OS beyond Talos/Proxmox.
- Changes to the provisioner/transport axis, secrets pipeline, or Talos runtime
  behavior.
- Absorbing lifecycle (preflight/boot/apply) into `osimage`.

## Decisions

### D1: Name the payload axis `osimage` (interface `OSImage`)
Reads in plain English beside the existing axis: *provisioner = how it boots;
osimage = what it installs.*
- **Alternatives:** `target` (too abstract — target of what?), `image`
  (collides with the `internal/image` writer), `payload`/`distro` (jargon).
  Rejected for obviousness.

### D2: `internal/image.Builder` becomes an OS-agnostic writer
`OSImage.FlashPlan(node, ref)` returns `{Parts []FilePart, Notes []string}` with
exactly one `IsMainImage` part. The writer streams the main image (decompress on
`.xz`) to file/device and, in file mode, drops sidecars beside it. Talos's
machineconfig sidecar and RPi EEPROM become Talos-provided parts.
- **Why:** removes the writer's hidden Talos flavor; a net simplification, not
  just a move.
- **Alternative:** keep Builder and add a parallel proxmox writer — rejected,
  duplicates device/file/compression logic and keeps the smell.

### D3: `osimage` owns packaging; `provisioner` keeps lifecycle
`OSImage` exposes only `Name/Resolve/NodeConfig/NetbootScript/FlashPlan`. No
`Preflight/Boot/Apply`.
- **Why:** the tempting move (give OSImage lifecycle hooks) re-implements
  `provisioner` and re-tangles the axes. Lifecycle (apply-config, bootstrap) is
  Talos-specific and already lives in `provisioner`; it stays. Proxmox has no
  lifecycle in v1.
- **Alternative:** a single mega-interface spanning both axes — rejected as the
  exact thing that created the `if`s at a larger scale.

### D4: OS choice moves to node-level `os: {name, version}`
The OS is orthogonal to transport (flash needs it too), so it must not live
under `boot.pxe`. Absent block defaults to `talos`.
- **Why:** correct location; backward compatible for existing nodes.
- **Alternative:** keep it under `boot.pxe` — rejected; it's the config-level
  version of the same mistake.

### D5: Resolve vs. fetch split
`Resolve(node) → Ref` returns the concrete identity + URLs (cheap, dry-run
friendly); a separate `Ensure(ref) → localPath` performs download/caching. This
reconciles with the existing `proxmox.DownloadISO` and keeps dry-run paths from
doing I/O.
- **Alternative:** fold download into `Resolve` — rejected; muddies dry-run and
  caching boundaries.

## Risks / Trade-offs

- [Moving working Talos code] → P1 wraps existing code paths without changing
  them; existing Talos tests gate every phase. No big-bang rewrite.
- [`Ref` becoming a god-struct] → keep it minimal; OS-specific fields stay
  internal to each impl where possible (the registry returns an `OSImage` that
  has closed over its resolution).
- [Two similar registries (provisioner + osimage)] → intentional: identical
  shape = one mental model. The packaging/lifecycle boundary (D3) is documented
  prominently to prevent drift.
- [Scope creep into lifecycle] → explicitly out (D3); a future orchestrated
  Proxmox install becomes a `provisioner` method, not an `osimage` concern.
- [Removing the unreleased `boot.pxe.target`] → only pc01 used it and it never
  booted a real machine; removed (not deprecated) to avoid carrying a dead
  field.

## Migration Plan

Phased; each phase independently shippable and leaves the tree green.

1. **P1 — seam, no behavior change.** Add `internal/osimage` (registry +
   interface + `Ref`/`FlashPlan`) and an `osimage/talos` impl wrapping today's
   factory download, `registry.Render`, and iPXE template. Route `render` +
   talos `pxe serve` through `osimage.For`. Existing tests stay green.
2. **P2 — Proxmox behind the seam.** Reimplement the resolver + sanboot script +
   ISO download as `osimage/proxmox`. Delete the six `if` branches; serve's
   proxmox path flows through `NetbootScript`.
3. **P3 — pure writer.** Refactor `internal/image.Builder` → writer taking a
   `FlashPlan`; move Talos sidecar + EEPROM into `osimage/talos`'s `FlashPlan`.
   Wire `flash` through `osimage.For(node).FlashPlan`. Replaces the dangling
   `flash.go` proxmox branch.
4. **P4 — config migration.** Add node-level `os:` + validation; default talos;
   remove `boot.pxe.target`/`version`. Migrate pc01 in `config.yaml`; update
   `nostos schema` + README.

**Rollback:** the change is additive until P4; reverting the `os:` migration
restores prior config parsing. Talos default preserves runtime behavior
throughout.

## Open Questions

- Should `pxe serve` and the `provisioner/pxe` install path share netboot-script
  rendering via `osimage` (they currently duplicate iPXE knowledge)? Likely yes
  in P2; confirm during implementation.
- Final home of `internal/netboot/proxmox`: relocate wholesale into
  `internal/osimage/proxmox`, or keep as a lower-level helper the OSImage
  consumes? Lean: consume it, then collapse if it adds no value.
- Does `Ref` need to be exported/serializable for `--dry-run` plan output, or
  stay internal? Decide when wiring dry-run.
