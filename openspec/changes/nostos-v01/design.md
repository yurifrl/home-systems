## Context

`nostos` is a new greenfield Python tool that consolidates bare-metal Talos provisioning for single-operator home labs. The proposal establishes WHY; this document establishes HOW.

Current state: every Talos home-labber writes bespoke shell scripts, hits the same failure modes, and loses weekends. The existing `home-systems` repo has four such scripts (`pxe/scripts/*.sh`), four Taskfiles, one README, and an incomplete trouble-shooting doc. Knowledge lives in conversation logs. No test coverage. No single entry point.

Constraints:
- **Ship a useful v0.1 in one development session.** Not a research project.
- **Single-operator assumptions.** No RBAC, no multi-user, no cloud.
- **Python-only.** Consumer already uses 1Password CLI, `talosctl`, `dnsmasq`, `docker`; adding a Python package is minimal friction. Go/Rust rewrite is scope creep.
- **Local-first.** Zero phone-home, zero mandatory SaaS, offline-capable after initial Talos asset download.
- **Pluggable secrets.** 1Password works for the initial user but must not be load-bearing — sops/env/file backends need to be 20-line plugins.
- **`.submodules/nostos/` is the development path.** Code lives in the consumer repo during v0.1; extract to own GitHub repo once stable. No premature split.

Stakeholders: a single developer (Yuri) for v0.1; any Talos home-labber once open-sourced.

## Goals / Non-Goals

**Goals:**
- One CLI binary (`nostos`) that fully owns bare-metal → `kubectl get nodes` showing `Ready`.
- Reproducible: `rm -rf state/` + re-run same commands = same result.
- Fail loud on well-known traps (DHCP race, expired cert, persisted STATE partition, BIOS boot order) with actionable error messages, not silent stuck-states.
- Secrets-backend adapter interface that's trivially extensible.
- uv-managed, `pipx install`able package (even if published to PyPI later).
- Testable: every capability module importable + unit-testable without needing real hardware.

**Non-Goals:**
- Cross-architecture host (only macOS + Linux hosts supported; Windows users should run in WSL2 or VM).
- GUI installer. `nostos web` is a status dashboard + command helper, not an installer wizard.
- Agent-based provisioning (Tinkerbell-style). Everything is push from operator laptop.
- Replacing `talosctl`. Where `talosctl` already does the job well, we shell out to it.
- Replacing `dnsmasq`. Writing a DHCP/TFTP server in Python is scope creep and a security minefield.
- Caching Talos factory assets across consumer-repos (no shared cache dir). Each `state/` is self-contained.
- Simultaneous cluster operations. One `nostos` invocation = one cluster context.

## Decisions

### D1. Language + packaging: uv-managed Python package

Chose **Python 3.11+ with uv** and a proper package layout (`src/nostos/` with `pyproject.toml`).

Alternatives considered:
- Single-file `uv run --script` (PEP 723). Rejected for v0.1 because: (a) web UI needs static assets and template files, (b) splitting into `cli.py`, `pxe.py`, etc. is already natural from day 1, (c) testability wants a proper package.
- Go. Rejected: longer iteration cycle for v0.1, no ecosystem win here, consumer already has Python via uv.
- Rust. Same as Go, plus higher barrier to contribution for home-labbers.
- Shell (keep current scripts). Rejected: that's the status quo this change exists to fix.

### D2. CLI framework: `click`

Chose `click`. Mature, stable, matches user's existing `python` skill preferences, good composition for nested commands (`nostos node add`, `nostos node list`).

Alternatives: `typer` (click wrapper with type hints — fine, but adds a dep), `argparse` (too verbose for 12-command surface).

### D3. Secrets adapter contract

Each backend implements:
```python
class SecretsBackend(Protocol):
    def resolve(self, uri: str) -> str: ...
    def validate(self) -> None: ...  # e.g. "am I signed in?"
```

URIs follow RFC 3986 syntax: `<scheme>://<authority>/<path>`:
- `op://<vault>/<item>/<field>` — 1Password (default)
- `sops://<file>#<key>` — sops-decrypted file + JSON path
- `env://<VARNAME>` — environment variable
- `file://<absolute-path>` — raw file content

Templates never change when switching backends — only `config.yaml` selects which backend resolves which scheme. A template using `op://kubernetes/talos/TS_AUTHKEY` works unchanged if the user's config points that URI at a sops backend (via aliases, if we add that later).

Alternatives: Vault integration, HashiCorp-style secret paths. Rejected for v0.1: not needed for the target user.

### D4. State directory — deterministic cache

All derived artifacts under `<config-dir>/state/` (configurable). Three invariants:
1. **Reproducibility.** Every file is regenerable from `config.yaml` + `templates/` + secrets-backend.
2. **Gitignore by default.** `nostos init` writes a `.gitignore` covering `state/`.
3. **Safe `rm`.** Nothing outside `state/` (and the per-device admin cert, which has a docstring flag). `nostos nuke` command wipes it cleanly.

Layout:
```
state/
├── assets/
│   ├── vmlinuz-amd64
│   ├── initramfs-amd64.xz
│   ├── ipxe.efi
│   └── boot.ipxe
├── ipxe-src/                      # iPXE checkout (gitignored), kept for incremental builds
├── configs/
│   └── <mac-hyphenated>.yaml      # rendered secret-bearing machineconfigs
├── talosconfig                    # admin cert (per-device, ephemeral)
├── kubeconfig                     # fetched post-bootstrap
├── cache/                         # HTTP response cache, e.g. schematic-id lookups
└── logs/
    └── serve-<timestamp>.log
```

### D5. PXE serving — keep dnsmasq + Python HTTP

`nostos serve` orchestrates two processes:
1. `python -m http.server` subprocess on port 9080 serving `state/assets/`
2. `dnsmasq` subprocess (requires sudo, as today) for DHCP + TFTP

