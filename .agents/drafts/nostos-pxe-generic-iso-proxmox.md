# nostos PXE — Generic ISO netboot (Proxmox-aware), YAML-native

Status: draft
Date: 2026-06-08
Owner: Yuri
Related: `.agents/drafts/nostos-pxe-reliable-ai-friendly.md`, nostos submodule
First target node: **pc01** (bare-metal x86, Proxmox VE)

> Goal in one line: `nostos pxe serve` should be able to netboot a node into
> the **Proxmox VE installer** (not just Talos), driven entirely from
> `config.yaml`, with a `version: latest` convenience that nostos resolves
> itself because it knows Proxmox's release layout.

---

## 1. Problem

Today nostos PXE is **hardwired to Talos**:

- `boot.ipxe` loads the Talos kernel + initramfs from `factory.talos.dev`.
- The HTTP server serves Talos machineconfigs at `/configs/<mac>.yaml`.
- The lifecycle assumes Talos maintenance mode → `apply-config` → reboot.

But pc01 is meant to run **Proxmox VE on bare metal**, then host two VMs:

- `talos-pc01` (VM 100) — a Talos Kubernetes worker (`proxmox/100.conf`).
- `windows` (VM 101) — Windows 11 with GPU passthrough (`proxmox/101.conf`).

To stand pc01 up we need to PXE-boot it into the **Proxmox installer**. PXE can
boot any OS installer; the limitation is purely that nostos's PXE path only
knows how to emit a Talos boot script. We want the same one-command
`nostos pxe serve` to dispatch per-node: Talos for the Talos nodes, Proxmox for
pc01 — chosen by the node's `config.yaml` entry, keyed off its MAC.

## 2. Goals / non-goals

**Goals**
- Add a **generic, YAML-native netboot target** to PXE so a node can boot a
  non-Talos installer (starting with Proxmox VE).
- Keep the boot identity model unchanged: **MAC → node → what to boot**. One
  `nostos pxe serve` serves a mixed fleet (Talos + Proxmox) simultaneously.
- A **Proxmox-aware resolver** built into nostos: `version: latest` (or a pinned
  `8.3-1`) resolves to the right kernel/initrd because nostos knows Proxmox's
  download layout. The operator writes intent, not URLs.
