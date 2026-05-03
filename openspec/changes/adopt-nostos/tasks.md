## 1. Prerequisites

- [ ] 1.1 Verify `nostos-v01` is shipped: `.submodules/nostos/` exists and `uv run --project .submodules/nostos nostos --version` prints `0.1.0`
- [ ] 1.2 Verify Dell controlplane is healthy: `kubectl get nodes` shows `dell01` `Ready`
- [ ] 1.3 Take a reference inventory: `kubectl get nodes,pods -A > /tmp/pre-adopt.txt` (for post-change diff)

## 2. Create nostos/ data directory

- [ ] 2.1 Create `nostos/` at repo root
- [ ] 2.2 Write `nostos/config.yaml` populated from `pxe/nodes.yaml` data (dell01 entry), add `cluster:` and `secrets:` stanzas per `nostos/config.yaml` schema
- [ ] 2.3 `git mv talos/templates/dell01.yaml nostos/templates/dell01.yaml`
- [ ] 2.4 Create `nostos/README.md` with a 2-paragraph overview of this home lab's node layout and links to `.submodules/nostos/README.md` for tool docs

## 3. Wire the Taskfile wrappers

- [ ] 3.1 Create `taskfiles/nostos.yml` with `build`, `render`, `up`, `down`, `status`, `bootstrap`, `wipe`, `config:refresh` tasks (each calling `uv run --project .submodules/nostos nostos --config nostos/config.yaml <subcommand>`)
- [ ] 3.2 Add `includes: nostos: ./taskfiles/nostos.yml` to root `Taskfile.yml`
- [ ] 3.3 Smoke test: `task nostos:status` executes without Python import or config-parse errors

## 4. Update .gitignore

- [ ] 4.1 Add `nostos/state/` to `.gitignore`
- [ ] 4.2 Remove `pxe/assets/` and `pxe/ipxe-src/` from `.gitignore` (no longer relevant)
- [ ] 4.3 Verify `git status` reports no unexpected files after rebuilding assets via `task nostos:build`

## 5. Prune talos.yml

- [ ] 5.1 Remove `apply:dell01` task from `taskfiles/talos.yml`
- [ ] 5.2 Remove the dell01 `op inject` line from `op:inject` task (dell01 is now a nostos concern)
- [ ] 5.3 Remove `.200` (Dell PXE-transient IP) from the `dashboard` task's node list
- [ ] 5.4 Verify `task talos:op:inject` still works for remaining workers (tp1, tp4, vm-pc01)

## 6. Delete legacy pxe/

- [ ] 6.1 Remove the `pxe:` include from root `Taskfile.yml`
- [ ] 6.2 Delete `taskfiles/pxe.yml`
- [ ] 6.3 `git rm -r pxe/` (scripts, nodes.yaml, README.md, schematic-amd64.yaml)
- [ ] 6.4 Verify `task --list` no longer shows any `pxe:` entries

## 7. Update CLAUDE.md

- [ ] 7.1 Replace the "Node-Specific Details → Control Plane" section to describe dell01 (not RPi)
- [ ] 7.2 Replace any `task pxe:*` references with `task nostos:*` equivalents
- [ ] 7.3 Add a new "Provisioning a new bare-metal node" paragraph pointing at `.submodules/nostos/README.md` and listing the 5-command happy path
- [ ] 7.4 Remove the "Common commands → Talos" references that still assume the pre-migration RPi

## 8. End-to-end verification

- [ ] 8.1 Run `task nostos:build` and confirm `nostos/state/assets/` is populated
- [ ] 8.2 Run `task nostos:render NODE=dell01` and confirm `nostos/state/configs/d0-94-66-d9-eb-a5.yaml` is produced with resolved secrets
- [ ] 8.3 Compare rendered output byte-for-byte with the last known-good `pxe/assets/configs/d0-94-66-d9-eb-a5.yaml` (stashed in `/tmp/home-systems-pxe-cleanup-*`) to confirm zero semantic drift
- [ ] 8.4 Run `task nostos:status` and confirm dell01 shows `Ready` + correct Talos version
- [ ] 8.5 Post-change inventory: `kubectl get nodes,pods -A > /tmp/post-adopt.txt`; `diff /tmp/pre-adopt.txt /tmp/post-adopt.txt` should show no node-level changes (pod names may rotate via controllers — that's fine)

## 9. Commit

- [ ] 9.1 Review `git status`; confirm expected changes only (nostos/ added, pxe/ deleted, taskfiles updated, CLAUDE.md updated, .gitignore updated, talos/templates/dell01.yaml moved)
- [ ] 9.2 Commit in logical batches: (a) add nostos/ + .submodules/nostos/, (b) add Taskfile wrappers, (c) delete pxe/ + prune talos.yml, (d) update CLAUDE.md
- [ ] 9.3 Final smoke: from a fresh shell, `task nostos:status` works end-to-end
