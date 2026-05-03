## 1. Scaffold the Go module

- [x] 1.1 Create `.submodules/nostos/go.mod` with `module github.com/yurifrl/nostos` and `go 1.22`
- [x] 1.2 Create `.submodules/nostos/cmd/nostos/main.go` with a cobra root command printing version
- [x] 1.3 Add `.submodules/nostos/.gitignore` for `bin/`, `dist/`, local editor files
- [x] 1.4 Add runtime deps to `go.mod` (Charm v2 uses `charm.land/...` imports): `github.com/spf13/cobra`, `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`, `charm.land/huh/v2`, `gopkg.in/yaml.v3`, `github.com/go-playground/validator/v10`
- [x] 1.5 Verify `go run ./.submodules/nostos/cmd/nostos --version` prints `nostos, version 0.1.0`
- [x] 1.6 Create `.submodules/nostos/README.md` with install + quickstart (reflect Go invocation)

## 2. Configuration model

- [ ] 2.1 Create `internal/config/config.go` with structs: `Cluster`, `Secrets`, `Node`, `Config`
- [ ] 2.2 Add `validator` tags for MAC shape (`mac`), IP (`ip4_addr`), role enum (`oneof=controlplane worker`), arch enum
- [ ] 2.3 Implement `Load(path string) (*Config, error)` using yaml.v3 + validator
- [ ] 2.4 Detect duplicate MACs across nodes
- [ ] 2.5 Human-readable validation errors (custom `ErrorsToString` on validator errors)
- [ ] 2.6 Tests: valid config, invalid MAC, duplicate MAC, missing required field, empty file

## 3. Config discovery

- [ ] 3.1 Create `internal/config/discovery.go` with `FindConfig(explicit string) (string, error)`
- [ ] 3.2 Search order: explicit flag → `$NOSTOS_CONFIG` → cwd/config.yaml → parent dirs for nostos/config.yaml
- [ ] 3.3 Tests for each branch

## 4. Secrets backend

- [ ] 4.1 `internal/secrets/backend.go` — `Backend` interface with `Scheme() string`, `Resolve(uri string) (string, error)`, `Validate() error`
- [ ] 4.2 `internal/secrets/registry.go` — scheme → constructor map; factory from `*config.Secrets`
- [ ] 4.3 `internal/secrets/op.go` — 1Password via `op` CLI subprocess; `Validate()` shells to `op whoami`
- [ ] 4.4 `internal/secrets/env.go` — env var resolver
- [ ] 4.5 `internal/secrets/file.go` — file read + trim
- [ ] 4.6 `internal/secrets/sops.go` — `sops --decrypt --extract` subprocess
- [ ] 4.7 `internal/secrets/resolve.go` — URI regex + `ResolveTemplate(text string, backends map[string]Backend) (string, error)`
- [ ] 4.8 Tests using a fake `Backend` that never hits real secrets
- [ ] 4.9 Regression: error messages contain URIs but never resolved values (asserted via log capture)

## 5. Registry

- [ ] 5.1 `internal/registry/registry.go` exposing `List`, `Get`, `Add`, `Remove`, `Render`
- [ ] 5.2 `Render` loads template, runs secrets resolve, writes to `state/configs/<mac-hyphenated>.yaml`
- [ ] 5.3 MAC-to-hyphen helper (`d0:94:66:d9:eb:a5` → `d0-94-66-d9-eb-a5`, lowercase)
- [ ] 5.4 Post-render `talosctl validate` hook (skip if talosctl absent)
- [ ] 5.5 `Probe(node)` → `NodeStatus` with ping + TCP:50000 + version
- [ ] 5.6 Tests with fixture consumer layout under `testdata/`

## 6. PXE build