- Backward compatible: existing Talos nodes keep working with **no config
  change**. Absence of the new block == Talos (today's behavior).

**Non-goals**
- Not a general "boot any Linux distro" framework yet. Proxmox is the first and
  only first-class `target`; the seam is generic but we only ship the Proxmox
  resolver in this iteration.
- Not managing the Proxmox install **answer file** / unattended automation in v1
  (see Open Decisions — interactive installer is acceptable for the first cut).
- Not managing the **VMs** inside Proxmox. Once Proxmox is on disk, VM creation
  (talos-pc01, windows) is a separate concern (`proxmox/*.conf`, future
  Crossplane).
- Not changing the Talos boot path, the secrets pipeline, or the lifecycle of
  any existing node.

## 3. Architecture

```
config.yaml                    nostos pxe serve (one daemon, whole fleet)
  nodes:                          │
    dell01: {…Talos…}             ├─ DHCP/TFTP (dnsmasq) + HTTP (:9080)
    pc01:                         │
      boot:                       ▼
        method: pxe          ┌──────────────────────────────────────────┐
        pxe:                 │ per-MAC boot dispatch                     │
          target: proxmox    │                                          │
          version: latest    │  MAC d0:94:… → dell01 → target: talos    │
                             │     → emit Talos iPXE (current path)      │
                             │                                          │
                             │  MAC fc:3c:… → pc01  → target: proxmox   │
                             │     → ProxmoxResolver.Resolve(version)   │
                             │        → {kernel, initrd, cmdline}        │
                             │     → emit generic-ISO iPXE script        │
                             └──────────────────────────────────────────┘
                                          │
                                          ▼
                        pc01 netboots Proxmox installer → installs to
                        install_disk → reboots → boots Proxmox from disk
```

### 3.1 The dispatch point

The single new branch is "what iPXE script does this MAC get?":

| Node `boot.pxe.target` | iPXE script emitted | Source |
|---|---|---|
| absent / `talos` | Talos kernel+initramfs from factory | current code path |
| `proxmox` | Proxmox kernel+initrd, resolved by version | **new** |

Everything upstream (interface detection, dnsmasq, sudo, multi-homing, the
NDJSON event tap from the reliability draft) is **shared and unchanged**.

## 4. Config schema (YAML-native)

New optional `boot.pxe` block on a node. Minimal happy path:

```yaml
nodes:
  pc01:
    mac: "fc:3c:d7:27:66:17"
    ip: 192.168.68.101
    install_disk: /dev/nvme0n1      # which NVMe Proxmox installs onto
    boot:
      method: pxe
      pxe:
        target: proxmox             # what to boot (default: talos)
        version: latest             # or a pinned "8.3-1"
```

Field semantics:

| Field | Required | Meaning |
|---|---|---|
| `boot.method` | yes (for new nodes) | `pxe` \| `tpi` (existing enum) |
| `boot.pxe.target` | no | `talos` (default) \| `proxmox` |
| `boot.pxe.version` | only when target≠talos | `latest` \| pinned (`"8.3-1"`) |
| `install_disk` | recommended | passed to the installer where supported |

Backward-compat rule: a node with **no `boot.pxe` block** (or `target: talos`)
gets exactly today's Talos behavior. No existing node YAML changes.

Validation:
- `version` is required when `target: proxmox`; reject otherwise (exit 10).
- pinned versions validated against the Proxmox pattern (`^\d+\.\d+-\d+$`)
  before any network call.

## 5. Proxmox resolver (nostos knows the internals)

A small package — `internal/netboot/proxmox` (name TBD) — that turns a
`version` into concrete boot artifacts. nostos owns the knowledge of Proxmox's
release layout so the YAML never carries URLs.

```
ProxmoxResolver.Resolve(version) → BootSpec{
    KernelURL  string   // e.g. extracted from the ISO / mirror
    InitrdURL  string
    Cmdline    string   // installer args (+ any nostos-supplied bits)
    ISOURL     string   // for the memdisk fallback (see §6)
    Version    string   // the concrete resolved version, for logging/events
}
```

Resolution rules:

- `version: latest`
  → `GET https://download.proxmox.com/iso/`
  → parse the directory listing for `proxmox-ve_<X.Y-Z>.iso`
  → sort by semver-ish `(X, Y, Z)`, pick the newest.
- `version: "8.3-1"` (pinned)
  → construct directly: `…/iso/proxmox-ve_8.3-1.iso` (validate it exists).
- The resolved concrete version + final URL(s) are **logged and emitted as an
  event** so `latest` is never a mystery after the fact.

Reliability hooks (reuse patterns already in the repo):
- Pin/record a **sha256** of the resolved ISO (Proxmox publishes checksums) so a
  re-serve of the same `version` is verifiable and cache-stable.
- Cache downloaded artifacts under the existing assets dir (same as Talos
  images) — `latest` resolves once per serve, not per PXE request.

## 6. How Proxmox actually boots over PXE (the hard bit)

Proxmox's installer is an ISO, not a clean kernel+initrd+cmdline like Talos.
Two viable mechanisms, decide in §9:

| Mechanism | How | Pros | Cons |
|---|---|---|---|
| **A. memdisk / full-ISO to RAM** | iPXE `sanboot`/`memdisk` loads the whole ISO into RAM | Works with the stock ISO, no extraction | Needs RAM ≥ ISO (~1.3GB+ ok on pc01), slower, some ISOs finicky |
| **B. kernel+initrd direct** | extract `linux26`/`vmlinuz` + `initrd.img` from the ISO, iPXE boots them with a cmdline that fetches the rest over HTTP | Fast, low RAM | Must host the ISO contents over HTTP and pass the right `proxmox-install` fetch args; brittle across PVE versions |

pc01 has plenty of RAM, so **memdisk (A)** is the low-risk first cut; B is an
optimization we can add behind the same `target: proxmox` once A is proven.

Either way the emitted artifact is "a generic iPXE script for this MAC" — the
resolver + a small template produce it; the serve loop just hands it out.

## 7. Lifecycle & handoff

Talos nodes have a rich lifecycle (maintenance → apply-config → bootstrap →
ready) that nostos tracks. Proxmox does **not** plug into that, and we should
not pretend it does:

- After the installer runs and writes to `install_disk`, pc01 reboots and boots
  Proxmox **from disk**. PXE's job is done at "installer launched."
- Mirror the reliability draft's **installed → boot-from-disk** idea: once pc01
  is known-installed, serve it an `exit` / boot-local iPXE script so a stray
  re-PXE doesn't reinstall. (Manual ack is fine for v1; auto-detection of
  "Proxmox is up" is a nice-to-have, not required.)
- Events still flow through the NDJSON tap (`discover → tftp → kernel →
  initrd → …`) so the operator/agent can watch progress in a readable log,
  per the reliability draft.

**Boundary:** PXE installs the hypervisor; it does **not** create VMs. The
talos-pc01 / windows VMs (`proxmox/*.conf`, future Crossplane) are downstream
and out of scope here.

## 8. Scope of code changes

