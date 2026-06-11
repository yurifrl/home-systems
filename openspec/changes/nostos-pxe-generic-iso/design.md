## Context

`nostos pxe serve` runs dnsmasq (DHCP-proxy + TFTP) and an HTTP server, then
hands every PXE-booting machine a single hardcoded Talos iPXE script that loads
the Talos kernel/initramfs from `factory.talos.dev` and a machineconfig from
`/configs/<mac>.yaml`. The boot identity model is already MAC-based: dnsmasq
sees the MAC, nostos maps it to a node in `config.yaml`.

pc01 (Gigabyte B450 Aorus Elite V2, AMI UEFI, Realtek GbE, dual M.2 NVMe) is to
be installed with **Proxmox VE on bare metal**, then host a Talos worker VM and
a Windows VM with GPU passthrough. PXE is the chosen install path. The only gap
is that nostos can emit *only* a Talos boot script. Everything else in the serve
path — interface detection, dnsmasq spawn, sudo handling, the NDJSON event tap,
the asset cache — is OS-agnostic and reusable as-is.

Constraints:
- Must not change the Talos boot path or any existing node's behavior.
- Must stay YAML-native: the operator declares intent (`target: proxmox`,
  `version: latest`), never URLs.
- Tests must not hit the live Proxmox network.

## Goals / Non-Goals

**Goals:**
- Per-MAC dispatch in `nostos pxe serve`: Talos nodes unchanged; `target:
  proxmox` nodes get a generic-ISO boot script that launches the Proxmox
  installer.
- A Proxmox-aware resolver that turns `version` (`latest` | pinned) into
  concrete boot artifacts, owning the knowledge of Proxmox's download layout.
- Reproducibility: record the concrete resolved version + checksum so `latest`
  is auditable after the fact.
- Backward compatibility: absence of `boot.pxe` (or `target: talos`) == today's
  behavior, no YAML migration.

**Non-Goals:**
- Creating/importing the VMs inside Proxmox (talos-pc01, windows) — downstream.
- Unattended Proxmox install via answer file — a follow-up; v1 is interactive.
- A general "any distro" netboot framework — Proxmox is the only first-class
  `target` shipped, even though the seam is generic.
- Auto-detecting "Proxmox is installed and up" — manual ack is acceptable for v1.

## Decisions

### D1: Per-MAC dispatch keyed on `boot.pxe.target`
The serve loop already resolves MAC → node. We add one branch: select which
iPXE script to emit based on `node.Boot.PXE.Target` (default `talos`).
- **Why:** smallest possible change to a proven path; mixed fleets work under
  one `nostos pxe serve`.
- **Alternative considered:** a separate `nostos pxe serve-proxmox` command.
  Rejected — duplicates the daemon, splits the fleet, and breaks the
  "one command" model.

### D2: Config schema `boot.pxe.{target,version}`
`target: talos|proxmox` (default `talos`); `version: latest|<pinned>` required
when `target != talos`. Pinned versions validated against `^\d+\.\d+-\d+$`
before any network call.
- **Why:** declarative, minimal, and backward-compatible by defaulting.
- **Alternative considered:** a free-form `iso_url`. Rejected — pushes URL
  knowledge into YAML, defeats the `latest` convenience and reproducibility.

### D3: Built-in Proxmox resolver (`internal/netboot/proxmox`)
`Resolve(version) → BootSpec{KernelURL, InitrdURL, ISOURL, Cmdline, Version}`.
- `latest`: GET the Proxmox ISO index, parse `proxmox-ve_X.Y-Z.iso`, sort by
  `(X,Y,Z)`, pick newest.
- pinned: construct the URL directly and confirm existence.
- Record the concrete version + sha256 (Proxmox publishes checksums).
- **Why:** keeps URL/layout knowledge in code, not config; makes `latest`
  reproducible.
- **Alternative considered:** shelling out or a third-party library. Rejected —
  a tiny HTML/index parse is enough and keeps the dependency surface flat.

### D4: memdisk (full ISO to RAM) as the first boot mechanism
iPXE loads the entire Proxmox ISO into RAM and boots it.
- **Why:** pc01 has ample RAM; avoids per-version kernel/initrd extraction and
  installer-fetch-arg brittleness across PVE releases. Works with the stock ISO.
- **Alternative considered:** kernel+initrd-direct (extract `linux26` +
  `initrd.img`, boot with installer fetch args). Faster/lower-RAM but brittle
  across versions — deferred as an optimization behind the same `target`.

### D5: PXE installs the hypervisor only; explicit handoff boundary
After the installer writes to `install_disk` and reboots, pc01 boots Proxmox
from disk. PXE's responsibility ends at "installer launched." Mirror the
existing reliability pattern: once a node is known-installed, serve it a
boot-local/`exit` iPXE script so a stray re-PXE doesn't reinstall.
- **Why:** Proxmox has no Talos-style lifecycle for nostos to track; pretending
  otherwise adds complexity with no payoff.

## Risks / Trade-offs

- **[Realtek + UEFI PXE can be fiddly]** → Validate empirically on first serve:
  if pc01 appears in the NDJSON event log (`discover → tftp`), Network Stack is
  on. One clean attempt confirms; no code mitigation needed.
- **[Proxmox ISO index format changes / scrape breaks `latest`]** → Pinned
  `version` is always available as an escape hatch; resolver records the
  concrete version so a working pin can be copied from a prior run. Keep the
  parser tolerant and unit-tested against a captured fixture.
- **[memdisk RAM/boot-time cost; some ISOs finicky]** → pc01 RAM is ample;
  kernel+initrd-direct remains a fallback behind the same `target` if memdisk
  misbehaves.
- **[Stray re-PXE reinstalls Proxmox]** → installed→boot-local script (D5);
  manual ack acceptable for v1.
- **[Outbound HTTPS to download.proxmox.com required at serve time]** → cache
  artifacts under the existing assets dir; `latest` resolves once per serve, not
  per request.

## Migration Plan

Additive, no migration of existing config required.
1. Ship schema + resolver + dispatch (no behavior change for Talos nodes).
2. Add the `pc01` node to `nostos/config.yaml`.
3. `nostos pxe serve`, power on pc01, watch the event log, install Proxmox to
   `/dev/nvme0n1`.
- **Rollback:** remove the `pc01` node and/or the `boot.pxe` block; Talos path
  is untouched throughout.

## Open Questions

- `latest` resolution source: directory-index scrape vs a more stable checksum
  index. Start with the index; record resolved version either way.
- Unattended install via Proxmox `answer.toml` — follow-up scope.
- Installed-state detection: manual ack vs probing pc01:8006 (PVE web UI) to
  auto-flip to boot-local.
- Whether to generalize `target` to other distros later (debian/ubuntu) — keep
  the enum open, ship only the Proxmox resolver now.
