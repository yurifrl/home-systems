## Context

This is the consumer-side change for the home-systems repo adopting the `nostos` tool (developed under `nostos-v01`). The repo currently provisions Talos via a tangle of `pxe/scripts/*.sh` and `taskfiles/*.yml`. This change swaps that for canonical nostos consumption.

Constraints:
- Must not touch any running Talos node. This is purely a repo-structure change.
- Existing `talos/` and `k8s/` directories stay intact.
- Session-created `talos/templates/dell01.yaml` moves to `nostos/templates/` because the nostos tool expects it there.
- Must happen *after* `nostos-v01` ships enough to produce the same behavior as the current scripts.

## Goals / Non-Goals

**Goals:**
- Single, canonical directory layout for all nostos-related data in this repo.
- Zero shell scripts under `pxe/` after this change.
- Keep the Taskfile wrapper pattern (operator prefers task runner aliases).
- Retain the ability to provision a new node without re-reading this session's conversation log.

**Non-Goals:**
- Refactoring `talos/nodes/*.yaml` (workers' existing configs). Those stay.
- Changing Kubernetes / ArgoCD manifests. Out of scope.
- Moving secrets between vaults. 1Password usage is unchanged.
- Automating worker reinstalls. That's a future operator task.

## Decisions

### D1. Directory layout

```
home-systems/
├── .submodules/nostos/           ← tool (from nostos-v01)
├── nostos/                       ← this home lab's data
│   ├── config.yaml               ← node registry, cluster, secrets-backend
│   ├── templates/                ← machineconfigs
│   │   └── dell01.yaml           ← moved from talos/templates/
│   └── state/                    ← gitignored cache
├── talos/                        ← unchanged
├── k8s/                          ← unchanged
├── Taskfile.yml                  ← updated: include nostos.yml
└── taskfiles/
    ├── nostos.yml                ← new: thin wrappers
    └── talos.yml                 ← pruned: pxe entries removed
```

Rationale: user explicitly chose "all nostos stuff in the `nostos/` directory" — templates, config, and state co-located. The tool stays under `.submodules/` so it's visually distinct from user data.

### D2. Submodule now or placeholder?

The nostos tool is developed inside this repo for v0.1. We have two options:

(a) `.submodules/nostos/` as a regular directory committed to this repo.
(b) `.submodules/nostos/` as a proper git submodule pointing at a separate repo (`github.com/yurifrl/nostos`).

**Choice: (a) for v0.1.** Rationale: iterating on the tool is faster when it's in the same working tree; we don't want to context-switch across two repos during initial development. Convert to (b) as part of an extraction change once v0.2 is stable.

Tradeoff: merge history between "tool code" and "home lab config" is intermingled. Not ideal, but cheap to fix at extraction time via `git filter-repo`.

### D3. Taskfile wrapper strategy

Operator prefers `task <thing>` over `nostos <thing>`. Keep that muscle memory. The wrappers are one-liners shelling out via uv:

```yaml
# taskfiles/nostos.yml
version: '3'
tasks:
  build:
    desc: "Download Talos assets + build iPXE binary"
    cmds:
      - go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml build
  render:
    desc: "Render machineconfig for a node. Usage: task nostos:render NODE=dell01"
    requires:
      vars: [NODE]
    cmds:
      - go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml render "{{.NODE}}"
  up:
    desc: "Start PXE server (foreground, Ctrl+C to stop)"
    cmds:
      - go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml serve
  bootstrap:
    desc: "Bootstrap etcd on first controlplane node. Usage: task nostos:bootstrap NODE=dell01"
    requires:
      vars: [NODE]
    cmds:
      - go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml bootstrap "{{.NODE}}"
  status:
    cmds:
      - go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml status
```

The `--config nostos/config.yaml` flag is passed explicitly so the wrapper works regardless of operator's cwd.

### D4. Deletion order

Taskfile references are updated *before* files are deleted, to keep the repo in a consistently valid state at every commit boundary. Order:

1. Add `.submodules/nostos/` (depends on nostos-v01 having shipped).
2. Create `nostos/config.yaml` + `nostos/templates/dell01.yaml` (mv from `talos/templates/`).
3. Add `taskfiles/nostos.yml` + wire into root `Taskfile.yml`.
4. Update `.gitignore` (add `nostos/state/`, `.submodules/nostos/.nostos-state/`).
5. Prune pxe entries from `taskfiles/talos.yml`.
6. Delete `taskfiles/pxe.yml` and root `Taskfile.yml` include.
7. Delete `pxe/` directory.
8. Update `CLAUDE.md` provisioning-workflow section.

### D5. Compat window

None. Operator chose "delete immediately" for the legacy scripts. There's no parallel-operation period — `task pxe:up` and `task nostos:up` coexisting would be confusing.

## Risks / Trade-offs

- **[Risk] nostos-v01 isn't shipped when adopt-nostos starts** → Mitigation: `adopt-nostos` has an explicit dependency on `nostos-v01` in its proposal. Don't apply this change until nostos-v01 tasks complete.
- **[Risk] Moving `talos/templates/dell01.yaml` breaks in-flight PXE boot** → Mitigation: verify Dell is up + `kubectl get nodes` shows Ready before running adoption tasks. Dell reboot during this change is not cluster-affecting (Talos survives reboot via persisted config).
- **[Risk] Operator has muscle memory for old task names** → Mitigation: document task-name migration in `CLAUDE.md` (`task pxe:up` → `task nostos:up`); keep names as similar as possible.
- **[Trade-off] Intermingled git history for tool + data** → Acceptable cost for v0.1 iteration speed; planned cleanup at extraction time.

## Migration Plan

1. Verify prerequisites: `nostos-v01` shipped, `go run ./.submodules/nostos/cmd/nostos --version` works, Dell controlplane is healthy.
2. Create `nostos/config.yaml` populated from existing `pxe/nodes.yaml` data.
3. `git mv talos/templates/dell01.yaml nostos/templates/dell01.yaml`.
4. Write `taskfiles/nostos.yml`.
5. Update root `Taskfile.yml` to include `nostos.yml`.
6. Update `.gitignore`.
7. Edit `taskfiles/talos.yml` to remove `apply:dell01`, `op:inject` dell01 line, any dell01 references only used for PXE.
8. Delete `taskfiles/pxe.yml` and the include in root `Taskfile.yml`.
9. Delete `pxe/` directory.
10. Update `CLAUDE.md`: replace RPi-era provisioning section with nostos quickstart.
11. Smoke test: `task nostos:status` shows dell01 Ready.

Rollback: `git revert` the commits. State cache in `nostos/state/` is regenerable.

## Open Questions

- **Q1.** Keep `docs/pxe-boot.md` deleted (done earlier this session), or write a new `docs/nostos.md` as part of this change? Lean toward: add it as a Task. Consumer docs matter.
- **Q2.** Should `taskfiles/talostroobleshooting.yml` and `taskfiles/talosupgrade-1.10.3.yml` be pruned too? They reference the old RPi-era layout but are orthogonal to nostos adoption. Defer to a separate cleanup change.
