## Why

`nostos` is a new open-source tool that owns the entire bare-metal → joined-Talos-node flow for single-operator home labs. Today, every home-labber running Talos reinvents the same 4 shell scripts, hits the same 8 undocumented failure modes (DHCP races, expired admin certs, persisted STATE partitions, BIOS boot-order traps), and loses an evening per node. `nostos` consolidates that knowledge into one installable Python tool with a sane CLI, optional web UI, and pluggable secret backends.

## What Changes

- Create a standalone `uv`-managed Python package implementing the full bare-metal-to-cluster flow.
- Expose a `nostos` CLI with 12 commands (`init`, `node add|list|remove`, `build`, `render`, `serve`, `install`, `wipe`, `bootstrap`, `config refresh`, `status`, `web`).
- Ship a **secrets-backend adapter** layer: 1Password (`op://`) is the default; `sops`, `env`, and `file` backends are swappable by config without template edits.
- Ship a **cluster-control** layer: `talosctl bootstrap`, wait-for-ready, kubeconfig fetch, **admin-cert regeneration against the existing CA** (solves the expired-client-cert trap), one-shot wipe.
- Ship an **optional local web dashboard** (opt-in via `nostos web`, localhost-only) that can both view status AND trigger mutations (reinstall, wipe, bootstrap).
- Develop in-tree at `.submodules/nostos/` in this repo initially; extract to `github.com/yurifrl/nostos` as its own project when stable (that extraction is a future change, not this one).

This change scopes **only the tool**. The home-systems repo's adoption of the tool (moving `pxe/` → `nostos/`, wiring Taskfiles) is tracked separately in the `adopt-nostos` change, which depends on this one.

## Capabilities

### New Capabilities
- `pxe-provisioning`: Build Talos kernel/initramfs/iPXE assets, render per-MAC machineconfigs, serve them over HTTP/TFTP/DHCP so a powered-on node can PXE-boot and self-install.
- `node-registry`: Single source of truth for node identity (name, MAC, IP, role, install disk, template binding) with reachability and Talos-version probing.
- `secrets-backend`: Pluggable adapter that resolves `<scheme>://...` refs in templates. Default backend is 1Password; contract supports sops, env, file without template edits.
- `cluster-control`: Post-install operations — `talosctl bootstrap`, wait-for-ready, kubeconfig fetch, admin-cert regeneration against the existing CA, one-shot wipe.
- `nostos-cli`: The command-line surface: argument parsing, config-file discovery, output formatting, error handling.
- `nostos-web`: Optional localhost single-page UI showing node status, reachability, command cheat-sheets. **Supports mutations** (reinstall, wipe, bootstrap) — not just read-only.

### Modified Capabilities
(none — greenfield tool)

## Impact

- **Code added:** `.submodules/nostos/` containing a proper uv-managed Python package:
  ```
  .submodules/nostos/
  ├── pyproject.toml
  ├── src/nostos/
  │   ├── __init__.py
  │   ├── cli.py                ← click entrypoint (nostos-cli capability)
  │   ├── pxe.py                ← pxe-provisioning capability
  │   ├── registry.py           ← node-registry capability
  │   ├── secrets.py            ← secrets-backend capability
  │   ├── cluster.py            ← cluster-control capability
  │   └── web/                  ← nostos-web capability (FastAPI + static)
  ├── tests/
  └── README.md
  ```
- **Dependencies:** `uv`, Python 3.11+, `click`, `rich`, `questionary`, `httpx`, `pyyaml`. For web: `fastapi`, `uvicorn`, `jinja2`. Runtime externals: `dnsmasq` (brew), `talosctl`, `docker` (v0.1 iPXE build), 1Password CLI (`op`).
- **Config contract:** `nostos` reads a `config.yaml` passed via `--config` (or auto-discovered). No hardcoded paths.
- **Input data the tool consumes (not provided by this change):** the consumer repo's `config.yaml` and `templates/*.yaml`. See `adopt-nostos` for home-systems' specific contents.
- **Output data the tool writes:** everything under `<config>/state/` (kernel, initramfs, iPXE binary, rendered configs, talosconfig, kubeconfig, logs). Path is configurable; `state/` is the default relative to the config file.
- **Not impacted:** Talos itself, Kubernetes workloads, any consumer repo's existing structure. This change is tool-only.

## Core Design Properties

Two invariants that downstream artifacts (specs, design, tasks) must preserve:

1. **State directory is a cache, never primary state.** Everything `nostos` writes under `<state-dir>/` must be reproducible from three inputs: the consumer's `config.yaml`, the consumer's `templates/*.yaml`, and the selected secrets backend (1Password vault by default). `rm -rf <state-dir>/` is a supported recovery path. No unique state (keys generated once, logs, manual tweaks) ever lives there alone. One intentional exception: the admin client cert minted by `config refresh` is stored in `<state-dir>/talosconfig` and is per-device by design — a new laptop generates its own cert against the same CA; this IS ephemeral, just scoped to the machine running `nostos`.
2. **The selected secrets backend is the only source of primary state.** Machine CA, cluster CA, service-account keys, cluster token, `cluster.id`, `cluster.secret`, extension secrets (TS_AUTHKEY) — all live in the backend (default: 1Password vault). The consumer's templates reference them by URI; `nostos` resolves at render time. No secret material is ever committed, written unencrypted to disk outside `<state-dir>/`, or logged.

## What This Is Not (Non-Goals)

Explicit to keep scope honest:

- **Not Sidero Omni.** Omni is a hosted SaaS. `nostos` is local-only, offline-capable, zero phone-home.
- **Not Matchbox / Tinkerbell / MaaS.** Those are datacenter-scale provisioners with databases and RBAC. `nostos` is single-operator, single-laptop.
- **Not a Kubernetes bootstrapper.** Talos already bootstraps Kubernetes once configured. `nostos` just gets Talos onto metal and runs `talosctl bootstrap`.
- **Not a Talos replacement or fork.** `nostos` is a thin orchestrator around the existing Talos factory, `talosctl`, and iPXE projects.
- **Not opinionated about networking.** If the consumer's LAN already runs DHCP (consumer router, etc.), `nostos`'s dnsmasq co-exists via PXE vendor-class filtering — it never fights the main DHCP server for non-PXE traffic.
- **Not a multi-cluster orchestrator.** One `config.yaml` = one cluster. Managing two clusters = two config files, run `nostos` twice.
- **Not a monitoring or observability tool.** `nostos status` reports node reachability + Talos version; nothing more. Prometheus/Grafana etc. are the consumer's problem.