We considered writing a Python TFTP server (no sudo needed for port 69 as root, but we'd still need sudo for port 67 DHCP). Keeping dnsmasq:
- Battle-tested for 20+ years; won't introduce subtle bugs under PXE ROM edge cases.
- Operator already has it installed (homebrew).
- The complexity was never dnsmasq — it was the glue. That's what we're replacing.

The iPXE retry-until-`${filename}` trick and all other hard-won knowledge is preserved in templated dnsmasq config + embedded iPXE script.

### D6. iPXE build — Docker in v0.1

`nostos build` uses Docker to cross-compile `ipxe.efi` (same as current `1-build-assets.sh`). Future `nostos build --prebuilt` option downloads a signed binary from our GitHub releases; out of scope for v0.1.

### D7. Admin-cert regeneration (`nostos config refresh`)

Solves the trap we hit this session: the Talos admin client cert expired, locking the operator out of `talosctl`. Flow:
1. Read CA cert + CA key from current rendered machineconfig (or from secrets backend).
2. `talosctl gen key`, `gen csr --roles os:admin`, `gen crt --ca ca --csr admin.csr --hours 876000` (≈100 years — acceptable for home-lab threat model; production users can pick shorter).
3. Write `state/talosconfig` with the new cert.

This is the single most valuable UX win of the whole tool. Cannot be implemented in shell without re-writing a lot.

### D8. Web UI — FastAPI + vanilla JS, localhost-only

`nostos web` starts FastAPI on `127.0.0.1:8080`. Bind is hardcoded to loopback. No auth — if an attacker is on loopback, they already have the laptop. Allows mutations (reinstall/wipe/bootstrap) per operator request — each mutation requires a typed confirmation ("type DELL01 to confirm") to prevent fat-finger destruction.

Alternatives: Flask (fine, FastAPI preferred for async + OpenAPI), Django (overkill), native (no, requires a frontend build step).

### D9. Command scope — v0.1 vs later

v0.1 ships: `init`, `node add`, `node list`, `build`, `render`, `serve`, `install` (cheat-sheet printer), `nuke` (wipe state dir).

v0.2 adds: `bootstrap`, `config refresh`, `status`, `wipe` (one-shot node wipe via PXE flag).

v0.3 adds: `web`, `node remove`, full schema validation.

Earlier commands don't block on later ones — happy path works end-to-end with v0.1 commands + manual `talosctl bootstrap`. The three post-install commands are convenience wrappers around talosctl that the operator can run by hand.

### D10. Configuration schema — pydantic

`config.yaml` parsed into pydantic models. Validation errors produce human-readable messages before any side effect runs. Example:
```yaml
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
secrets:
  backend: onepassword
  onepassword:
    account: my.1password.com
    vault: kubernetes
nodes:
  dell01:
    mac: "d0:94:66:d9:eb:a5"
    ip: 192.168.68.100
    role: controlplane
    arch: amd64
    install_disk: /dev/nvme0n1
    template: dell01.yaml
```

## Risks / Trade-offs

- **Python `dnsmasq` process management on macOS:** signal delivery + sudo orphan processes. → Mitigation: always launch via `subprocess.Popen` with a sentinel file; `nostos serve` has an idempotent `--down` flag that kills stale processes regardless of whether the parent exited cleanly.
- **iPXE binary size limit (~256KB for Dell UEFI TFTP):** future iPXE releases might exceed. → Mitigation: build job asserts binary <256KB, fails loud.
- **1Password session timeout during long operations:** `op` CLI expires mid-flight. → Mitigation: secrets backend pre-resolves all needed URIs at render-time in one pass, caches in-memory for the duration of a single invocation only.
- **macOS `base64` emits CRLF:** hit this session, bricked admin-cert generation for 20 minutes. → Mitigation: Python's `base64` module instead of shelling out, eliminates class of bug.
- **Docker dependency in v0.1 friction:** users without Docker can't build iPXE. → Mitigation: clear error message with install instructions; v0.2 prebuilt binary option.
- **Extraction to separate repo later:** path refactoring risk. → Mitigation: code from day 1 assumes it's a standalone package (no imports from outside `.submodules/nostos/`, no hardcoded parent-repo paths).

## Migration Plan

This change's migration plan is limited to "create a new tool in `.submodules/nostos/`." Consumer-side migration is the `adopt-nostos` change.

1. Scaffold `.submodules/nostos/` with pyproject.toml, package skeleton, CI basics.
2. Implement capabilities in dependency order: `secrets-backend` → `node-registry` → `pxe-provisioning` → `cluster-control` → `nostos-cli` → `nostos-web`.
3. Port existing home-systems knowledge via tests (fixtures from this repo's current Dell flow prove parity).
4. v0.1 tag inside `.submodules/nostos/` when all v0.1 commands work end-to-end on the Dell.

Rollback: `rm -rf .submodules/nostos/`. Because `adopt-nostos` has not yet run, nothing in `home-systems` depends on this code. Rollback is trivial until that second change lands.

## Open Questions

- **Q1.** Extraction timing: when to split `.submodules/nostos/` into `github.com/yurifrl/nostos`? Proposed: after v0.2 (once `status` + `bootstrap` commands prove the design beyond initial provisioning).
- **Q2.** Binary distribution: PyPI + `pipx install nostos`? Or Homebrew tap? Or both? Defer to extraction time.
- **Q3.** Should `nostos render` check 1Password session liveness proactively (`op whoami`) and prompt for signin, or error out? Lean toward: prompt interactively if stdin is a TTY, error out otherwise.
- **Q4.** Telemetry — none in v0.1. Should there be anonymous opt-in crash reporting later? Unclear; defer.
