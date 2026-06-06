# nostos

> νόστος — Homer's word for homecoming. The Odyssey is one long nostos. So is every bare-metal install.

A single Go CLI (with a Charm v2 TUI) that owns the bare-metal-to-cluster flow for single-operator [Talos Linux](https://www.talos.dev/) home labs.

## Status

**v0.1 — alpha.** Developed in-tree at `.submodules/nostos/` inside the
[yurifrl/home-systems](https://github.com/yurifrl/home-systems) repo. Will be
extracted to its own repo (`github.com/yurifrl/nostos`) at v0.2.

A prior Python prototype exists on the `python` branch for historical reference.

## Invocation

Always via `go run`. No install, no build step, no binary to ship:

```bash
# from the home-systems repo root:
go run ./.submodules/nostos/cmd/nostos --version
go run ./.submodules/nostos/cmd/nostos --help
```

The repo root has a `go.work` pointing at `.submodules/nostos/` so Go resolves
the module correctly from any working directory in the repo.

## Quickstart (once v0.1 ships)

```bash
# in a new directory
go run ./.submodules/nostos/cmd/nostos init
go run ./.submodules/nostos/cmd/nostos node add dell01
go run ./.submodules/nostos/cmd/nostos build
go run ./.submodules/nostos/cmd/nostos up dell01     # end-to-end install
```

## `flash`: zero-touch remote nodes

`nostos flash <node>` produces a flashable Talos disk image for a node that
lives at a remote site (different LAN, joined via Tailscale). The image
carries the rendered machineconfig as a sidecar; the operator at the remote
site plugs in power + ethernet, you `talosctl apply-config --insecure` once,
and the node joins the cluster via the tailnet.

```bash
# preview the plan
nostos flash rpi01 --out /tmp/rpi01.raw.xz --compress --dry-run

# build the image (downloads Talos raw image, mints Tailscale key, renders config)
nostos flash rpi01 --out /tmp/rpi01.raw.xz --compress

# or flash directly to a connected SD/SSD (asks for confirmation):
nostos flash rpi01 --device /dev/disk10 --yes
```

For RPi nodes (`overlay: rpi_generic`), `flash` also emits a small
`<name>-eeprom/` directory with `start4.elf`, `fixup4.dat`, `recovery.bin`,
`pieeprom.bin`, and `boot.conf` (BOOT_ORDER=0xf21). Copy those onto a FAT32
SD card, boot the Pi 4 once to flash the EEPROM, then swap to the Talos
disk for normal boot.

### Cross-subnet routing

All node templates ship with `TS_EXTRA_ARGS=--accept-routes` by default so
every node accepts subnet routes advertised by its peers. Without this,
etcd peer-to-peer traffic across LANs (e.g. an offsite Pi reaching a
home-LAN controlplane) silently fails. `nostos render` emits a stderr
warning if it produces a config that has Tailscale but no `--accept-routes`.

## Multi-arch build

`nostos build` (no flags) iterates every node in `config.yaml`, collects
unique `(schematic_id, arch)` pairs, and downloads kernel + initramfs for
each. RPi nodes (`overlay: rpi_generic`) also pull `start4.elf` and
`fixup4.dat` from the official `raspberrypi/firmware` repository. Pass
`--arch <amd64|arm64>` or `--legacy` to fall back to the v0.1 single-arch
path.

## Requirements

- Go 1.22+
- [talosctl](https://www.talos.dev/latest/talos-guides/install/talosctl/)
- [dnsmasq](https://dnsmasq.org/) (macOS: `brew install dnsmasq`)
- Docker (first `build` only; v0.2 will ship pre-built iPXE binaries)
- One of: [1Password CLI `op`](https://developer.1password.com/docs/cli/), sops, env vars, plain files

## Tailscale auth keys

`tailscale://authkey` is a dynamic secret reference. Every
`nostos render <node>` mints a fresh Tailscale auth key and writes the real
`tskey-auth-...` value only into the rendered machineconfig.

See [`docs/tailscale-authkey-refresh.md`](../../docs/tailscale-authkey-refresh.md)
for the focused behavior doc.

## Stack

- Go 1.22+
- [cobra](https://github.com/spf13/cobra) — subcommand routing
- Charm v2:
  - [bubbletea](https://charm.land/bubbletea) — TUI runtime
  - [lipgloss](https://charm.land/lipgloss) — styling
  - [bubbles](https://charm.land/bubbles) — reusable components
  - [huh](https://charm.land/huh) — interactive forms
- stdlib `crypto/ed25519`, `crypto/x509` — native admin-cert regen (no `talosctl gen` shell-out)

## Non-goals

- Not [Sidero Omni](https://www.siderolabs.com/platform/sidero-omni/). No SaaS, zero phone-home.
- Not [Matchbox](https://github.com/poseidon/matchbox) / [Tinkerbell](https://tinkerbell.org/). Single-operator, not datacenter.
- Not a Talos or Kubernetes replacement. Thin orchestrator around existing tools.
- No web UI in v0.1. TUI only. Web UI is a v0.3 conversation.

## License

MIT.
