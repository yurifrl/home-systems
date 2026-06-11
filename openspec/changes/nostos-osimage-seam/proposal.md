## Why

nostos cleanly abstracts **how** a machine boots (`internal/provisioner`: pxe,
tpi) but has never abstracted **what** it installs — "Talos" is an unnamed,
hardcoded assumption baked into render, the iPXE template, and the image writer.
Adding Proxmox (the `nostos-pxe-generic-iso` change) exposed that gap as six
scattered `if target == "proxmox"` branches across config, flash, serve, and
render, plus an OS field mis-located under `boot.pxe.target` (the OS choice is
not a PXE concern — `flash` needs it too). This change introduces the missing
axis as a peer registry so Talos and Proxmox become siblings and future OSes are
drop-in.

## What Changes

- Introduce `internal/osimage`: a registry (mirroring `internal/provisioner`'s
  `Register`/`New`/`Factory` shape) keyed by OS name, with an `OSImage`
  interface that owns **packaging**: resolve a version to a concrete image,
  render any node config, render the per-MAC netboot iPXE script, and produce a
  flash plan.
- Move the existing Talos packaging knowledge behind an `osimage/talos` impl
  (wrapping today's factory download, `registry.Render`, and iPXE template) and
  the Proxmox knowledge behind an `osimage/proxmox` impl (the resolver +
  sanboot script + ISO download already built).
- Delete the six `if target/proxmox/talos` branches; route `flash`, `pxe serve`,
  and `render` through `osimage.For(node)`.
- Refactor `internal/image.Builder` into an **OS-agnostic writer** that consumes
  a `FlashPlan` (`{main image, sidecars, notes}`); the Talos machineconfig
  sidecar and RPi EEPROM become Talos-provided parts, not writer built-ins.
- **Config migration (BREAKING for the unreleased `boot.pxe.target` field
  only):** move the OS choice up to a node-level `os: {name, version}` block;
  default to `talos` when absent (no YAML change for existing nodes); remove the
  short-lived `boot.pxe.target`/`boot.pxe.version` fields.
- `osimage` owns packaging only; `provisioner` keeps lifecycle. Proxmox is
  installed via `flash` or `pxe serve` (no orchestrated `node install` lifecycle
  in v1).

Out of scope: any new OS beyond Talos/Proxmox; a Proxmox `node install`
lifecycle; changes to the provisioner/transport axis.

## Capabilities

### New Capabilities
- `osimage-registry`: The pluggable OS-image abstraction — the `OSImage`
  interface, the self-registration registry, `osimage.For(node)` selection with
  a Talos default, and the `Ref`/`FlashPlan` value types that let transports
  consume any OS without branching.
- `image-writer`: The OS-agnostic flash writer (refactored from
  `internal/image.Builder`) that writes a `FlashPlan` to a file or block device
  — main image streamed (decompress on `.xz`), sidecars dropped beside it in
  file mode — with no knowledge of Talos, Proxmox, configs, or EEPROM.

### Modified Capabilities
- `pxe-netboot-targets`: Replace the `boot.pxe.target`/`boot.pxe.version` schema
  and per-MAC `if proxmox` dispatch with node-level `os: {name, version}` and
  dispatch routed through `osimage.For(node).NetbootScript(...)`. The observable
  behavior (Talos default; Proxmox memdisk/sanboot) is preserved; the
  configuration surface and internal dispatch change.

## Impact

- **Code (nostos submodule):**
  - New: `internal/osimage/` (registry + interface + `Ref`/`FlashPlan`),
    `internal/osimage/talos/`, `internal/osimage/proxmox/`.
  - Refactored: `internal/image/` → OS-agnostic writer consuming `FlashPlan`.
  - Simplified: `internal/cli/flash.go`, `internal/cli/commands.go`
    (`pxe serve`), `internal/registry/registry.go` (`render`) — branches
    removed, routed through `osimage`.
  - `internal/config/config.go` — new node-level `os:` block + validation;
    `boot.pxe.target`/`version` removed.
  - `internal/cli/schema/schema.go` + nostos README — document `os:` block.
  - Relocated: the proxmox resolver/download/sanboot logic from
    `internal/netboot/proxmox` into `internal/osimage/proxmox` (or consumed by
    it).
- **Config:** `nostos/config.yaml` pc01 entry migrates from `boot.pxe.target` to
  `os: {name: proxmox, version: latest}`.
- **Tests:** existing Talos config/pxe/image tests are the regression net the
  refactor must keep green at every phase.
- **No impact** on the provisioner/transport axis, the secrets pipeline, or any
  existing node's runtime behavior (Talos default preserves current behavior).
