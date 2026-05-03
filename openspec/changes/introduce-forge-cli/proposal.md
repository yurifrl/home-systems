## Why

Onboarding a bare-metal Talos node today requires running four disconnected shell scripts, four overlapping Taskfiles, a manual `op inject` flow, and undocumented recovery steps (expired admin cert, BIOS boot order, `wipe=system` loop, Deco DHCP race). Bringing up the Dell as controlplane took an 8-hour evening and the knowledge lives only in a conversation log. A single-tool, self-contained workflow turns this into a reproducible 5-command operation and makes future nodes (workers rebuilds, new hardware) a 15-minute job instead of an evening.

## What Changes

- Introduce `forge`, a `uv`-based Python CLI that owns the entire bare-metal → joined-node flow.
- **BREAKING** `pxe/scripts/*.sh` and `taskfiles/pxe.yml` become thin compatibility wrappers in v0.1 and are removed in v0.3.
- **BREAKING** Node metadata moves from `pxe/nodes.yaml` + `talos/templates/*.yaml` (two places) to a single `forge.yaml` plus per-node templates under `templates/`.
- Add a **secrets adapter** layer: 1Password (`op://`) stays as default, but sops/env/file backends become swappable via `forge.yaml`.
- Add **recovery operations** that today are tribal knowledge: admin-cert regeneration against the existing CA, one-shot wipe-and-reinstall, BIOS boot-order cheat-sheet per node.
- Add **local web dashboard** (opt-in, `forge web`, localhost-only) for status and cheat-sheets.
- Retire `task talos:op:*`, `task talos:apply*`, `task pxe:*` in favor of `forge` commands once parity is verified.

## Capabilities

### New Capabilities
- `pxe-provisioning`: Build Talos kernel/initramfs/iPXE assets, render per-MAC machineconfigs, serve them over HTTP/TFTP/DHCP so a powered-on node can PXE-boot and self-install.
- `node-registry`: Single source of truth for node identity (name, MAC, IP, role, install disk, template binding) with reachability and Talos-version probing.
- `secrets-backend`: Pluggable adapter that resolves `<scheme>://...` refs in templates. Default backend is 1Password; contract supports sops, env, file without template edits.
- `cluster-control`: Post-install operations — `talosctl bootstrap`, wait-for-ready, kubeconfig fetch, admin-cert regeneration against the existing CA, one-shot wipe.
- `forge-cli`: The command-line surface: `init`, `node add|list|remove`, `build`, `render`, `serve`, `install`, `wipe`, `bootstrap`, `config refresh`, `status`, `web`.
- `forge-web`: Optional localhost single-page UI showing node status, reachability, and copy-to-clipboard command cheat-sheets. Read-only in v1; mutations require CLI.

### Modified Capabilities
(none — this is a greenfield tool; existing files remain until retirement in a later change)

## Impact

- **Code added:** new `forge/` directory (single-file `uv run --script` in v0.1; proper package once the web UI lands).
- **Code retired (staged):** `pxe/scripts/*.sh`, `taskfiles/pxe.yml`, and the `op:*` + `apply*` entries of `taskfiles/talos.yml`.
- **Config moves:** `pxe/nodes.yaml` → `forge.yaml`. `talos/templates/*.yaml` stays in place (forge reads from there).
- **State dir:** new `.forge/` (gitignored) for built assets, rendered secret-bearing configs, cached kubeconfig, logs. Replaces `pxe/assets/` and `pxe/ipxe-src/`.
- **Dependencies:** `uv`, Python 3.11+, `click`, `rich`, `questionary`, `httpx`, `pyyaml`, `jinja2` (for template rendering if we go beyond pure text substitution). Docker still required for iPXE build in v0.1; v0.2 may ship a pre-built iPXE to drop this.
- **External systems touched:** 1Password CLI (`op`), `dnsmasq` (homebrew), `talosctl`. All pre-existing in the user's environment.
- **Docs:** new `docs/forge.md` replaces the stale `docs/pxe-boot.md` (already removed in prior cleanup).
- **Not impacted:** Talos cluster itself, running workers, ArgoCD apps, Helm charts under `k8s/`. This change is purely about the provisioning workflow.
