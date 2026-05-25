# nostos operator guide (v0.3)

> νόστος — Homer's word for homecoming. The Odyssey is one long nostos. So is
> every bare-metal install.

This is the canonical operator guide for `nostos`, the single Go CLI that
takes a stack of bare-metal boxes and turns them into a Talos Linux
Kubernetes cluster, with Tailscale joined and ArgoCD ready to GitOps. It
covers the two boot methods that ship in v0.3 — **PXE** (x86) and **TPI**
(Turing Pi 2 BMC for RK1 boards) — plus four per-vendor playbooks
(`dell-optiplex-3080m`, `turing-rk1`, `generic-amd64`, `raspberry-pi-5`),
the secrets pipeline (1Password + Tailscale OAuth), the read-only
dashboard TUI, the MCP server, and recovery for every failure mode the
author has actually hit.

The guide is the running document for this home lab. Every command is
copy-pasteable; every error message has a "what it means" entry; every
"why" call-out explains a design choice that affects the operator. The
intended reader is a single operator on a laptop with `op signin`
available, the Turing Pi 2 BMC reachable as `turingpi.local`, and a Dell
OptiPlex 3080M wired into the same `/24`.

```
Generated: 2026-05-25.  nostos 0.3.0.  Talos v1.10.3.
Repo:      github.com/yurifrl/home-systems  (path: nostos/, .submodules/nostos/)
```

---

## Table of contents

