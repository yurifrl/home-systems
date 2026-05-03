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

## Requirements

- Go 1.22+
- [talosctl](https://www.talos.dev/latest/talos-guides/install/talosctl/)
- [dnsmasq](https://dnsmasq.org/) (macOS: `brew install dnsmasq`)
- Docker (first `build` only; v0.2 will ship pre-built iPXE binaries)
- One of: [1Password CLI `op`](https://developer.1password.com/docs/cli/), sops, env vars, plain files

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
