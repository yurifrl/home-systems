## Context

Second attempt at `nostos`. The Python prototype on the `python` branch taught
us the hard parts: HTTP/DNS serve layout, iPXE embed script, Deco-DHCP race,
persisted-STATE trap, admin-cert regen. This rewrite keeps those lessons and
trades Python+rich+FastAPI for Go+Cobra+Charm.

Constraints:
- **Ship a usable v0.1 end-to-end.** Parity with the Python prototype's working
  subset (build, render, serve, up, wipe, bootstrap, status, config refresh,
  nuke). No feature regressions.
- **Single verb: `go run`.** No install, no build step in the operator flow.
- **Charm TUI, not web UI.** Bubble Tea owns interactive views. Drop FastAPI.
- **Reuse the consumer contract.** `nostos/config.yaml` + `nostos/templates/*`
  on disk stay valid. A consumer who ran the Python prototype can swap the
  tool without editing any YAML.
- **Preserve byte-identical render.** `nostos render dell01` output must equal
  `op inject` output for the same template. This was verified in the Python
  prototype (11.2) and is a regression-guard for the rewrite.

Non-constraints:
- Zero performance targets beyond "feels instant." No benchmarking.
- No cross-compilation. macOS + Linux host only.
- No Windows support.

## Goals / Non-Goals

**Goals:**
- `nostos up <node>` drives bare-metal → `kubectl get nodes` Ready in one invocation.
- All hard-won behaviors preserved: iPXE retry-loop embed, `${next-server}` templating, one-shot wipe with auto-clear, cert regen against existing CA.
- Cobra-based subcommand tree where each leaf command is either (a) a plain function that does its thing and prints results, or (b) launches a Bubble Tea program for the few interactive flows.
- Test coverage equivalent to Python's 72-test suite: config parsing, secret resolution, registry operations, build logic (mocked Docker), serve (mocked processes), cluster ops (mocked talosctl), CLI dispatch, TUI models via `teatest`.

**Non-Goals:**
- Web UI. Deferred to v0.3.
- `go install` distribution. v0.1 is `go run` only.
- Binary releases / Homebrew formula. v0.2 maybe.
- Windows / WSL. macOS + Linux host only.
- Multi-cluster orchestration.
- Performance tuning beyond "startup is acceptable."

## Decisions

### D1. Language + invocation: Go ≥1.22, always `go run`

Chose Go for: single static binary compile model (used internally via build cache), stdlib strength for crypto/network/exec, first-class concurrency for log tailing during install, Charm ecosystem maturity.

Invocation is pinned to `go run ./cmd/nostos`. Rationale:
- No install step → no stale binary lag when developing the tool in-tree.
- Go's build cache makes subsequent `go run` sub-200ms.
- Keeps the `.submodules/nostos/` a normal Go module that anyone can `git clone && go run`.
- Eventually publishes as a binary (v0.2+), but that's orthogonal.

Alternatives rejected:
- Stay in Python. Archived as `python` branch — works for prototype, hit TUI rendering limits and "operator doesn't want uv" feedback.
- Rust. Longer iteration loop, smaller Charm-equivalent TUI ecosystem (ratatui is nice but less batteries-included than Bubble Tea).
- Zig/other. Ecosystem cost doesn't match v0.1 needs.

### D2. CLI framework: Cobra + Bubble Tea split

Cobra handles subcommand parsing, flag wiring, help text, shell completions. Bubble Tea handles interactive full-screen flows. They compose cleanly:
- Leaf command like `nostos render dell01` runs as plain Go, prints result.
- Leaf command like `nostos up dell01` instantiates a Bubble Tea `tea.Program` that renders progress events as they arrive from the orchestrator's event channel.
- Interactive wizard `nostos node add` uses Huh forms (form library built on Bubble Tea).

Alternatives: pure `flag` stdlib (too verbose for 12+ subcommands), urfave/cli (fine, but Cobra ubiquitous in Go ecosystem and integrates well with Charm demos).

### D3. TUI event model

Orchestrator emits `Event` values on a channel. Bubble Tea v2 `Update` consumes them and returns a new model + `tea.Cmd`. This mirrors the Python generator pattern but in idiomatic Go v2:

```go
import tea "charm.land/bubbletea/v2"

type Event struct {
    Kind    EventKind
    Message string
    Node    string
    At      time.Time
}

type model struct {
    events []Event
    done   bool
    err    error
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyPressMsg:        // v2: KeyPressMsg, not KeyMsg
        if msg.String() == "q" || msg.String() == "ctrl+c" {
            return m, tea.Quit
        }
    case Event:
        m.events = append(m.events, msg)
        if msg.Kind == KindReady { m.done = true; return m, tea.Quit }
    }
    return m, nil
}

func (m model) View() tea.View {   // v2: returns tea.View, not string
    v := tea.NewView(renderEvents(m.events))
    // v.AltScreen = false by default; --alt-screen opts in.
    return v
}

// In the orchestrator:
func Install(ctx context.Context, cfg *Config, node *Node, events chan<- Event) error { ... }
```