- [0. What this gets you](#0-what-this-gets-you)
- [1. Hardware checklist](#1-hardware-checklist)
- [2. First-time setup](#2-first-time-setup)
- [3. The PXE flow (Dell, generic amd64)](#3-the-pxe-flow-dell-generic-amd64)
- [4. The TPI flow (Turing Pi 2 / RK1)](#4-the-tpi-flow-turing-pi-2--rk1)
- [5. Per-vendor playbooks](#5-per-vendor-playbooks)
  - [5.1 dell-optiplex-3080m](#51-dell-optiplex-3080m)
  - [5.2 turing-rk1](#52-turing-rk1)
  - [5.3 generic-amd64](#53-generic-amd64)
  - [5.4 raspberry-pi-5](#54-raspberry-pi-5)
- [6. Tailscale OAuth setup](#6-tailscale-oauth-setup)
- [7. Recovery scenarios](#7-recovery-scenarios)
- [8. The dashboard](#8-the-dashboard)
- [9. Reference](#9-reference)
  - [9.1 Full CLI surface](#91-full-cli-surface)
  - [9.2 Config schema (annotated nostos.yaml)](#92-config-schema-annotated-nostosyaml)
  - [9.3 Exit codes](#93-exit-codes)
  - [9.4 Error catalogue](#94-error-catalogue)
  - [9.5 MCP integration](#95-mcp-integration)
  - [9.6 File layout](#96-file-layout)
  - [9.7 Common one-liners](#97-common-one-liners)

---

## 0. What this gets you

A working **Talos Linux** cluster with N nodes (one controlplane minimum,
0…N workers) where every node's kubelet reports **Ready**, every node is
joined to your **Tailscale** tailnet with the right tag, the kubeconfig
context lives in `nostos/state/kubeconfig`, and the cluster is in shape
for **ArgoCD** to take over. nostos owns bare-metal-to-cluster only:
once `nostos status` shows everything green, the cluster is a normal
Talos cluster and you should drive it with `kubectl` / `argocd` / Helm
from there. nostos does not run apps, manage namespaces, or manage
GitOps state.

---

## 1. Hardware checklist

This guide explicitly supports four platforms in v0.3. Anything
amd64-with-UEFI-and-PXE will work as `generic-amd64`. ARM64 is
RK1-via-TPI or Raspberry Pi 5 only.

### 1.1 Required (all platforms)

- A wired LAN segment with **DHCP** available to the boot NIC. nostos's
  `pxe` server binds to one interface and serves iPXE chainload via
  `dnsmasq` proxy DHCP — it does **not** replace your existing DHCP
  server, but the existing server must hand out IPs to the booting
  node's MAC.
- An operator workstation on the same `/24` (or otherwise able to
  reach the booting node on TCP 50000 — Talos maintenance API).
  macOS or Linux. Go 1.22+, `talosctl`, `dnsmasq` (`brew install
  dnsmasq` on macOS), Docker for the first `nostos build`, and one
  of: `op` (1Password CLI) / sops / env vars / plain files.
- A Tailscale tailnet (for cluster join) — see [§6](#6-tailscale-oauth-setup).
- A 1Password account with a `kubernetes` vault — see [§2.4](#24-secrets-backend-1password).

### 1.2 Per-platform

#### x86 mini-PC / NUC / Dell OptiPlex (PXE method)

- UEFI boot. Legacy/CSM **off**. Secure Boot **off** (Talos kernels
  are not signed for vendor CAs).
- Onboard NIC with PXE option ROM enabled. Wake-on-LAN optional but
  helpful for `--reinstall`.
- Install disk: NVMe preferred (`/dev/nvme0n1`); SATA SSD ok
  (`/dev/sda`); spinning rust unsupported.
- BIOS boot order: `IPv4 PXE` first, internal disk second.
- See playbook [§5.1 dell-optiplex-3080m](#51-dell-optiplex-3080m) or
  [§5.3 generic-amd64](#53-generic-amd64).

#### Turing Pi 2 + RK1 SoMs (TPI method)

- Turing Pi 2 carrier board. Up to 4 RK1 SoMs in slots 1–4.
- Each RK1: 8 GB or 16 GB RAM variants both work; eMMC (`/dev/mmcblk0`)
  or NVMe (`/dev/nvme0n1`) install disk.
- BMC reachable on the LAN (default mDNS name `turingpi.local`).
  **Rotate the default `root` password during BMC first-boot** and
  store it in 1Password.
- The Talos schematic for RK1 **must** include the
  `siderolabs/sbc-rockchip` overlay. Mint a schematic at
  `https://factory.talos.dev` with that overlay; paste the resulting
  ID into the per-node `schematic_id:` field. Example:
  `3616c4c824f2540c0a14da0cc8e6fc46143f2ca0cc75c9c6376a66e562894950`.
- The `tpi` CLI is invoked via SSH-on-BMC; the BMC firmware must be
  recent enough to expose `tpi flash --image …` and `tpi power
  on/off`.
- See playbook [§5.2 turing-rk1](#52-turing-rk1).

#### Raspberry Pi 5 (PXE method, arm64)

- EEPROM bootloader at the latest stable. NVMe or USB 3 install disk.
  SD only for development; high wear.
- Talos schematic must include `siderolabs/sbc-raspberrypi`.
- See playbook [§5.4 raspberry-pi-5](#54-raspberry-pi-5).

### 1.3 Network expectations

- Single `/24`. Mixed VLANs work but are unsupported here.
- DHCP server hands out IPs to booting MACs. nostos runs `dnsmasq`
  in **proxy DHCP** mode by default — it adds the iPXE
  `next-server`+`filename` options without offering a lease.
- Outbound HTTPS to `factory.talos.dev`, `api.tailscale.com`, and
  whichever container/Helm registries your apps use.
- TCP 50000 from operator → node (Talos maintenance API), TCP
  6443 from operator → controlplane VIP (Kubernetes API).

> **Why proxy DHCP?** Your home router almost certainly already
> hands out leases. Replacing it would create a second authoritative
> DHCP server and bring the network down. Proxy DHCP only adds boot
> options to existing leases.

---

## 2. First-time setup

### 2.1 Install nostos

In this repo the binary is built into `.bin/nostos` for fast local
runs, but the canonical invocation is `go run`:

```bash
cd /Users/yuri/Workdir/Yuri/home-systems
go run ./.submodules/nostos/cmd/nostos --version
# nostos version 0.3.0
```

Or use the cached binary:

```bash
./.bin/nostos --version
# nostos version 0.3.0
```

> **Why `go run`?** No install step, no version drift between dev
> machines, no "which nostos is on PATH" confusion. The `go.work`
> at the repo root pins the submodule.

### 2.2 `nostos init`

Run from an empty directory; scaffolds `config.yaml`, `templates/`,
and `state/` with sane defaults:

```bash
mkdir -p ~/labs/example && cd ~/labs/example
go run /Users/yuri/Workdir/Yuri/home-systems/.submodules/nostos/cmd/nostos init
```

The result:

```
.
├── config.yaml         ← edit this
├── templates/          ← per-node Talos machineconfig templates
└── state/              ← cache; gitignored; rebuildable
    ├── assets/         ← Talos kernel/initramfs/iPXE
    ├── cache/          ← image cache + digest TOFU records
    ├── configs/        ← rendered, secret-bearing per-MAC machineconfigs
    └── logs/
```

In **this** repo the equivalent already exists at
`/Users/yuri/Workdir/Yuri/home-systems/nostos/`. Don't re-init on top
of it.

### 2.3 Edit `nostos.yaml`

The minimum viable config has a `cluster:` block, a `secrets:` block,
and at least one entry under `nodes:`. The full annotated schema is in
[§9.2](#92-config-schema-annotated-nostosyaml). The home-systems
config is reproduced here:

```yaml
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
  talos_version: v1.10.3
  schematic_id: 4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b
secrets:
  backend: onepassword
  onepassword:
    account: my.1password.com
    vault: kubernetes
  tailscale:
    oauth_client_id_ref:     op://kubernetes/talos/tailscale-oauth-client-id
    oauth_client_secret_ref: op://kubernetes/talos/tailscale-oauth-client-secret
    tags:        [tag:k8s]
    expiry:      7776000
    reusable:    false
    ephemeral:   false
    preauthorized: true
    description: nostos
nodes:
  dell01:
    mac: "d0:94:66:d9:eb:a5"
    ip: 192.168.68.100
    role: controlplane
    arch: amd64
    install_disk: /dev/nvme0n1
    template: dell01.yaml
  tp1:
    ip: 192.168.68.107
    role: worker
    arch: arm64
    install_disk: /dev/mmcblk0
    template: tp1.yaml
    schematic_id: 3616c4c824f2540c0a14da0cc8e6fc46143f2ca0cc75c9c6376a66e562894950
    boot:
      method: tpi
      tpi:
        host: turingpi.local
        slot: 1
        username_ref: op://kubernetes/home-systems/TURING_BMC_USERNAME
        password_ref: op://kubernetes/home-systems/TURING_BMC_PASSWORD
```

Key fields:

- `cluster.endpoint` — the **first** controlplane's IP. nostos will
  drive the bootstrap against this address.
- `cluster.talos_version` — pinned. `nostos build` downloads exactly
  this version's image+iPXE.
- `cluster.schematic_id` — default schematic for nodes that don't
  override. Must include `siderolabs/tailscale` if you use Tailscale.
- `nodes.<name>.schematic_id` — per-node override (required for RK1
  because of the rockchip overlay).
- `nodes.<name>.boot.method` — `pxe` (default if absent) or `tpi`.
- `nodes.<name>.template` — file in `templates/` to render.

> **Why pin Talos version + schematic ID?** Rebuilding with a moving
> target is how you get a cluster where dell01 is on v1.10.3 and tp1
> is on v1.10.5 with a kernel mismatch in some CSI driver. Pin
> everything, bump deliberately.

### 2.4 Secrets backend (1Password)

Install the 1Password CLI, sign in, and enable the **CLI integration**
in the desktop app (Settings → Developer → "Connect with 1Password
CLI").

```bash
op signin
op vault list
# you should see: kubernetes
```

Create three items in the `kubernetes` vault:

1. `talos/tailscale-oauth-client-id` — the OAuth client ID from
   [§6](#6-tailscale-oauth-setup).
2. `talos/tailscale-oauth-client-secret` — the OAuth client secret.
3. `home-systems/TURING_BMC_USERNAME` and `home-systems/TURING_BMC_PASSWORD`
   for the Turing Pi BMC.

Validate the backend:

```bash
./.bin/nostos secrets list --output json
./.bin/nostos secrets test --output json
```

Each returns one record per scheme (`env`, `file`, `op`, `tailscale`)
with `status: PASS` or `status: FAIL` plus an `error`. PASS for `env`
and `file` is automatic — they have no probe state. PASS for `op`
means the active session resolved a probe path. PASS for `tailscale`
means a real auth-key was minted **and revoked** through the OAuth
client (round-trip).

> **Why mint+revoke as the test?** A read-only OAuth scope check would
> not catch the most common failure (operator forgot to add `tag:k8s`
> to `tagOwners` in the ACL). The mint-revoke cycle exercises the same
> code path that `nostos node install` will use later.

If `secrets test op` fails with `1Password session not active; run:
op signin`: run `op signin` and retry. If `secrets test tailscale`
fails with HTTP 403 or "tag not in tagOwners": go to
[§6](#6-tailscale-oauth-setup) and fix the ACL.

---

## 3. The PXE flow (Dell, generic amd64)

End-to-end install of a Dell OptiPlex 3080M as the controlplane. The
TPI flow ([§4](#4-the-tpi-flow-turing-pi-2--rk1)) is structurally the
same; the differences are flagged.

### 3.1 Preflight

```bash
cd /Users/yuri/Workdir/Yuri/home-systems
./.bin/nostos status --output json | jq '.cluster.healthy'
./.bin/nostos secrets test --output json
```

If `secrets test tailscale` fails, **stop**. A node wiped before the
secrets pipeline is healthy will land in PXE-loop limbo because
`nostos render` will fail to mint a Tailscale authkey, which will
fail to render the per-MAC config, which means the node will boot
into Talos maintenance and time out.

### 3.2 Build assets

`nostos build` downloads Talos kernel/initramfs/iPXE binary for the
pinned schematic and version. Idempotent; safe to re-run.

```bash
./.bin/nostos build
```

Sample output:

```
→ resolving schematic 4a0d65c6... v1.10.3 amd64
✓ kernel:   nostos/state/assets/v1.10.3/amd64/vmlinuz       (cached)
✓ initrd:   nostos/state/assets/v1.10.3/amd64/initramfs.xz  (cached)
✓ ipxe:     nostos/state/assets/v1.10.3/amd64/ipxe.efi      (built)
```

### 3.3 Render the per-node config

```bash
./.bin/nostos render dell01 --dry-run --output json
./.bin/nostos render dell01
```

Dry-run emits a Plan envelope:

```json
{"status":"preview","method":"render","would_execute":[
  {"phase":"resolve-secrets","detail":"op://kubernetes/talos/cluster-ca → state/cache/secrets/cluster-ca"},
  {"phase":"mint-tailscale","detail":"oauth → tag:k8s, expiry=7776000"},
  {"phase":"write-config","detail":"state/configs/d0:94:66:d9:eb:a5.yaml"}
]}
```

The real run resolves all `op://` refs, mints a fresh Tailscale auth
key, and writes `nostos/state/configs/<mac>.yaml`. The file is
gitignored — it contains the cluster CA private key, the etcd
encryption secret, the Tailscale authkey, and any other secret
referenced from the template. **Don't commit it. Don't email it.**

### 3.4 Start the PXE server

`pxe` runs in the foreground until Ctrl+C. `dnsmasq` needs root for
ports 67/69.

```bash
sudo ./.bin/nostos pxe --iface en0
```

Sample log:

```
→ pxe: serving on 192.168.68.50  (iface en0)
→ http: state/assets/v1.10.3/amd64/ → :8080
→ http: state/configs/             → :8080/configs/
→ dnsmasq: proxy-dhcp on 192.168.68.0/24
   next-server: 192.168.68.50
   filename:    ipxe.efi
ready. waiting for chainload requests.
```

Leave it running and open a second terminal for the install command.

### 3.5 Install the node

```bash
./.bin/nostos node install dell01 --yes
```

What this does (also visible as
`nostos node install dell01 --dry-run --output json`):

1. **Preflight** — confirms the node is in `config.yaml`, the per-MAC
   config is rendered, the PXE server is reachable, and (for a
   re-install) the operator passed `--reinstall`.
2. **Power-on / wait** — the operator powers the box on. `nostos`
   polls the iPXE chainload request log; when the booting MAC
   matches the configured one, it knows the chainload landed.
3. **Maintenance probe** — once the booting node enters Talos
   maintenance mode (TCP 50000 listening, no apid yet), nostos
   reports `up`. PXE deadline default: 20 minutes.
4. **Apply machineconfig** — `talosctl apply-config --insecure -f
   state/configs/<mac>.yaml`. Talos installs the OS to
   `install_disk`, reboots, and joins kubelet.
5. **Bootstrap** (controlplane only) — once apid responds, `nostos
   bootstrap dell01` runs `talosctl bootstrap` against the node and
   waits for etcd quorum.
6. **Fetch kubeconfig** — `talosctl kubeconfig` writes
   `nostos/state/kubeconfig`.
7. **Verify Ready** — `kubectl --kubeconfig nostos/state/kubeconfig
   get nodes` in a poll loop until `Ready`.

Sample session output (lifted from a real `dell01` install):

```
About to install dell01 (method=pxe). Pass --yes to skip prompt.
→ preflight: config ok, mac d0:94:66:d9:eb:a5 rendered
→ preflight: pxe http :8080 reachable
→ waiting for chainload (timeout: 20m0s)
... [power-cycled the box] ...
→ chainload: GET /ipxe.efi from 192.168.68.100  (12s)
→ chainload: GET /v1.10.3/amd64/vmlinuz         (18s)
→ chainload: GET /configs/d0:94:66:d9:eb:a5.yaml(31s)
→ talos maintenance: up at 192.168.68.100:50000 (1m24s)
→ apply-config (--insecure)
✓ apply-config: ok
→ waiting for apid (post-reboot)
→ apid: up at 192.168.68.100:50000              (4m02s)
→ bootstrap etcd
✓ etcd: 1/1 healthy
→ fetch kubeconfig → nostos/state/kubeconfig
→ kubelet Ready: dell01                          (5m18s)
DONE. dell01 is Ready.
```

Total wall-clock: 5–7 min on Dell, 12–18 min on RK1.

### 3.6 Errors you might see

| Error | What it means |
|---|---|
| `error[validation_failed/E_NODE_NAME_FORMAT]` | The node name has illegal characters. Use `[a-zA-Z0-9][a-zA-Z0-9-]{0,62}`. |
| `error[not_found/E_NODE_NOT_FOUND]: node "dell01" not in config.yaml` | Add a `nodes.dell01:` block. |
| `error[network_error/E_BMC_UNREACHABLE]` | TPI-only — the BMC is offline or behind a different IP. See [§7.2](#72-bmc-unreachable). |
| `error[auth_error/E_BMC_AUTH]` | BMC creds wrong. Re-pull from 1Password; verify `op read op://kubernetes/home-systems/TURING_BMC_PASSWORD`. |
| `error[timeout/E_WAIT_MAINTENANCE]` | Talos didn't reach maintenance within the deadline. Check the node UART / iDRAC console. RK1: see [§7.5](#75-half-flashed-rk1). |
| `error[conflict/E_NODE_READY]: dell01 is already Ready; pass --reinstall` | The orchestrator short-circuited because the kubelet is already Ready. Pass `--reinstall --yes`. |
| `error[validation_failed/E_TLS_FACTORY_DIGEST]` | First-run TOFU recorded a different digest than what factory.talos.dev now serves. Either bump `talos_version` deliberately, or delete `nostos/state/cache/digests.json` and let TOFU re-record. |

---

## 4. The TPI flow (Turing Pi 2 / RK1)

Same lifecycle as PXE; the chainload step is replaced by
**`tpi flash`** (BMC-side write of the Talos image to eMMC/NVMe) and
the maintenance-mode wait deadline is bumped to 30 min.

### 4.1 Per-node schematic_id

The Talos default factory image does **not** boot on RK1. Mint a
schematic at `https://factory.talos.dev` with the
`siderolabs/sbc-rockchip` overlay (and `siderolabs/tailscale` if
you're using Tailscale). Paste the ID into the per-node
`schematic_id:`:

```yaml
nodes:
  tp1:
    arch: arm64
    schematic_id: 3616c4c824f2540c0a14da0cc8e6fc46143f2ca0cc75c9c6376a66e562894950
    install_disk: /dev/mmcblk0
    template: tp1.yaml
    boot:
      method: tpi
      tpi:
        host: turingpi.local
        slot: 1
        username_ref: op://kubernetes/home-systems/TURING_BMC_USERNAME
        password_ref: op://kubernetes/home-systems/TURING_BMC_PASSWORD
```

> **Why per-node?** Different SoMs need different overlays. dell01 is
> generic amd64; tp1/tp4 are RK1; a Pi 5 needs raspberrypi. Letting
> nodes override the cluster-default `schematic_id` means a single
> repo can drive a heterogeneous cluster.

### 4.2 BMC creds via op://

The BMC username and password are referenced through `op://`. nostos
resolves them at preflight time, never logs them, and never writes
them to `state/`. To rotate:

1. Set the new password via the BMC web UI (Settings → Users).
2. Update the 1Password item.
3. Re-run `nostos secrets test op` to confirm.

Default creds (`root`/`turing`) are unsupported. nostos will refuse
to install if the username is `root` and the password matches the
first-boot default; rotate before you start.

### 4.3 Image cache TOFU

The `tpi` provider downloads the Talos arm64 raw image once into
`nostos/state/cache/<schematic_id>/<version>/<arch>/metal.raw.xz`
and records its SHA-256 in `digests.json`. Subsequent installs
verify the recorded digest before flashing. The cache is shared
across nodes that use the same schematic+version+arch (so tp1 and
tp4 both pull from the same cached file).

> **Why TOFU instead of strict-pin?** v0.2 shipped strict-pin and the
> operator (me) had to manually `sha256sum` the factory image once
> per node before the first install. The dance was tedious enough
> that v0.3 relaxes to TOFU: first run records, subsequent runs
> verify. A WARN line tells you you're in TOFU mode. To strict-pin
> later, copy the recorded digest into `cluster.image_digests:` and
> nostos refuses to download anything else.

### 4.4 The `tpi` dance

```bash
./.bin/nostos node install tp1 --yes
```

Internally:

```
→ preflight: tcp turingpi.local:443 ok
→ preflight: GET / (auth) ok, BMC firmware 2.x.x
→ image cache: state/cache/3616c4c.../v1.10.3/arm64/metal.raw.xz
   sha256:e3b0c44... (verified)
→ tpi power off --node 1
→ tpi flash --image .../metal.raw.xz --node 1
   ... ~3-6 min ...
→ tpi power on --node 1
→ waiting for maintenance (timeout: 30m0s)
→ talos maintenance: up at 192.168.68.107:50000 (4m32s)
→ apply-config (--insecure)
→ apid: up at 192.168.68.107:50000              (8m11s)
→ kubelet Ready: tp1                            (9m44s)
DONE. tp1 is Ready.
```

### 4.5 Manual apply-config fallback

If WaitMaintenance trips (the node entered maintenance later than
the 30 min deadline) but you can confirm via `talosctl version
--insecure -n 192.168.68.107` that the node IS reachable, you can
finish the install by hand:

```bash
talosctl --talosconfig nostos/state/talosconfig \
  apply-config --insecure \
  -n 192.168.68.107 \
  --file nostos/state/configs/<mac>.yaml
```

Substitute the actual MAC (find it via `ls nostos/state/configs/`).
After `apply-config` lands, run:

```bash
./.bin/nostos bootstrap tp1   # only for the FIRST controlplane
./.bin/nostos kubeconfig tp1  # any node — refreshes state/kubeconfig
./.bin/nostos status
```

### 4.6 `cluster cleanup` for zombies

When you reflash an RK1, the old node entry sometimes lingers:
`kubectl get nodes` lists the zombie name (`talos-76w-r75`),
Tailscale tailnet keeps the offline ephemeral device. v0.3 ships
`nostos cluster cleanup` to reconcile both surfaces against the
nostos config.

```bash
./.bin/nostos cluster cleanup --dry-run --output json
./.bin/nostos cluster cleanup --age-days 7 --yes --really-yes
```

Two confirms (`--yes --really-yes`) because a wrong cleanup wipes
real nodes. Read the dry-run output before the real run.

---

## 5. Per-vendor playbooks

The four playbooks ship embedded in the binary and live as Markdown
under
`.submodules/nostos/internal/dashboard/playbooks/embed/<vendor>-<model>.md`.
The dashboard's `s` (setup-info) chord renders them in a Glamour
panel keyed by the selected node's `vendor-model` tag. Operator
overlays at `nostos/docs/<vendor>-<model>.md` merge over the
defaults.

The full text follows.

### 5.1 dell-optiplex-3080m

```
# Dell OptiPlex 3080M — Talos PXE Setup

This playbook captures the BIOS knobs the Dell OptiPlex 3080M needs to PXE-boot
into Talos and the disk choices that have bitten me.

## BIOS settings

1. Reboot → tap **F2** to enter Setup.
2. **Boot Sequence** → set to **UEFI**.
3. **Secure Boot** → **Disabled** (Talos kernels are not signed for Dell's CA).
4. **Integrated NIC** → **Enabled w/ PXE Boot**.
5. **Boot Order** → put `IPv4 Ethernet` ahead of any local disk for the install,
   then re-order back to NVMe-first after the first successful install.

## Disks

- The 3080M has both an NVMe slot (M.2 2280) and a 2.5" SATA bay.
- Talos `install_disk` should target `/dev/nvme0n1` — confirm with
  `talosctl -n <ip> disks` after maintenance boot.

## Recovery

If PXE chainload hangs, hold the power button 10s, re-enter BIOS, and verify
the NIC's option ROM is still **enabled** — Dell's BIOS sometimes resets this
after a CMOS battery dip.
```

### 5.2 turing-rk1

```
# Turing RK1 — Talos on the Turing Pi 2 BMC

The RK1 is an ARM64 SoM with eMMC and an optional NVMe SSD; install through
the Turing Pi 2 BMC (`tpi`) CLI.

## BMC credentials

- Default user is `root`, password is set during BMC firmware first-boot.
- Rotate via the BMC web UI → **Settings → Users**.
- Persist the rotated password in your secrets backend; reference it from
  `nostos/config.yaml` via `boot.tpi.password_ref: op://...`.

## Slot mapping

The BMC numbers slots **1..4** (left-to-right looking at the front panel).
A node's `boot.tpi.slot` MUST match the physical slot, not its position in
your config.

## Boot order

- eMMC vs NVMe: the RK1 will prefer NVMe if present and bootable. To wipe
  and re-flash, use `tpi power off → tpi flash --image <talos> --node <slot>`.
- After install, set `install_disk: /dev/nvme0n1` (NVMe present) or
  `/dev/mmcblk0` (eMMC only).

## SPI flash recovery

If a node bricks during flash, hold the **MASKROM** button on the SoM while
power-cycling the BMC; the SoM will enumerate as a USB device on the BMC's
host port for `rkdeveloptool` recovery.
```

### 5.3 generic-amd64

```
# Generic amd64 — BIOS / UEFI playbook

This playbook covers generic amd64 hosts (mini-PCs, NUCs, OptiPlexes that
aren't the 3080M variant) onboarded via Talos PXE.

## Required BIOS / UEFI settings

1. **Boot mode**: UEFI only (Legacy / CSM disabled).
2. **Secure Boot**: **OFF**. Talos kernels are not signed for vendor keys.
3. **TPM**: Either state works, but leave consistent across re-installs.
4. **Fast Boot**: OFF — full POST is required for reliable PXE.
5. **CPU virtualization**: VT-x / AMD-V **ON** (kube workloads expect it).

## Boot order

Set the first boot device to **IPv4 PXE / Network**, with the install disk
as second (so the box boots from disk after Talos lands the ISO):

    1) IPv4 PXE / NIC (LAN1)
    2) Internal NVMe / SATA install disk
    3) USB / removable media

## NIC configuration

- Wake-on-LAN: ON if you intend to use `nostos node install --reinstall`.
- DHCP must be available on the PXE VLAN; nostos pxe-server handles the
  `next-server` / `filename` handoff.

## Install disk

- Recommended: NVMe (`/dev/nvme0n1`). Fast wipes, fast boots.
- SATA SSD (`/dev/sda`) works; spinning rust is unsupported.
- For dual-disk boxes, set `install_disk:` explicitly in `config.yaml` —
  do not rely on Talos auto-pick.

## Re-flash workflow

    nostos node install <name> --reinstall --yes

The TUI's `r` keybind invokes the same path.
```

### 5.4 raspberry-pi-5

```
# Raspberry Pi 5 — Talos playbook

The Pi 5 needs a small ritual before Talos can boot it. Do this **once**
per board, then it joins the cluster like any other node.

## Bootloader EEPROM update

Use the latest stable EEPROM firmware. From a Raspberry Pi OS image:

    sudo apt update && sudo apt full-upgrade
    sudo rpi-eeprom-update -a
    sudo reboot

Verify with `vcgencmd bootloader_version` after reboot.

## Boot order — enable USB / NVMe boot

    sudo raspi-config
    # → Advanced Options → Bootloader Version → Latest
    # → Advanced Options → Boot Order → USB Boot

Equivalent CLI:

    sudo rpi-eeprom-config --edit
    # set:
    BOOT_ORDER=0xf416   # try NVMe, then USB, then SD

## config.txt notes

The Pi 5 boots Talos via the standard arm64 image; nostos's schematic
already includes the `siderolabs/sbc-raspberrypi` overlay. Do **not**
hand-edit `config.txt` — Talos manages it. If you must add HAT-specific
DT overlays, do it through the Talos machine config patch, not on the
SD card directly.

## Storage choice

| Medium | When to use                                         | install_disk        |
| ------ | --------------------------------------------------- | ------------------- |
| SD     | Dev / disposable nodes only; high wear              | /dev/mmcblk0        |
| USB 3  | Decent option for low-write workloads               | /dev/sda            |
| NVMe   | Recommended for clusters; needs HAT or CM5 carrier  | /dev/nvme0n1        |

The Pi 5 root filesystem must be **ext4** (Talos default). Do not pre-format.

## Network

- Use the onboard 1 GbE; the USB-Ethernet path is unreliable for PXE.
- DHCP must serve the Pi's MAC; nostos pxe-server handles the rest.

## Re-flash

    nostos node install <name> --reinstall --yes

If the bootloader doesn't pick PXE, hold the BOOTSEL flow per the Pi
imager docs and re-image the SD with the latest EEPROM, then retry.
```

---

## 6. Tailscale OAuth setup

Every Talos node joins the tailnet at boot via a freshly-minted
Tailscale auth-key. The key is minted by nostos against an **OAuth
client** scoped to `Auth Keys: Write`, with a `tag:k8s` (or whatever
you configure) attached. The OAuth client itself is the long-lived
secret; auth-keys are short-lived (90 d default, ephemeral=false,
preauthorized=true).

### 6.1 ACL — `tagOwners` for `tag:k8s`

In the Tailscale admin console (`https://login.tailscale.com/admin/acls`),
edit the policy file to make sure your OAuth client can mint keys
that own `tag:k8s`. The minimum delta:

```json
{
  "tagOwners": {
    "tag:k8s": ["autogroup:admin"]
  },
  "acls": [
    {"action": "accept", "src": ["tag:k8s"], "dst": ["tag:k8s:*"]},
    {"action": "accept", "src": ["autogroup:admin"], "dst": ["tag:k8s:*"]}
  ]
}
```

> **Why `autogroup:admin`?** OAuth clients run with the privileges
> of the entity that created them; `autogroup:admin` covers any
> admin in the tailnet. If you want a tighter scope, replace with
> a specific user or group, but then nostos must run as that user.

If you use a different tag (e.g. `tag:home-k8s`), update both the
ACL and `secrets.tailscale.tags` in `nostos.yaml`.

### 6.2 Create the OAuth client

1. Open `https://login.tailscale.com/admin/settings/oauth` in your
   browser.
2. Click **Generate OAuth client…**.
3. **Description**: `nostos`.
4. **Scopes**: tick **`Auth Keys`** → **`Write`** only. Untick
   everything else (no `Devices`, no `DNS`, no `Routes`).
5. **Tags**: add `tag:k8s` (the same tag you put in `tagOwners`).
6. Click **Generate**. Copy the **client ID** and **client secret**
   immediately — the secret is shown once.

### 6.3 Store in 1Password

Create two items in the `kubernetes` vault, both of type
`Password`:

- `talos/tailscale-oauth-client-id` (the public-ish client ID)
- `talos/tailscale-oauth-client-secret` (the secret)

Reference them from `nostos.yaml`:

```yaml
secrets:
  tailscale:
    oauth_client_id_ref:     op://kubernetes/talos/tailscale-oauth-client-id
    oauth_client_secret_ref: op://kubernetes/talos/tailscale-oauth-client-secret
    tags:        [tag:k8s]
    expiry:      7776000      # 90 days
    reusable:    false
    ephemeral:   false
    preauthorized: true
    description: nostos
```

### 6.4 Validate

```bash
./.bin/nostos secrets test tailscale --output json
```

PASS means: nostos exchanged the OAuth client for an access token,
minted a real auth-key with tag `tag:k8s`, then revoked it. If you
see `403 forbidden`, it almost always means `tag:k8s` is missing
from `tagOwners` (see [§6.1](#61-acl--tagowners-for-tagk8s)).

You can list and revoke individual auth keys:

```bash
./.bin/nostos secrets keys list --output json
./.bin/nostos secrets keys revoke kABCxyz123 --dry-run
./.bin/nostos secrets keys revoke kABCxyz123
```

> **Why mint a fresh key per render?** Talos machine configs are
> stored encrypted at rest, but a leaked rendered config would let
> an attacker join the tailnet as `tag:k8s`. Short-lived
> non-reusable keys mean a leaked config is useless after first
> boot.

---

## 7. Recovery scenarios

### 7.1 Node won't boot after flash

Symptoms: PXE chainload completes, but maintenance mode never comes
up. Or: TPI flash completes, `tpi power on` runs, the SoM stays
dark.

Checks:

1. **Schematic mismatch.** `nostos.yaml` says `arch: arm64` but the
   schematic is the amd64 default. Look in
   `nostos/state/assets/<version>/<arch>/` — the directory must
   match the node's arch. Fix: set the per-node `schematic_id` per
   [§4.1](#41-per-node-schematic_id) and re-run `nostos build`.
2. **eMMC vs SPI boot order (RK1).** RK1 honors NVMe first if a
   bootable NVMe is present, else eMMC. If you flashed eMMC but an
   old NVMe still has a bootloader, the SoM boots the NVMe. Power
   off, pull the NVMe, retry. Or: `tpi flash --image … --node <n>`
   targets eMMC by default; pass `--device /dev/nvme0n1` to flash
   NVMe.
3. **UART.** Connect the BMC's serial console (`tpi uart --node 1`)
   and watch for U-Boot output. `mmc read` errors or a
   `Synchronous Abort` panic during early kernel init usually
   means the schematic is wrong.
4. **Dell BIOS option-ROM reset.** After a CMOS battery dip the
   3080M sometimes disables the NIC option ROM, killing PXE silently.
   Re-enter BIOS, confirm `Integrated NIC` → `Enabled w/ PXE Boot`.

### 7.2 BMC unreachable

Error: `error[network_error/E_BMC_UNREACHABLE]: tpi: dial
turingpi.local:443: connect: device not configured (os error 6)`.

> **Why "device not configured"?** That's the macOS kernel's wording
> for "no route to host". The `tpi` subprocess returns the OS error
> verbatim; nostos translates it to `E_BMC_UNREACHABLE`.

Checks:

1. `ping turingpi.local` — does mDNS resolve? If not, look up the
   BMC IP in your router and either fix mDNS (avahi/Bonjour) or
   put the IP directly in `boot.tpi.host`.
2. `nc -vz turingpi.local 443` — is the BMC's HTTPS up?
3. `curl -k -u root:<password> https://turingpi.local/api/bmc/info` —
   does auth work? Some firmware revisions use `/redfish/v1/`
   instead of `/api/bmc/`. nostos preflights both.
4. Default creds rotation: if you never rotated `root`/`turing`,
   the BMC may have been replaced or factory-reset; re-rotate and
   update 1Password.

### 7.3 etcd quorum lost

Single-controlplane recovery (the home-lab norm):

```bash
talosctl --talosconfig nostos/state/talosconfig \
  -n 192.168.68.100 service etcd stop

talosctl --talosconfig nostos/state/talosconfig \
  -n 192.168.68.100 etcd snapshot \
  --from-node 192.168.68.100 \
  /tmp/etcd.snap

talosctl --talosconfig nostos/state/talosconfig \
  -n 192.168.68.100 bootstrap --recover-from /tmp/etcd.snap
```

If you have **no** snapshot and only one controlplane: the cluster
state is gone. Re-bootstrap (`nostos bootstrap dell01`) and let
ArgoCD reconcile workloads from git. Persistent data on PVCs
survives because Talos doesn't touch them.

For three-controlplane clusters, lose-one-recover:

```bash
talosctl --talosconfig nostos/state/talosconfig \
  -n 192.168.68.100 etcd remove-member \
  --node-id <broken-node-id>
./.bin/nostos node install <broken> --reinstall --yes
```

### 7.4 Tailscale revoked all keys

After a security incident or `op` re-shuffle, you may need to
nuke every minted auth-key and re-mint:

```bash
./.bin/nostos secrets keys list --output json | jq -r '.id' | \
  xargs -I {} ./.bin/nostos secrets keys revoke {}
./.bin/nostos secrets test tailscale
for n in dell01 tp1 tp4; do
  ./.bin/nostos render "$n"
done
```

Then `talosctl apply-config` each rendered file to its node, or
`nostos node install <n> --reinstall --yes` if the node is wipeable.

### 7.5 Half-flashed RK1

Symptom in `tpi uart`:

```
U-Boot SPL 2024.01-rk3588 (...)
mmc_load_image_raw_sector: mmc block read error
SPL: failed to boot from MMC1
### ERROR ### Please RESET the board ###
```

The flash got interrupted (operator hit Ctrl+C, BMC rebooted, or
power blip). Recovery:

1. `tpi power off --node <slot>`.
2. Re-run `nostos node install <name> --reinstall --yes`. The
   image cache is intact (digest is recorded only after Close+Rename),
   so the flash will resume in ~3 min instead of refetching.
3. If the SoM is fully bricked (no UART, no USB enumeration), hold
   the **MASKROM** button on the SoM and power-cycle the BMC; use
   `rkdeveloptool` from a machine plugged into the BMC's USB-C
   front port to re-image the SPI loader.

---

## 8. The dashboard

`nostos dashboard` is a Bubble Tea v2 single-window TUI that gives
you one pane: cluster health, per-node health, ArgoCD app health,
discovery sweep, version drift. v0.3 ships **action handlers**
(`r` reinstall, `d` delete, `i` identify) plus the read-only
chords (`s` setup-info, `u` upgrade preview, `n` name unknown,
`H` hide, `/` search, `?` help, `G` open guide section).

### 8.1 Interactive

```bash
./.bin/nostos dashboard
```

Use `j`/`k` to move between rows, `tab` to cycle panels (cluster /
nodes / apps / discoveries), `?` for help, `q` to quit. Action
chords are contextual to the selected row's bucket — see [§8.4](#84-action-chords).

### 8.2 Headless (`--once`)

For CI, cron, MCP-call from an agent, or a quick "what's the state":

```bash
./.bin/nostos dashboard --once --output json | jq '.aggregate_state'
./.bin/nostos dashboard --once --output json --fields aggregate_state,nodes,checks
./.bin/nostos dashboard --once --output json --no-upstream  # skip HTTP
```

`--no-upstream` skips the upstream-version diff (factory.talos.dev
HEADs, OCI registry HEADs). Use it on flaky links or in CI.

### 8.3 Refresh tiers

Two tiers; both run synchronously in the Bubble Tea event loop:

- **fast** (5 s) — ICMP, Talos apid, Tailscale presence, k8s API.
  Anything cheap enough to poll often.
- **slow** (5 min) — schematic-vs-running diff, Talos
  upstream-release diff, Helm chart version diff vs OCI registry,
  ArgoCD application sync state. Anything that needs a network
  round-trip to a third party.

Upstream diffs are cached at
`~/.cache/nostos/upstream-versions.json` with a 24 h TTL. Cold
start hydrates from cache, then refreshes in the background.

### 8.4 Action chords

Footer is contextual by selected row:

| Chord | Where | What |
|---|---|---|
| `n` | unknown discovery row | emit a `nodes:` patch on stdout (no write) |
| `H` | any row | toggle hidden/visible (persists in `~/.config/nostos/dashboard.toml`) |
| `s` | known node | open the per-vendor playbook in a Glamour panel |
| `u` | any | run `cluster upgrade --dry-run` and show the diff |
| `r` | known node | `node install <name> --reinstall --yes` (confirm) |
| `d` | known node | `node remove <name> --yes` (confirm) |
| `i` | known node | identify (Redfish blink → NIC packet flood → tpi UART probe) |
| `G` | any | open the relevant section of this guide |
| `/` | any | filter the rows |
| `?` | any | help (curated, not auto-gen) |

`r`/`d`/`i` are dispatched through the action seam at
`internal/dashboard/actions/`. To smoke-test without touching real
infra:

```bash
./.bin/nostos dashboard --dispatch=mock
```

The mock dispatcher logs intended actions but never spawns
subprocesses.

### 8.5 The five aggregate states

Top-bar shows exactly one of:

- **`ALL_GREEN`** — every probe in every tier passes. No imperative.
- **`DEGRADED`** — at least one non-fatal probe failed (e.g.
  upstream diff says Talos has a newer patch). Imperative line:
  "Press G to open the upgrade preview."
- **`BROKEN`** — at least one fatal probe failed (k8s API
  unreachable, kubelet NotReady, etcd unhealthy). Imperative:
  "Press G to open the recovery section for: …".
- **`UNCONFIGURED`** — zero nodes in `config.yaml`. Body replaced
  by a four-step CTA: `nostos init` → `nostos secrets test
  tailscale` → plug a node in → press `n` on the discovered MAC.
- **`TRANSITIONING`** — at least one per-node `flock` is held
  (`nostos node install` is in flight from another shell).
  Imperative: "Hold; install in progress."

> **Why an explicit `UNCONFIGURED`?** A green bar on a zero-node
> config is technically true but useless. The CTA is the
> opinionated alternative.

---

## 9. Reference

### 9.1 Full CLI surface

The list below is auto-generated from `./.bin/nostos schema --all`.
Re-generate any time with:

```bash
./.bin/nostos schema --all | jq -r 'to_entries[] | "- `\(.key)` — \(.value.description)"'
```

- `bootstrap` — Bootstrap etcd on NODE (first controlplane only).
- `build` — Download Talos assets + build iPXE binary.
- `cluster` — Cluster-level operations.
- `cluster.cleanup` — Reconcile k8s + Tailscale state with nostos config.
- `completion` — Generate the autocompletion script for the specified shell
- `completion.bash` — Generate the autocompletion script for bash
- `completion.fish` — Generate the autocompletion script for fish
- `completion.powershell` — Generate the autocompletion script for powershell
- `completion.zsh` — Generate the autocompletion script for zsh
- `config` — Config subcommands.
- `config.refresh` — Regenerate admin client certificate.
- `dashboard` — Live single-pane TUI for cluster + nodes + ArgoCD apps.
- `init` — Scaffold a new nostos project (config.yaml, templates/, state/).
- `kubeconfig` — Refresh state/kubeconfig from a running controlplane.
- `mcp` — Run JSON-RPC MCP server over stdio (one tool per cobra command).
- `node` — Manage node registrations.
- `node.install` — End-to-end install for NAME (method-dispatched: pxe|tpi).
- `node.list` — List registered nodes with live reachability.
- `node.remove` — Remove a node from config.yaml.
- `node.show` — Show one node's reachability and config
- `nuke` — Remove state/ entirely (regenerable from config.yaml).
- `pxe` — Start PXE server (HTTP + dnsmasq) until Ctrl+C.
- `render` — Render NODE's machineconfig with secrets injected.
- `schema` — Print machine-readable schema descriptors for nostos commands.
- `secrets` — Inspect and validate secret backends.
- `secrets.keys` — Tailscale auth-key inspection.
- `secrets.keys.list` — List Tailscale auth keys.
- `secrets.keys.revoke` — Delete a Tailscale auth key by id.
- `secrets.list` — List configured secret backends with Validate() status.
- `secrets.test` — Run Validate() against one (or all) backends.
- `status` — Show per-node reachability + Talos version.
- `up` — [deprecated] alias for `nostos node install`.
- `wipe` — Queue a one-shot disk wipe for NODE on its next PXE boot.

#### Mutation matrix

| Command | Idempotent | Destructive | Confirm |
|---|---|---|---|
| `init` | yes | no | no |
| `build` | yes | no | no |
| `render` | yes | no | no |
| `node list` / `node show` | yes | no | no |
| `status` | yes | no | no |
| `secrets list` / `secrets test` | yes | no | no |
| `secrets keys list` | yes | no | no |
| `schema` | yes | no | no |
| `dashboard --once` | yes | no | no |
| `node install` | no | YES | `--yes` |
| `node remove` | no | YES | `--yes` |
| `nuke` | no | YES | `--yes` |
| `wipe` | no | YES | no |
| `bootstrap` | no | YES | no |
| `secrets keys revoke` | yes (404 OK) | YES | no |
| `cluster cleanup` | depends | YES | `--yes --really-yes` |

Every mutation accepts `--dry-run` and emits a Plan envelope:

```json
{"status":"preview","method":"node.install","would_execute":[
  {"phase":"preflight","detail":"..."},
  {"phase":"flash","detail":"..."}
]}
```

### 9.2 Config schema (annotated nostos.yaml)

```yaml
# Cluster-wide settings.
cluster:
  # Logical name; appears in talosconfig contexts.
  name: talos-default
  # First controlplane endpoint. nostos bootstraps against this.
  endpoint: https://192.168.68.100:6443
  # Pin exactly one Talos version. `nostos build` downloads it.
  talos_version: v1.10.3
  # Default schematic ID. Overridden per-node when arch/overlays differ.
  # Mint at https://factory.talos.dev.
  schematic_id: 4a0d65c669d46663f377e7161e50cfd570c401f26fd9e7bda34a0216b6f1922b
  # Optional strict-pin map. When present, an entry must exist for every
  # (schematic_id, version, arch) tuple, and the recorded TOFU digest must
  # match. Refuses any download otherwise. Leave absent for TOFU.
  # image_digests:
  #   3616c4c8.../v1.10.3/arm64: sha256:e3b0c44...
  #   4a0d65c6.../v1.10.3/amd64: sha256:abcd1234...

# Secrets pipeline.
secrets:
  # Which backend resolves the op:// refs in templates.
  # Valid: onepassword | sops | env | file
  backend: onepassword

  onepassword:
    account: my.1password.com
    vault: kubernetes

  # Tailscale OAuth — see §6.
  tailscale:
    oauth_client_id_ref:     op://kubernetes/talos/tailscale-oauth-client-id
    oauth_client_secret_ref: op://kubernetes/talos/tailscale-oauth-client-secret
    tags:        [tag:k8s]
    expiry:      7776000          # seconds (90 days)
    reusable:    false
    ephemeral:   false
    preauthorized: true
    description: nostos

# Per-node registry.
nodes:
  dell01:
    # MAC of the booting NIC. Required for PXE; optional for TPI.
    mac: "d0:94:66:d9:eb:a5"
    # Static IP after install. Required for status/health probes.
    ip: 192.168.68.100
    # controlplane | worker
    role: controlplane
    # amd64 | arm64
    arch: amd64
    # Talos install_disk machineconfig setting.
    install_disk: /dev/nvme0n1
    # File in templates/.
    template: dell01.yaml
    # boot.method defaults to pxe if absent.
    # boot:
    #   method: pxe

  tp1:
    ip: 192.168.68.107
    role: worker
    arch: arm64
    install_disk: /dev/mmcblk0
    template: tp1.yaml
    # Per-node override for the rockchip overlay schematic.
    schematic_id: 3616c4c824f2540c0a14da0cc8e6fc46143f2ca0cc75c9c6376a66e562894950
    boot:
      method: tpi
      tpi:
        host: turingpi.local
        slot: 1
        username_ref: op://kubernetes/home-systems/TURING_BMC_USERNAME
        password_ref: op://kubernetes/home-systems/TURING_BMC_PASSWORD
```

### 9.3 Exit codes

Live catalogue: `./.bin/nostos schema --exit-codes`.

| Code | Category | Meaning |
|---|---|---|
| 0 | success | success |
| 1 | internal_error | un-classified bug; report it |
| 10 | validation_failed | bad input, schema violation, parse error |
| 11 | network_error | timeout, refused, DNS, TCP probe failed |
| 12 | auth_error | op session, BMC creds, OAuth scope |
| 13 | conflict | lock held, node already Ready, dup MAC |
| 14 | not_found | node not in config, key id absent, schema method |
| 15 | timeout | operation deadline exceeded |

> **Why 1 instead of, say, 9 for `internal_error`?** Compatibility with
> shell idioms (`set -e`, `&&`) which expect any non-zero to mean
> "stop". 1 is the universally-understood "something went wrong".
> 10–15 are reserved for typed conditions a caller can usefully
> branch on.

### 9.4 Error catalogue

`E_*` codes returned in the `code` field of structured errors.
Source: `internal/cli/errs/`, `internal/cli/inputx/`,
`internal/cli/helpers.go`, and per-command files.

| Code | Category | What it means |
|---|---|---|
| `E_VALIDATION` | validation_failed | Generic validation; check `message`. |
| `E_NETWORK` | network_error | Generic network; check `message`. |
| `E_AUTH` | auth_error | Generic auth; check `message`. |
| `E_CONFLICT` | conflict | Generic conflict; check `message`. |
| `E_NOT_FOUND` | not_found | Generic missing-thing; check `message`. |
| `E_TIMEOUT` | timeout | Generic deadline exceeded. |
| `E_INTERNAL` | internal_error | Bug in nostos. Re-run with `--verbose`. |
| `E_NODE_NAME_EMPTY` | validation_failed | Node name argument is empty. |
| `E_NODE_NAME_CONTROL` | validation_failed | Node name has ASCII control chars. |
| `E_NODE_NAME_FORMAT` | validation_failed | Node name doesn't match `^[a-zA-Z0-9][a-zA-Z0-9-]{0,62}$`. |
| `E_NODE_NAME_TOO_LONG` | validation_failed | Over 63 chars. |
| `E_NODE_NOT_FOUND` | not_found | Node name not in `config.yaml`. |
| `E_CONFIG_NOT_FOUND` | not_found | `config.yaml` not at expected path. |
| `E_CONFIG_PARSE` | validation_failed | YAML parse error. |
| `E_CONFIG_PATH_CONTROL` | validation_failed | `--config` path has control chars. |
| `E_CONFIG_PATH_TRAVERSAL` | validation_failed | `--config` path contains `..`. |
| `E_CONFIG_PATH_ABS` | validation_failed | `--config` could not be resolved to an absolute path. |
| `E_CONFIG_PATH_OUTSIDE` | validation_failed | `--config` resolves outside the home dir or repo root. |
| `E_OPREF_CONTROL` | validation_failed | `op://` ref has control chars. |
| `E_OPREF_QUERY` | validation_failed | `op://` ref has a query/fragment. |
| `E_OPREF_FORMAT` | validation_failed | `op://` ref doesn't match `^op://[\w-]+/[\w.-]+(/[\w.-]+){0,2}$`. |
| `E_FIELDS_CONTROL` | validation_failed | `--fields` mask has control chars. |
| `E_FIELDS_EMPTY` | validation_failed | `--fields` has an empty entry (e.g. `a,,b`). |
| `E_FIELDS_IDENT` | validation_failed | `--fields` entry isn't a valid identifier. |
| `E_FIELDS_UNKNOWN` | validation_failed | `--fields` entry isn't in this command's stdout schema. |
| `E_CONFIRM_REQUIRED` | conflict | Destructive command needs `--yes`. |
| `E_SCHEMA_METHOD` | not_found | `schema <method>` arg isn't in the schema tree. |

Plus the typed errors raised inside the tpi provider (translated
into the categories above):

- `ErrBMCUnreachable` → `E_NETWORK` / `E_BMC_UNREACHABLE`
- `ErrBMCAuth` → `E_AUTH` / `E_BMC_AUTH`
- `ErrBMCVersion` → `E_VALIDATION` / `E_BMC_VERSION`

### 9.5 MCP integration

`nostos mcp` runs a JSON-RPC 2.0 server on stdio, where every tool
is `nostos.<method-id>` and the input schema is derived from the
cobra flags + the schema registry. Tool calls return the same JSON
payload as `nostos <command> --output json`.

List tools:

```bash
echo '{"jsonrpc":"2.0","method":"tools/list","id":1}' | \
  ./.bin/nostos mcp | jq '.result.tools[].name'
```

Call a tool (status):

```bash
echo '{"jsonrpc":"2.0","method":"tools/call","id":2,
       "params":{"name":"nostos.status","arguments":{}}}' | \
  ./.bin/nostos mcp | jq '.result.content[0].json'
```

Wire into Claude Desktop / similar by registering the binary as an
MCP server in the client's config:

```json
{
  "mcpServers": {
    "nostos": {
      "command": "/Users/yuri/Workdir/Yuri/home-systems/.bin/nostos",
      "args": ["mcp"]
    }
  }
}
```

> **Why "one tool per cobra command"?** Single source of truth.
> Adding a flag to a cobra command propagates to MCP automatically.
> No second schema to keep in sync.

### 9.6 File layout

```
home-systems/
├── nostos/                          ← OPERATOR DATA (committed except state/)
│   ├── config.yaml                  ← cluster + node registry
│   ├── templates/                   ← per-node Talos machineconfig templates
│   │   ├── dell01.yaml
│   │   ├── tp1.yaml
│   │   └── tp4.yaml
│   └── state/                       ← cache; gitignored; rebuildable
│       ├── assets/                  ← Talos kernel/initramfs/iPXE per version+arch
│       ├── cache/                   ← image cache + digests.json (TOFU)
│       ├── configs/                 ← rendered <mac>.yaml files (SECRETS!)
│       ├── kubeconfig
│       ├── talosconfig
│       └── logs/
├── .submodules/nostos/              ← THE TOOL (git submodule; do not edit ad hoc)
│   ├── cmd/nostos/                  ← main package
│   ├── internal/cli/                ← cobra commands, jsonio, errs, schema, dryrun, inputx
│   ├── internal/cluster/            ← orchestrator (PXE+TPI lifecycle)
│   ├── internal/provisioner/        ← provisioner interface + pxe + tpi providers
│   ├── internal/dashboard/          ← Bubble Tea v2 model + actions + playbooks
│   ├── internal/secrets/            ← op / env / file / tailscale schemes
│   ├── internal/mcp/                ← JSON-RPC server
│   └── AGENTS.md                    ← invariants for AI/CLI callers
├── .bin/
│   └── nostos                       ← cached binary (rebuilt on demand)
├── docs/
│   ├── nostos-guide.md              ← THIS FILE
│   ├── talos.md                     ← Talos-specific notes
│   └── ...
└── ~/.cache/nostos/
    └── upstream-versions.json       ← 24h dashboard cache
```

### 9.7 Common one-liners

Show all unhealthy nodes:

```bash
./.bin/nostos status --output json | \
  jq '.nodes[] | select(.ping != "up" or .apid != "up")'
```

Find zombie devices on the LAN (in arp, not in config):

```bash
./.bin/nostos dashboard --once --output json | \
  jq '.discoveries[] | select(.bucket == "unknown")'
```

Test all secrets backends:

```bash
./.bin/nostos secrets test --output json | jq
```

Aggregate state at a glance:

```bash
./.bin/nostos dashboard --once --output json --no-upstream | \
  jq '{state: .aggregate_state, imperative: .imperative}'
```

Render every node (idempotent — safe to script):

```bash
for n in $(./.bin/nostos node list --output json | jq -r '.name'); do
  ./.bin/nostos render "$n"
done
```

Revoke every Tailscale auth-key (after a security incident):

```bash
./.bin/nostos secrets keys list --output json | jq -r '.id' | \
  xargs -I {} ./.bin/nostos secrets keys revoke {}
```

Dry-run a cleanup of zombies older than a week:

```bash
./.bin/nostos cluster cleanup --age-days 7 --dry-run --output json
```

Pretty-print the full CLI tree:

```bash
./.bin/nostos schema --all | \
  jq -r 'to_entries[] | "\(.key)\t\(.value.description)"' | column -t -s $'\t'
```

Dump a single command's contract for an LLM:

```bash
./.bin/nostos schema node.install --output json
```

Refresh kubeconfig (e.g. cert expired):

```bash
./.bin/nostos config refresh
./.bin/nostos kubeconfig dell01
```

---

## Do-not-commit list

The following must NEVER be committed to git:

- Anything under `nostos/state/configs/` (rendered machineconfigs
  with embedded secrets).
- `nostos/state/talosconfig`, `nostos/state/kubeconfig` (client
  certs).
- 1Password OAuth client IDs and secrets, Tailscale auth-keys, BMC
  passwords (paste into 1Password, reference via `op://`).
- Home-network IPs, MAC addresses, DHCP lease tables (low risk but
  policy here).

The `.gitignore` at the repo root covers `nostos/state/`. If you
add a new template under `templates/`, **never paste a literal
secret** — use `op://` refs and let `nostos render` resolve them
into `state/configs/` (gitignored).