- [ ] 6.1 `internal/pxe/embed.go` — embedded `EmbedIpxe` + `BootIpxeTmpl` constants (retry loop + `${next-server}` + `/assets/` URL prefix)
- [ ] 6.2 `internal/pxe/build.go` — `BuildAll(ctx, cfg, paths) error`
- [ ] 6.3 Download kernel + initramfs via `net/http` + `io.Copy` into `state/assets/`
- [ ] 6.4 Clone iPXE (git subprocess) into `state/ipxe-src/` if missing
- [ ] 6.5 `docker run` Alpine cross-compile of `snponly.efi` with `EMBED=embed.ipxe`
- [ ] 6.6 Assert `ipxe.efi` < 300 KiB, fail loud otherwise
- [ ] 6.7 Render `boot.ipxe` with `${next-server}` and `/assets/vmlinuz-<arch>` kernel URL + optional `extra_kernel_args` (used for wipe)
- [ ] 6.8 Unit test for `boot.ipxe` rendering; integration test gated on Docker presence

## 7. PXE serve

- [ ] 7.1 `internal/pxe/serve.go` with `Server` struct + `Serve(ctx)` method
- [ ] 7.2 Start Go stdlib HTTP server (`http.FileServer(http.Dir(paths.State))`) on :9080
- [ ] 7.3 Detect ethernet interface via `net.Interfaces()` (skip lo/awdl/utun); fall back to `ifconfig` parse
- [ ] 7.4 Stage `ipxe.efi` to `/tmp/nostos-tftp/` 0644, ensure dir is 0755
- [ ] 7.5 Start dnsmasq subprocess with PXE vendor-class filter (tag:pxe, tag:ipxe via user-class=iPXE)
- [ ] 7.6 Kill stale HTTP on port before binding (lsof-based)
- [ ] 7.7 **Before rendering boot.ipxe, inspect `state/pending-wipes.json`; if non-empty, re-render with `talos.experimental.wipe=system` in kernel cmdline** (wires the v0.1 Python prototype's missing piece)
- [ ] 7.8 Clean shutdown on SIGINT/SIGTERM: terminate children, remove staged TFTP files, restore boot.ipxe (without wipe flag)
- [ ] 7.9 Write logs to `state/logs/serve-<timestamp>.log`
- [ ] 7.10 `nostos serve --down` kills any stale nostos-managed dnsmasq + http-server

## 8. Cluster control

- [ ] 8.1 `internal/cluster/bootstrap.go` wrapping `talosctl bootstrap`
- [ ] 8.2 Poll for etcd Running/OK with configurable timeout (default 5 min)
- [ ] 8.3 Reject bootstrap on non-controlplane role
- [ ] 8.4 Idempotent on already-bootstrapped
- [ ] 8.5 Fetch kubeconfig to `state/kubeconfig` after success
- [ ] 8.6 `internal/cluster/cert.go` — **native admin cert regen**:
  - [ ] 8.6.1 Extract CA cert + key from rendered machineconfig (parse YAML, base64-decode)
  - [ ] 8.6.2 Generate Ed25519 keypair via `crypto/ed25519`
  - [ ] 8.6.3 Build CSR with custom Talos extension OID `1.3.6.1.4.1.58107.1.1` carrying `os:admin` role bytes
  - [ ] 8.6.4 Sign with CA via `x509.CreateCertificate`
  - [ ] 8.6.5 PEM-encode cert + key; base64-encode for talosconfig YAML
  - [ ] 8.6.6 Emit `state/talosconfig` with context matching the cluster name, mode 0600
  - [ ] 8.6.7 Regression test: byte-diff the custom extension against a reference from `talosctl gen crt`
- [ ] 8.7 `internal/cluster/status.go` — per-node probe (reuses `registry.Probe`), returns `[]NodeStatus`
- [ ] 8.8 `internal/cluster/wipe.go` — JSON-persisted pending-wipes with `Queue`, `Consume`, `Pending` functions
- [ ] 8.9 `internal/cluster/orchestrate.go` — end-to-end `Install(ctx, cfg, paths, node, events chan<- Event)`:
  - [ ] 8.9.1 Queue wipe (unless skipWipe); on exit restore pending-wipes if interrupted
  - [ ] 8.9.2 Ensure `BuildAll` completed; ensure `Render` completed
  - [ ] 8.9.3 Start serve subflow (inline, not separate process) with log tailing
  - [ ] 8.9.4 Detect `GET /configs/<mac>.yaml` in HTTP access log → emit `ConfigFetched` event
  - [ ] 8.9.5 Poll ping + apid at static IP until both up, or timeout
  - [ ] 8.9.6 Stop serve subprocesses
  - [ ] 8.9.7 Consume wipe flag (success case); restore boot.ipxe clean
  - [ ] 8.9.8 If role=controlplane: run bootstrap + fetch kubeconfig
  - [ ] 8.9.9 Emit `Ready` event

## 9. CLI wiring

- [x] 9.1 `internal/cli/root.go` — root cobra command, global flags `--config`, `--output`, `--verbose`
- [x] 9.2 `init.go` — scaffold config.yaml + templates/ + state/.gitignore
- [~] 9.3 `node.go` — `add` (Huh form), `list` (Lipgloss table), `remove` (--yes) — list + remove wired; `add` Huh form deferred to section 10
- [ ] 9.4 `build.go` — calls `pxe.BuildAll`
- [x] 9.5 `render.go` — calls `registry.Render`
- [ ] 9.6 `serve.go` — calls `pxe.Server.Serve`; respects pending-wipes
- [ ] 9.7 `up.go` — creates Event channel + launches Bubble Tea `tui.Install` program
- [ ] 9.8 `wipe.go` — `cluster.wipe.Queue`
- [ ] 9.9 `bootstrap.go` — `cluster.Bootstrap` + `FetchKubeconfig`
- [ ] 9.10 `cert.go` — `nostos config refresh --hours N`
- [x] 9.11 `status.go` — `--watch` launches `tui.Status` program; else plain table (plain table done; `--watch` deferred)
- [ ] 9.12 `kubeconfig.go` — `FetchKubeconfig` only
- [ ] 9.13 `nuke.go` — `os.RemoveAll(paths.State)` with `--yes` gate
- [ ] 9.14 Error formatter wraps errors; stack trace under `--verbose`
- [x] 9.15 `completion` subcommand from cobra built-ins (bash/zsh/fish/powershell)

## 10. TUI

- [ ] 10.1 `internal/tui/style.go` — Lipgloss shared styles
- [ ] 10.2 `internal/tui/install.go` — Bubble Tea model: list of Events with icons + colors, spinner for progress, final "Ready" panel
- [ ] 10.3 `internal/tui/status.go` — Bubble Tea model: refreshing `bubbles/table`, key bindings `r` (refresh), `q` (quit)
- [ ] 10.4 `internal/tui/nodeadd.go` — Huh form with MAC/IP/role/arch/disk/template fields + inline validation
- [ ] 10.5 Non-TTY fallback: detect `isatty(stdout)`; fall back to plain-text Event streaming
- [ ] 10.6 `teatest` golden snapshots for install + status

## 11. End-to-end verification

- [ ] 11.1 `go test ./...` passes for all unit tests (config, secrets, registry, cert, cli dispatch, TUI models)
- [x] 11.2 Byte-identical render vs `op inject` on the same template (regression test against Python prototype's result) — verified live: `diff /tmp/legacy.yaml nostos/state/configs/d0-94-66-d9-eb-a5.yaml` is empty, both 12351 bytes
- [ ] 11.3 Integration: `go test -tags=integration ./...` runs iPXE Docker build, asserts size < 300 KiB
- [ ] 11.4 Manual smoke: `nostos up dell01` against real Dell with Docker running; confirm full flow power-on → Ready in under 10 min

## 12. Packaging and docs

- [ ] 12.1 `.submodules/nostos/README.md` with schema reference, supported secrets backends, BIOS checklist
- [ ] 12.2 `.submodules/nostos/CHANGELOG.md` with v0.1.0 entry
- [ ] 12.3 `.submodules/nostos/LICENSE` (MIT)
- [ ] 12.4 Update `.agents/drafts/nostos-pitch.html` to reflect Go+Charm stack
- [ ] 12.5 Update `openspec/changes/adopt-nostos/` tasks/design to reference `go run ./.submodules/nostos/cmd/nostos ...` invocation (was `uv run --project`)
