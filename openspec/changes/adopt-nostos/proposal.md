## Why

`home-systems` currently provisions bare-metal Talos nodes through a tangle of four shell scripts and four Taskfiles. The `nostos` tool (developed under `.submodules/nostos/`, intended to graduate to its own open-source repo) consolidates that workflow. This change is the home-systems-side adoption: restructure this repo so it consumes `nostos` as a vendored tool, move data into a canonical location, and delete the legacy PXE scaffolding.

## What Changes

- Add `.submodules/nostos/` containing the nostos tool source (initially developed in-tree; becomes a real git submodule once extracted to `github.com/yurifrl/nostos`).
- Add `nostos/` directory at repo root containing this home lab's data:
  - `nostos/config.yaml` — node registry, cluster settings, secrets-backend selection (replaces `pxe/nodes.yaml`).
  - `nostos/templates/*.yaml` — machineconfigs (moved from `talos/templates/`).
  - `nostos/state/` — gitignored cache for built assets, rendered configs, talosconfig, logs (replaces `pxe/assets/` + `pxe/ipxe-src/`).
- **BREAKING** Delete `pxe/` directory entirely (scripts, README, schematic, nodes.yaml).
- **BREAKING** Delete `taskfiles/pxe.yml` and all `task pxe:*` commands.
- **BREAKING** Remove pxe-related entries from `taskfiles/talos.yml` (`op:inject` now scoped to talos nodes only; `apply:dell01` removed).
- Add `taskfiles/nostos.yml` with thin wrappers: `task nostos:build`, `task nostos:render`, `task nostos:up`, `task nostos:bootstrap`, `task nostos:status`. Each wrapper shells out to `uv run --project .submodules/nostos nostos <cmd> --config nostos/config.yaml`.
- Update `.gitignore`: add `nostos/state/`, remove `pxe/assets/` and `pxe/ipxe-src/` entries.
- Update `CLAUDE.md` with new provisioning-workflow guidance (replaces stale RPi-era references).

## Capabilities

### New Capabilities
- `nostos-integration`: Defines the home-systems-side contract for using nostos — directory layout, Taskfile wrapper conventions, submodule vendoring approach, and migration path from the legacy `pxe/` scripts.

### Modified Capabilities
(none — this repo has no existing OpenSpec specs to modify)

## Impact

- **Files added:** `.submodules/nostos/` (placeholder until submodule), `nostos/config.yaml`, `nostos/templates/dell01.yaml` (moved), `taskfiles/nostos.yml`.
- **Files removed:** `pxe/` (whole directory), `taskfiles/pxe.yml`.
- **Files modified:** `Taskfile.yml` (includes `nostos.yml`, drops `pxe.yml`), `taskfiles/talos.yml` (drops pxe entries), `.gitignore`, `CLAUDE.md`.
- **Depends on:** `nostos-v01` change (the tool must exist before this repo can adopt it). Sequencing matters: `nostos-v01` tasks run first, then `adopt-nostos` tasks.
- **Cluster impact:** zero. This is a repo-structure change. The running Talos cluster is not touched.
- **Docs:** new `docs/nostos.md` or `nostos/README.md` explaining this home lab's specific node registry and operational cheat-sheets.

## Core Design Properties

Inherited from `nostos-v01` and enforced at this repo's level:

1. **`nostos/state/` is a cache, never primary state.** `rm -rf nostos/state/` is a supported recovery path. Anything in it can be rebuilt from `nostos/config.yaml` + `nostos/templates/` + 1Password.
2. **1Password is the only primary state source.** `nostos/config.yaml` and templates reference it; no secret material ever lands in the repo.
3. **The tool is replaceable.** Because `nostos` lives in `.submodules/` and this repo only interacts with it through the CLI + `nostos/config.yaml`, swapping to a different provisioner later is a self-contained change, not a rewrite.