```
internal/config/
  config.go            # add PXEBoot{Target, Version} under Boot; validation
internal/netboot/
  proxmox/
    resolver.go        # Resolve(version) → BootSpec; "latest" + pinned
    resolver_test.go   # parse fixture of the iso/ listing; semver sort
internal/pxe/
  serve.go             # dispatch: per-MAC target → which iPXE script
  ipxe_proxmox.go      # generic-ISO iPXE template (memdisk first)
  embed.go             # ship any needed memdisk/iPXE bits
internal/cli/
  node.go / node_add   # wizard prompts for target + version (optional)
  schema/schema.go     # surface the new fields in `nostos schema`
```

Reuses unchanged: interface detection, dnsmasq spawn, sudo handling,
event/NDJSON tap, asset cache.

## 9. Open decisions

- **Boot mechanism**: memdisk (A) vs kernel+initrd (B) for the first cut.
  Leaning A (RAM is ample on pc01, fewer PVE-version footguns).
- **`latest` resolution source**: scrape `download.proxmox.com/iso/` directory
  listing vs. a more stable signal (checksum index / enterprise mirror). Listing
  is simplest; record the resolved version for reproducibility either way.
- **Unattended install**: ship a Proxmox `answer.toml` (automated installer,
  available in recent PVE) vs. interactive install for v1. Interactive is fine
  to start; the answer file is the natural follow-up and keeps it YAML-native.
- **Installed-state detection**: manual ack vs. probing pc01:8006 (PVE web UI)
  to flip to boot-from-disk automatically.
- **Generalization**: keep `target` an open enum (proxmox now, debian/ubuntu
  later) vs. a narrower `proxmox`-only field. Recommend open enum, single
  resolver shipped.

## 10. Sequencing

1. **Schema** — add `boot.pxe.{target,version}` + validation + `nostos schema`.
   (No behavior change; Talos path untouched.)
2. **Resolver** — `internal/netboot/proxmox` with `latest` + pinned, unit-tested
   against a captured `iso/` listing fixture. No live network in tests.
3. **Dispatch + iPXE template** — serve emits the Proxmox (memdisk) script for
   `target: proxmox` MACs; Talos MACs unchanged.
4. **pc01 onboarding** — add pc01 to `config.yaml`, `nostos pxe serve`, boot it,
   confirm the installer launches and writes to `install_disk`.
5. **Polish** — installed→boot-local script; record resolved version + sha256;
   optional answer-file unattended install.

## 11. First concrete target (pc01)

```yaml
# nostos/config.yaml (addition)
nodes:
  pc01:
    mac: "fc:3c:d7:27:66:17"
    ip: 192.168.68.101
    install_disk: /dev/nvme0n1     # confirmed: first M.2 NVMe
    boot:
      method: pxe
      pxe:
        target: proxmox
        version: latest
```

Then: `nostos pxe serve` → power on pc01 → Proxmox installer over the network →
install to `/dev/nvme0n1` → reboot → Proxmox on bare metal → (downstream) create
talos-pc01 + windows VMs.

### pc01 hardware (Gigabyte B450 Aorus Elite V2, rev 1.x)

Confirmed from the board spec; resolves most of the earlier unknowns:

| Spec | Value | Impact |
|---|---|---|
| NIC | Realtek GbE (RTL8111/8168 family) | UEFI PXE-capable. **Must enable "Network Stack" + IPv4 PXE** in the AMI UEFI boot settings; Realtek PXE works but is the one likely-fiddly spot. |
| Firmware | licensed AMI UEFI | Use UEFI PXE → matches nostos `ipxe.efi`. CSM/legacy also available if UEFI PXE misbehaves. |
| Storage | **Dual M.2 (2× NVMe)** + 4× SATA | Matches "two nvme". Install target is one of the two M.2 — confirm device path in the installer (likely `/dev/nvme0n1`). |
| GPU | discrete (NVIDIA) in the x16 slot | Passthrough is a **Phase-2 (VM)** concern, not PXE. Note: B450/Ryzen IOMMU groups can be coarse; the x16 GPU may share a group — verify with `lspci`/IOMMU listing once Proxmox is up. |

**Confirmed (2026-06-08):**
- Install target: `/dev/nvme0n1` (first M.2 NVMe). Verify visually in the
  installer disk picker before committing.
- BIOS Network Stack / IPv4 PXE: enabled. Validated empirically on the first
  serve (pc01 appears in the event log → it's on).
- DHCP coexistence: LAN router (192.168.68.1) runs DHCP; nostos `--proxy` mode
  (the default) handles this with no extra config.

### Downstream (Phase 2, out of scope here)
GPU passthrough for the `windows` / `talos-pc01` VMs needs IOMMU enabled
(`amd_iommu=on iommu=pt` kernel args on the Proxmox host) and acceptable IOMMU
group isolation for the GPU — a B450 caveat to validate after the hypervisor is
installed, tracked separately from this PXE work.