The TUI and the orchestrator are independently testable. `teatest` exercises the TUI via fake events; the orchestrator is tested against fake processes (via a mockable `Command` interface).

**v2 API rules the implementation MUST follow** (see `charm-stack` skill for full reference):
- `View()` returns `tea.View`, not `string`. Wrap text with `tea.NewView(s)`.
- Key handling uses `tea.KeyPressMsg` (and `tea.KeyReleaseMsg` when keyboard enhancements are enabled). Never `tea.KeyMsg`.
- Terminal features (alt-screen, mouse mode, window title) are declared on the `tea.View` struct, not as `tea.Cmd`s. No `tea.EnterAltScreen`.
- Bubbles components use functional-option constructors (e.g. `viewport.New(viewport.WithWidth(80))`) and getter/setter methods (e.g. `vp.SetWidth(40)` / `vp.Width()`). Never set exported fields.
- Lipgloss `AdaptiveColor` is removed; use `lipgloss.LightDark(isDark)(light, dark)` driven by `tea.BackgroundColorMsg`.

### D4. Directory layout

```
.submodules/nostos/
├── go.mod                             module github.com/yurifrl/nostos
├── go.sum
├── cmd/nostos/
│   └── main.go                        cobra root; wires subcommand registry
├── internal/
│   ├── cli/                           subcommand constructors (return *cobra.Command)
│   │   ├── root.go
│   │   ├── init.go
│   │   ├── node.go                    node add/list/remove
│   │   ├── build.go
│   │   ├── render.go
│   │   ├── serve.go
│   │   ├── up.go                      launches tui.InstallProgram
│   │   ├── wipe.go
│   │   ├── bootstrap.go
│   │   ├── cert.go                    nostos config refresh
│   │   ├── status.go
│   │   ├── kubeconfig.go
│   │   └── nuke.go
│   ├── config/                        yaml parsing + validation
│   ├── registry/                      node operations
│   ├── secrets/
│   │   ├── backend.go                 interface
│   │   ├── registry.go                scheme → constructor map
│   │   ├── op.go
│   │   ├── sops.go
│   │   ├── env.go
│   │   ├── file.go
│   │   └── resolve.go                 URI-matching regex + template walk
│   ├── pxe/
│   │   ├── build.go                   asset download + Docker iPXE build
│   │   ├── serve.go                   HTTP + dnsmasq subprocess supervisor
│   │   └── embed.go                   embed.ipxe + boot.ipxe as Go strings
│   ├── cluster/
│   │   ├── bootstrap.go
│   │   ├── cert.go                    native crypto/x509, no talosctl shell-out
│   │   ├── status.go
│   │   ├── wipe.go                    pending-wipes json persistence
│   │   └── orchestrate.go             install_node equivalent, emits Events
│   └── tui/
│       ├── install.go                 Bubble Tea model for `up`
│       ├── status.go                  Bubble Tea model for `status --watch`
│       ├── nodeadd.go                 Huh form for `node add`
│       └── style.go                   Lipgloss shared styles
├── testdata/
└── README.md
```

### D5. iPXE build strategy unchanged

Still Docker-cross-compile the same `snponly.efi` with the same retry-loop embed script. Go shells to `docker run`. Size ceiling unchanged (300 KiB — empirically confirmed OK for Dell OptiPlex UEFI TFTP).

### D6. HTTP serve root is the state dir (not state/assets)

The Python prototype's last bug: HTTP served from `state/assets/`, so `/configs/<mac>.yaml` 404'd because `state/configs/` was sibling, not child. The Go impl serves from `state/` as root, so:
- `GET /assets/vmlinuz-amd64`     → state/assets/vmlinuz-amd64
- `GET /assets/initramfs-amd64.xz` → state/assets/initramfs-amd64.xz
- `GET /assets/boot.ipxe`          → state/assets/boot.ipxe
- `GET /configs/<mac>.yaml`        → state/configs/<mac>.yaml

This is the key structural fix. boot.ipxe's kernel URL must include the `/assets/` prefix.

### D7. Admin-cert regeneration: native, no talosctl gen shell-out

The Python version shelled to `talosctl gen key/csr/crt` for admin cert regen. Go has `crypto/ed25519`, `crypto/x509`, `encoding/pem`, `encoding/base64` in the stdlib — generate the Ed25519 keypair, marshal a CSR with the `os:admin` role extension, sign with the cluster CA (also from stdlib), PEM-encode, base64-wrap, emit `talosconfig` YAML. Zero subprocesses. Faster, testable without installing talosctl.

The `os:admin` role extension encoding: Talos uses a custom x509 extension OID (`1.3.6.1.4.1.58107.1.1`) whose value is a comma-joined ASCII list of roles. We replicate this directly; documented in `internal/cluster/cert.go`.

### D8. One-shot wipe flag: actually wired this time

