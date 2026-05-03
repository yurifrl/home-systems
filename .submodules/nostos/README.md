# nostos

> νόστος — Homer's word for homecoming. The Odyssey is one long nostos. So is every bare-metal install.

A single CLI (and optional web UI) that owns the bare-metal-to-cluster flow for single-operator [Talos Linux](https://www.talos.dev/) home labs.

## Status

**v0.1 — alpha.** Actively developed inside [yurifrl/home-systems](https://github.com/yurifrl/home-systems) under `.submodules/nostos/`. Will be extracted to its own repo (`github.com/yurifrl/nostos`) at v0.2.

## What it does

Replaces the usual tangle of PXE shell scripts and taskfiles with one command:

```bash
nostos node add dell01          # interactive: MAC, IP, role, disk
nostos build                    # download Talos + build iPXE, once
nostos render dell01            # op-inject secrets → per-MAC config
nostos serve                    # start PXE (dnsmasq + HTTP)
# → power on the Dell, walk away
nostos bootstrap dell01         # talosctl bootstrap + wait-for-ready
```

## Install

```bash
# editable install from a consumer repo
uv tool install --editable .submodules/nostos

# or with pipx
pipx install --editable .submodules/nostos
```

Requires:
- Python 3.11+
- [uv](https://docs.astral.sh/uv/) or [pipx](https://pipx.pypa.io/)
- [talosctl](https://www.talos.dev/latest/talos-guides/install/talosctl/)
- [dnsmasq](https://dnsmasq.org/) (homebrew on macOS)
- Docker (for v0.1 iPXE build; dropped in v0.2)
- One of: [1Password CLI `op`](https://developer.1password.com/docs/cli/) (default), sops, or plain env/file

## Quickstart

```bash
# in a new directory
nostos init                      # scaffolds config.yaml + templates/ + state/
nostos node add dell01           # wizard
nostos build
nostos render dell01
sudo -v && nostos serve          # needs sudo for dnsmasq
# (power on the node, wait for PXE install)
nostos bootstrap dell01          # first controlplane only
nostos status                    # confirm Ready
```

## Non-goals

- Not [Sidero Omni](https://www.siderolabs.com/platform/sidero-omni/). No SaaS, zero phone-home.
- Not [Matchbox](https://github.com/poseidon/matchbox) / [Tinkerbell](https://tinkerbell.org/). Single-operator, not datacenter.
- Not a Talos or Kubernetes replacement. Thin orchestrator around existing tools.

## License

MIT.
