## Why

A Python prototype of `nostos` shipped in this repo and got archived to the `python` branch once it hit two structural issues: rich-Live rendering glitched over SSH/low-fi terminals, and the operator's mental model ("I want one binary, no venv, no uv tool install, no pip") didn't fit. Rewriting in **Go** using the Charm libraries eliminates both problems: single-binary execution via `go run`, first-class TUI via Bubble Tea instead of rich-Live, and a distribution story that doesn't require a Python runtime on the operator's laptop.

The product vision is unchanged — one tool that owns bare-metal Talos provisioning for single-operator home labs, with pluggable secrets backends, one-shot wipe flow, admin-cert regeneration, and end-to-end install orchestration.

## What Changes

- Implement `nostos` in **Go ≥1.22** under `.submodules/nostos/`, invoked exclusively via `go run` (no `go install`, no pre-built binary, no `go build` step in the operator workflow).
- Use the Charm stack **v2** (released Feb 2026): [`bubbletea/v2`](https://charm.land/bubbletea), [`lipgloss/v2`](https://charm.land/lipgloss), [`bubbles/v2`](https://charm.land/bubbles), [`huh/v2`](https://charm.land/huh). Import paths use `charm.land/...`, not `github.com/charmbracelet/...` (v1 lives there).
- CLI surface wired with [`cobra`](https://github.com/spf13/cobra) — cobra owns subcommand routing, Bubble Tea owns interactive views (`nostos up`, `nostos status`, `nostos node add`).
- Drop the separate FastAPI web UI. The TUI replaces it. A web UI can be reintroduced in a later change (v0.3) if operator demand exists; for v0.1 the TUI is the only UI.
- Preserve every hard-won behavior from the Python prototype: retry-loop iPXE embed, `${next-server}` + `${filename}` runtime resolution, one-shot wipe flag, Deco-DHCP race handling, admin-cert regeneration against the existing CA, byte-identical render output vs. `op inject`.
- Taskfile wrappers change: `task nostos:<cmd>` invokes `go run ./.submodules/nostos/cmd/nostos <cmd>` with the consumer `--config nostos/config.yaml`.

## Capabilities

### New Capabilities
- `pxe-provisioning`: Build Talos kernel/initramfs/iPXE assets, render per-MAC machineconfigs, serve them over HTTP/TFTP/DHCP so a powered-on node can PXE-boot and self-install.
- `node-registry`: Single source of truth for node identity (name, MAC, IP, role, install disk, template binding) with reachability and Talos-version probing.
- `secrets-backend`: Pluggable interface that resolves `<scheme>://...` refs in templates. Default backend is 1Password CLI; contract supports sops, env, file without template edits.
- `cluster-control`: Post-install operations — `talosctl bootstrap`, wait-for-ready, kubeconfig fetch, **admin-cert regeneration against the existing CA using `crypto/x509` natively** (no `talosctl gen` shell-out), one-shot wipe.
- `nostos-cli`: The command surface — `cobra`-based subcommands (`init`, `node add|list|remove`, `build`, `render`, `serve`, `up`, `wipe`, `bootstrap`, `config refresh`, `status`, `kubeconfig`, `nuke`). Global flags: `--config`, `--output text|json`, `--verbose`.
- `nostos-tui`: Interactive Bubble Tea views — install progress (`nostos up`), live cluster dashboard (`nostos status --watch`), node-add wizard (`nostos node add`). Replaces rich-Live + FastAPI web UI.

### Modified Capabilities
(none — greenfield; the archived Python implementation lived on a branch, not in main specs)

## Impact

- **Code added:** `.submodules/nostos/` containing a Go module:
  ```
  .submodules/nostos/
  ├── go.mod
  ├── go.sum
  ├── cmd/nostos/main.go          ← cobra root command
  ├── internal/
  │   ├── cli/                    ← cobra subcommand handlers
  │   ├── config/                 ← yaml.v3 parsing + validation
  │   ├── secrets/                ← op / sops / env / file backends
  │   ├── pxe/                    ← build + serve
  │   ├── cluster/                ← bootstrap / cert / status / wipe
  │   ├── registry/               ← node ops
  │   └── tui/                    ← Bubble Tea models per view
  ├── testdata/
  └── README.md
  ```
- **Dependencies** (Go modules, exact versions pinned in `go.mod`):
  - `github.com/spf13/cobra`
  - `charm.land/bubbletea/v2`
  - `charm.land/lipgloss/v2`
  - `charm.land/bubbles/v2`
  - `charm.land/huh/v2`
  - `gopkg.in/yaml.v3`
  - `github.com/go-playground/validator/v10`
  Standard library otherwise (`crypto/ed25519`, `crypto/x509`, `encoding/base64`, `net/http`, `os/exec`, `encoding/json`).
- **Runtime externals** (unchanged from Python version): `dnsmasq` (brew), `talosctl`, `docker` (v0.1 iPXE build), 1Password CLI (`op`). Go itself is a new runtime requirement (≥1.22); operator already has it per standard macOS dev environment.
- **Invocation contract:** every operator command is `go run ./.submodules/nostos/cmd/nostos ...`. Go's build cache makes subsequent invocations near-instant (sub-100ms startup after first run). Never `go install`. Never ship compiled binaries in v0.1.
- **Config contract:** `config.yaml` schema unchanged from the archived Python prototype — cluster + secrets + nodes map. Consumers' `nostos/config.yaml` and `nostos/templates/*.yaml` remain compatible.
- **Output contract:** every list/query command supports `--output json` for machine consumers; `text` (default) uses Lipgloss-rendered tables.
- **Not impacted:** Talos itself, Kubernetes workloads, the consumer's non-nostos repo structure, the archived Python branch (it stays at `python` as reference).

## Core Design Properties

Preserved from the archived prototype; must continue to hold for this rewrite:

1. **State directory is a cache, never primary state.** Everything `nostos` writes under `<state-dir>/` is reproducible from three inputs: `config.yaml`, `templates/*.yaml`, and the selected secrets backend. `rm -rf <state-dir>/` is a supported recovery path. One intentional exception: the per-device admin client cert in `<state-dir>/talosconfig` — regenerated on demand, scoped to the laptop running `nostos`.
2. **The selected secrets backend is the only source of primary state.** No secret material is ever committed, written unencrypted to disk outside `<state-dir>/`, or logged. Error messages reference URIs, never resolved values.
3. **The tool runs without compilation or installation.** `go run ./...` is the only verb an operator needs to know. This forces the codebase to stay small enough that `go run` from cold cache is still acceptable (target: <3s cold, <200ms warm).

## What This Is Not (Non-Goals)

- **Not Sidero Omni.** Local-only, offline-capable, zero phone-home.
- **Not Matchbox / Tinkerbell / MaaS.** Single-operator, single-laptop.
- **Not a Kubernetes bootstrapper.** Talos already bootstraps Kubernetes once configured; `nostos` stops at `talosctl bootstrap`.
- **Not a Talos replacement or fork.** Thin orchestrator around Talos factory, `talosctl`, iPXE.
- **Not a web dashboard in v0.1.** TUI only. Web UI is a v0.3 conversation.
- **Not opinionated about networking.** If the consumer's LAN already runs DHCP (consumer router, etc.), `nostos`'s dnsmasq co-exists via PXE vendor-class filtering — never fights the main DHCP server for non-PXE traffic.
- **Not a multi-cluster orchestrator.** One `config.yaml` = one cluster.
- **Not a monitoring tool.** `nostos status` reports reachability + Talos version; nothing more.