Python v0.1 had this as a design invariant but never wired it through `render_boot_ipxe`. Critical fix: during `nostos up` (and on every `nostos serve` boot rendering), the serve layer reads `state/pending-wipes.json` and re-renders `boot.ipxe` with `talos.experimental.wipe=system` appended to the kernel cmdline for the target MAC. Since iPXE runs a single boot.ipxe for all clients (we can't template per-MAC server-side easily), wipe is all-or-nothing per serve session. If multiple wipes queued, all matching nodes wipe on their next PXE boot. Pending-wipes are cleared on successful install-completion (detected via `nostos up` waiting for node to come back at its static IP and apid to bind).

Spec requirement: `nostos serve` must inspect pending-wipes before rendering boot.ipxe. `nostos up` does the same via its own serve subflow.

### D9. Testing

- Unit: `go test ./...` for pure functions (config validation, URI resolution, cert generation, iPXE embed text).
- Subprocess mocking: wrap `exec.Command` in an interface (`Commander`) so tests inject fakes for `op`, `talosctl`, `docker`, `dnsmasq`.
- TUI: `teatest` with `termtest.Finished` + golden snapshots.
- Integration: `//go:build integration` tag gates tests that need real Docker / real op / real Talos node. Run via `go test -tags=integration ./...` when available.
- No coverage target in v0.1 — "happy paths all green" is the bar.

### D10. Package visibility

Everything under `internal/` — not importable from outside. The tool isn't a library in v0.1, it's a binary. If v0.3+ wants to let others build on top of the orchestrator, we promote `internal/cluster` to `pkg/cluster` at that time.

## Risks / Trade-offs

- **[Risk] `go run` cold-start latency for first-time operator** → Mitigation: Go's module cache means the first `go run` downloads deps (~10-30s once), subsequent invocations are sub-200ms. Taskfile wrappers include a `task nostos:warm` one-liner that runs `go build -o /dev/null ./cmd/nostos` to pre-populate the build cache.
- **[Risk] Charm libraries semver churn** → Mitigation: pin exact versions in go.mod; upgrade opt-in per change.
- **[Risk] Operator loses TUI output on pipe / non-TTY** → Mitigation: detect `isatty(stdout)`; fall back to plain-text event log if not a TTY. Bubble Tea supports this via `tea.WithOutput`.
- **[Trade-off] No web UI in v0.1** → Accepted. TUI covers single-operator case. Web UI resurrected in v0.3 if demand exists.
- **[Trade-off] No compiled binary distribution** → Accepted. v0.1 is "clone and `go run`". v0.2 conversation.
- **[Risk] Native x509 implementation of os:admin role extension doesn't match Talos expectations** → Mitigation: write a test that decodes an existing `talosctl gen crt`-produced cert and asserts our emitted cert has identical extension bytes.
- **[Risk] Hard-won Python bugfixes (like HTTP cwd, wipe flag wiring, Deco DHCP retry) get lost in rewrite** → Mitigation: the design doc (this file) explicitly enumerates them; tasks.md covers each as a numbered step.

## Migration Plan

This change is greenfield on main (Python prototype preserved on `python` branch).

1. Scaffold `.submodules/nostos/` with `go.mod`, `cmd/nostos/main.go` returning "hello".
2. Port capabilities in dependency order: `config` → `secrets` → `registry` → `pxe/build` → `pxe/serve` → `cluster/cert` → `cluster/bootstrap` → `cluster/orchestrate` → `tui` → wire them through `cli`.
3. Verify byte-identical render vs `op inject` (regression test against the Python prototype's parity result).
4. Verify iPXE binary builds to ≤300 KiB via Docker in CI.
5. Manual smoke: `nostos up dell01` against a freshly-wiped Dell, confirm the full flow from power-on to `kubectl get nodes` Ready.
6. Update `nostos-pitch.html` to reflect Go+Charm stack (rewrite code snippets).
7. Update `adopt-nostos` change's Taskfile references from `uv run --project` to `go run ./.submodules/nostos/cmd/nostos`.

Rollback: `rm -rf .submodules/nostos/`. Python prototype still exists at the `python` branch for reference.

## Open Questions

- **Q1.** Go module path: `github.com/yurifrl/nostos` at extraction time? Currently `go.mod` declares `module nostos` (unqualified, only valid in-tree). Decide at v0.2 extraction.
- **Q2.** Bubble Tea alt-screen mode for `nostos up`? Full-screen alt-screen looks polished but hides the terminal history. Default to inline rendering (no alt-screen); operator can opt in with `--alt-screen` if they prefer.
- **Q3.** `nostos status --watch` as a TUI loop? Yes for v0.1. `nostos status` (non-watch) stays as a plain-text table for scripting.
- **Q4.** `--output json` implementation across every command: consistent shape, or per-command ad-hoc? Lean toward a top-level `Response` envelope with `ok` / `error` / `data` for uniform script consumption.
