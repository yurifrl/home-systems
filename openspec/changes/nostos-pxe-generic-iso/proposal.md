## Why

`nostos pxe serve` can only netboot Talos: its iPXE script, served configs, and
lifecycle are all Talos-specific. But pc01 needs to run **Proxmox VE on bare
metal** (to then host a Talos worker VM and a Windows VM with GPU passthrough),
and PXE is the cleanest way to install it — pc01's Realtek NIC + AMI UEFI
support network boot. We want the same one command to install Proxmox on pc01
and Talos on everything else, chosen entirely from `config.yaml`.

## What Changes

- Add a `boot.pxe` block to a node in `config.yaml` with two fields:
  - `target`: `talos` (default) | `proxmox`
  - `version`: `latest` | a pinned release (e.g. `8.3-1`), required when
    `target` is not `talos`.
- `nostos pxe serve` dispatches the iPXE script **per-MAC**: Talos nodes get
  today's Talos boot script (unchanged); `target: proxmox` nodes get a
  generic-ISO boot script that launches the Proxmox installer.
- Add a **Proxmox-aware resolver** inside nostos that turns `version` into
  concrete boot artifacts: `latest` discovers the newest
  `proxmox-ve_X.Y-Z.iso` from the Proxmox download index; a pinned version
  builds the URL directly. The resolved concrete version + checksum are
  recorded/emitted so `latest` is reproducible after the fact.
- First boot mechanism: **memdisk** (load the full ISO into RAM) — pc01 has
  ample RAM and this avoids per-version extraction fragility. A
  kernel+initrd-direct path is a later optimization behind the same `target`.
- Onboard **pc01** to `config.yaml` (`target: proxmox`, `version: latest`,
  `install_disk: /dev/nvme0n1`).
- **Non-breaking**: a node with no `boot.pxe` block (or `target: talos`) keeps
  exactly today's Talos behavior. No existing node YAML changes.

Out of scope: creating the VMs inside Proxmox (talos-pc01, windows), unattended
Proxmox answer-file install, and any non-Proxmox distro resolver.

## Capabilities

### New Capabilities
- `pxe-netboot-targets`: Per-node selection of what `nostos pxe serve` boots
  (Talos vs a generic ISO installer), the `boot.pxe.{target,version}` config
  schema and its validation, per-MAC iPXE dispatch, and the generic-ISO
  (memdisk) boot script.
- `proxmox-release-resolver`: Resolving a Proxmox `version` (`latest` or pinned)
  to concrete boot artifacts (kernel/initrd/ISO URL + checksum) using nostos's
  built-in knowledge of the Proxmox release/download layout.

### Modified Capabilities
<!-- None. PXE behavior is additive; no existing spec exists for the Talos path,
     and that path is unchanged. -->

## Impact

- **Code (nostos submodule, `github.com/yurifrl/nostos`):**
  - `internal/config/config.go` — new `PXEBoot{Target, Version}` under `Boot`;
    validation; surfaced in `nostos schema`.
  - `internal/netboot/proxmox/` (new) — `Resolve(version) → BootSpec`; unit
    tests against a captured `iso/` index fixture (no live network).
  - `internal/pxe/serve.go` — per-MAC target dispatch.
  - `internal/pxe/` — new generic-ISO (memdisk) iPXE template + any embedded
    assets.
  - `internal/cli/` node wizard + `schema/schema.go`.
- **Config:** `nostos/config.yaml` gains the `pc01` node.
- **Dependencies:** outbound HTTPS to `download.proxmox.com` at serve time for
  `latest` resolution and ISO fetch; cached under the existing assets dir.
- **No impact** on the Talos boot path, the secrets pipeline, or any existing
  node's lifecycle.
