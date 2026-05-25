## Why

`nostos` v0.2 (`openspec/changes/nostos-v02-provisioners/`) shipped the
`Provisioner` interface, the `tpi` provider, and a refactored PXE flow.
Real-world use against tp1/tp4 surfaced four operational defects, the
secrets pipeline split between nostos and the legacy `taskfiles/turing.yml`
recipes is now actively confusing, and the CLI is still mostly
prose-output — both AI agents and tired humans struggle to drive it.

This change closes the v0.2 bug list, retires the legacy taskfile recipes,
hardens every `nostos` subcommand against agent invocation (JSON I/O,
schema introspection, dry-run, structured errors), ships an interactive
Bubble Tea v2 **dashboard** as a *live status board* (read-only in v0.3 —
not an autopilot), and publishes the canonical PXE+TPI operator guide
(install + recover only; per-vendor playbooks slip to v0.4).

It deliberately does NOT introduce an MCP server (deferred to v0.4),
new provisioners (`redfish` / `proxmox` / `usb` ride their own openspec
changes per the v0.2 roadmap), Tailscale device cleanup mutations
(security review surfaced spoofing concerns — deferred to v0.4), or
dashboard action dispatch (`r`/`d`/`i` keys deferred to v0.4).

### Concrete pains motivating v0.3

- **Bug #1** — `BootTimeout` defaults to 10 min in
  `.submodules/nostos/internal/cluster/orchestrate.go:64-65`, but RK1
  cold boots routinely take 12-18 min. Operator falls back to manually
  running `talosctl apply-config --insecure`, defeating the point.
- **Bug #2 regression risk** — the empty-filename bug for tpi-rendered
  configs was patched in v0.2 but has no pinned regression test.
- **Bug #5** — image cache TOFU (`internal/provisioner/tpi/image_cache.go:27-39`)
  records the digest in a side-file (`digests.json`) AFTER trusting the
  on-disk file. Kill -9 mid-download leaves a partial file plus, on the
  next run, a chance of recording a corrupt digest as canonical.
- **Bug #6** — wrong/unreachable BMC surfaces as
  `Device not configured (os error 6)` from the `tpi` subprocess; the
  message blames the OS, not the BMC. Operators waste 20+ minutes
  chasing kernel-level red herrings.
- **Hygiene** — `taskfiles/turing.yml` still ships `flash` / `download` /
  `install-talos` recipes that bypass the secrets pipeline. They will
  silently produce a misconfigured node if invoked.
- **Hygiene** — `kubectl get nodes` still lists `talos-76w-r75`, a
  zombie tp1 entry from before v0.2. Tailscale tailnet keeps offline
  ephemeral devices long after the bare metal was reflashed.
- **AI usability** — `nostos status` prints lipgloss-styled prose. There
  is no `--output json`, no `nostos schema`, no `--dry-run`. Agents
  cannot reliably parse the output, cannot discover flags, and cannot
  preview destructive commands.
- **Operator usability** — to answer "is the cluster healthy?" today an
  operator runs `kubectl get nodes`, `talosctl health`,
  `tailscale status`, `kubectl get applications -n argocd`, and reads
  four different formats. No single pane.
- **Documentation gap** — `nostos/README.md` plus `docs/talos.md` plus
  `docs/turingpi.md` collectively describe maybe 60% of the install
  flow, none of the recovery scenarios, and zero per-vendor BIOS quirks.

## What Changes

### Stream A — v0.2 bug fixes

- **A1** Honor a per-provisioner deadline. Add
  `Provisioner.MaxWaitMaintenance() time.Duration` (default 0 = use
  the orchestrator default). `tpi` returns 30 min; `pxe` returns 0
  (orchestrator's 20 min stands). The orchestrator uses
  `max(opts.WaitMaintenanceDeadline, prov.MaxWaitMaintenance())`.
  Per-flag override `--wait-deadline` still wins.
- **A2** Rework image-cache TOFU race. Stream-hash on download into a
  `*.part` file, write the digest record only after `Close+Rename`
  succeeds, and on cache-hit re-verify the digest before trusting the
  recorded value. `kill -9` mid-download leaves no file and no digest
  record.
- **A3** BMC pre-flight. Before calling the `tpi` binary, probe
  TCP `host:443`, then HTTP `GET /` with auth. Translate the three
  failure modes to typed errors: `ErrBMCUnreachable`, `ErrBMCAuth`,
  `ErrBMCVersion`. Exit code on the wrapped `tpi` error is mapped to
  the same set when the pre-flight passed but `tpi` still failed.
- **A4** Pin a regression test for the v0.2 `.yaml` empty-filename fix.
  Loading `nodes[tp1]` MUST yield a non-empty rendered filename
  identifying the node.

### Stream B — Hygiene and cleanup

- **B1** Replace `taskfiles/turing.yml` recipes with thin nostos
  wrappers. Existing `flash`, `download`, `install-talos`, `get`
  recipes print a deprecation message and `exit 1`. New `task
  turing:install NODE=<name>` is an alias for `task nostos:install`.
- **B2** Same treatment for `taskfiles/talos.yml::apply` (workers) where
  nostos has the equivalent (`tp1`, `tp4` covered by `nostos node
  install`). `vm-pc01` and `pc01` recipes survive untouched (no
  provider yet — see v0.2 non-goals 7.2/7.3).
- **B3** `kubectl delete node talos-76w-r75` is documented as a
  one-shot cleanup step in the operator guide; not automated by
  nostos in v0.3.
- **B4 — DEFERRED to v0.4.** Tailscale device cleanup. Security review
  flagged that the OAuth-backed DELETE path is exploitable for tailnet
  spoofing if an attacker forces nostos to delete a still-trusted
  device. v0.3 keeps the existing manual `tailscale` CLI workflow and
  documents it in the operator guide; v0.4 ships `nostos cluster
  cleanup` with explicit allow-listing + two-keystroke confirm.

### Stream C — AI-friendly CLI hardening

Drives every `nostos` subcommand through the 8 principles from
`/Users/yuri/.agents/skills/ai-friendly-cli/SKILL.md`. v0.3 ships
**C1–C7**; **C8** (MCP) is deferred to v0.4.

- **C1** `--output json` (NDJSON for list operations) on every command
  that today prints prose. Unset / `--output text` keeps current
  behavior. Default remains `text` for humans.
- **C2** New `nostos schema [<command-path>]` subcommand returns the
  full flag/arg/type/required/enum graph as JSON. With no argument,
  emits the full tree. With an argument, emits a single command's
  schema.
- **C3** Field masks — `--fields=id,ip,role` on `list` / `show` /
  `dashboard --once` for token-cost discipline.
- **C4** `--dry-run` on every mutation: `node install`, `secrets keys
  revoke`, `cluster cleanup`, future `cluster upgrade`. Dry-run output
  is JSON describing the planned actions.
- **C5** Structured errors — JSON `{error, code, message, details}` on
  stdout (when `--output json`) or stderr (when text), separate from
  any prose hint stream. Exit codes are documented in the schema and
  in `AGENTS.md`.
- **C6** Input hardening — reject **all** ASCII control characters
  (0x00-0x1F + 0x7F) in any user-supplied string (node names, field
  masks, `--config` paths); reject `..` segments and any path that
  resolves outside the operator's home directory or the repo root
  *after* symlink resolution; reject embedded query parameters or
  fragments in `op://` refs; reject YAML inputs whose anchors resolve
  to filesystem paths.
- **C7** Add `.submodules/nostos/AGENTS.md` documenting non-obvious
  invariants (always pass `--reinstall` when re-flashing; always run
  `nostos secrets test tailscale` before `node install`; required
  sequences; exit codes; idempotency guarantees).
- **C8** **DEFER to v0.4** — MCP server surface (same business logic,
  JSON-RPC over stdio). Not built in v0.3.

### Stream D — `nostos dashboard` TUI (READ-ONLY MVP)

Bubble Tea v2 dashboard at `internal/cli/dashboard/`. Single window,
split-pane, live-refreshing, with `--once --output json` headless mode
for cron and CI. **v0.3 framing: live status board, not autopilot.**
Action-handler dispatch (`r` reinstall / `d` delete / `i` identify) is
deferred to v0.4 — v0.3 reads, names, hides, filters, searches, shows
help, and renders living docs.

- Discovery: ARP+ICMP sweep on the configured `/24`, mDNS for
  `_workstation._tcp` and `_smb._tcp`, Talos maintenance API probe on
  TCP 50000, Tailscale device list (existing OAuth backend), ArgoCD
  Application list via the kubeconfig, and BMC discovery for the
  *configured* BMC host only (no internal-IP scanning).
- Match: bind discovered Devices to configured Nodes by MAC, then IP,
  then Tailscale 100.x address. Three buckets: `known`, `orphan`
  (configured but missing on net), `unknown` (on net but not in
  config).
- Health: per-cluster (etcd quorum, k8s api, Tailscale online count,
  ArgoCD synced+healthy), per-node (ICMP, Talos apid, Talos version,
  kubelet Ready, Tailscale registered, schematic match), per-app
  (ArgoCD synced+healthy, chart version vs upstream). The check
  registry is **hardcoded** in v0.3; a plugin model is deferred.
- Refresh tiers (collapsed from 4 → 2): **fast** (5s, liveness:
  ICMP / apid / Tailscale presence / k8s api) and **slow** (5min,
  upstream version diff). UI keystrokes process synchronously in
  the Bubble Tea event loop; no separate "fast UI tier".
- Diff with upstream: cached in `~/.cache/nostos/upstream-versions.json`
  (24h TTL). Talos releases via factory.talos.dev, Helm charts via
  OCI registry HEAD, container images via registry HEAD. (No general
  dashboard-state snapshot — Bubble Tea cold start is fast enough.)
- **Aggregate states (5):** `ALL_GREEN`, `DEGRADED`, `BROKEN`,
  `UNCONFIGURED` (zero nodes in config), `TRANSITIONING` (an install /
  reflash is in flight per per-node flock state).
- **Empty-cluster CTA:** when `UNCONFIGURED`, the body is replaced by
  a four-step checklist: `nostos init` → `nostos secrets test
  tailscale` → plug a node in → press `n` on the discovered MAC. Not
  a green bar.
- **Missing kubeconfig:** top-level warning row; probes that do not
  need kubeconfig still run; never silently skip.
- **Top-bar imperative line:** when state is `DEGRADED` or `BROKEN`,
  the top bar carries one human sentence ("Press G to fix the most
  pressing issue" — v0.3 `G` opens the relevant guide section,
  read-only; v0.4 wires it to dispatch).
- **Symbols (colorblind-safe):** `✓ ⚠ ✗ ?` plus bracket variants
  `[OK] [WARN] [FAIL] [?]` under `NO_COLOR=1` / non-TTY / `--ascii`.
- **Read-only action set (v0.3):** `[n]ame` (unknown rows; emits
  config patch on stdout — no write), `[H]ide` (capital H; toggles
  visibility of operator-marked devices), `[s]etup-info` (living
  docs), `[u]pgrade` (preview-only via `cluster upgrade --dry-run`),
  `[/]search`, `[?]help`. The footer is **contextual** by selected
  row type: unknown row offers `[n]ame`; orphan row offers nothing
  (read-only); known/healthy row offers nothing in v0.3. `?` help
  is **curated** prose, not auto-generated from the schema.
- **Deferred to v0.4:** `[i]dentify` (when shipped, prefer Redfish
  blink → NIC packet flood → `tpi` UART, not the other order),
  `[r]einstall`, `[d]elete`, guided-fix dispatcher, MCP surface.
- Living docs: `s` opens a Glamour-rendered Markdown panel sourced
  from an embedded `embed.FS` keyed by `<vendor>-<model>` and merged
  with operator overlays at `nostos/docs/<vendor>-<model>.md`. v0.3
  ships defaults for **`dell-optiplex-3080m`** and **`turing-rk1`**
  only — the actual home-systems hardware. Other platforms render
  a "create one with `nostos docs init`" placeholder.

### Stream E — PXE+TPI operator guide

Single Markdown file at `/Users/yuri/Workdir/Yuri/home-systems/docs/nostos-guide.md`
(NOT inside `.submodules/nostos/`). v0.3 ships **install + recover
only** (sections 0-3 + 7 + 9): what this gets you, hardware
checklist, first-time setup, PXE+TPI flows, recovery scenarios,
reference (CLI + config schema + exit codes). Per-vendor playbooks
(Sections 5/6) and Tailscale OAuth deep-dive slip to v0.4.

Every command is copy-pasteable; every error message has a "what
it means" entry; "Why" boxes explain design choices. The guide
carries an explicit **"do not commit" list**: home-network IPs,
BMC default credentials, OAuth client IDs and secrets, MAC
addresses, `op://` vault paths, kubeconfig client certs.

## Capabilities

### New Capabilities

- `dashboard` — interactive TUI surface; lifecycle, refresh tiers,
  discovery layer, action contract, headless mode. See
  `specs/dashboard/spec.md`.
- `cli-machine-output` — JSON I/O, schema introspection, field masks,
  dry-run, structured errors, input hardening. See
  `specs/cli-machine-output/spec.md`.

### Modified Capabilities

- `provisioner` (from `nostos-v02-provisioners`) — adds
  `MaxWaitMaintenance() time.Duration` and a BMC pre-flight contract
  for providers that talk to a BMC. Backwards-compatible: default
  implementation returns 0 / no pre-flight. PXE provider unaffected.
- `tpi-provisioning` — image cache rewrites the TOFU race with stream
  hashing + atomic record-after-rename. Adds the BMC pre-flight
  (`ErrBMCUnreachable` / `ErrBMCAuth` / `ErrBMCVersion`). Default
  WaitMaintenance deadline becomes 30 min.
- `nostos-cli` — every command gains `--output json`, every mutation
  gains `--dry-run`, root command gains `nostos schema` and `nostos
  dashboard`. Errors become structured. The list of mutations gaining
  `--dry-run` is enumerated in tasks.md §3.

## Impact

- **Code added (sketch — implemented in follow-up PRs, not this
  change):**
  - `.submodules/nostos/internal/cli/dashboard/` — Bubble Tea v2 model,
    discovery probes, action dispatcher.
  - `.submodules/nostos/internal/cli/jsonio/` — shared `--output json`
    encoder, NDJSON streamer, `--fields` projector.
  - `.submodules/nostos/internal/cli/schema/` — reflection-driven
    schema builder, shared by `nostos schema` and `dashboard ?`.
  - `.submodules/nostos/internal/cli/dryrun/` — typed `Plan` value
    that mutations populate; default printer emits JSON.
  - `.submodules/nostos/internal/discovery/` — ARP, ICMP, mDNS,
    Talos maintenance probe, Tailscale list, ArgoCD list.
  - `.submodules/nostos/internal/provisioner/tpi/bmc_preflight.go` —
    TCP+HTTP probe + typed errors.
  - `.submodules/nostos/internal/provisioner/tpi/image_cache.go` —
    rewritten per A2 (stream hash, record-after-rename).
- **Code changed:**
  - `internal/cluster/orchestrate.go` — honor `MaxWaitMaintenance`.
  - `internal/cli/*.go` — every command wires through `jsonio`.
  - All taskfile recipes per Stream B.
- **Operator config:**
  - `nostos/config.yaml` unchanged.
  - New optional `~/.config/nostos/dashboard.toml` for hidden devices
    + UI prefs (XDG; falls back to
    `nostos/state/dashboard.toml` only when XDG_CONFIG_HOME is unset
    AND the operator has no `~/.config`). See design D-Decision-4.
  - New `~/.cache/nostos/upstream-versions.json` (24h TTL).
- **Replaced flows:** `taskfiles/turing.yml::flash` /
  `download` / `install-talos` / `get`; `taskfiles/talos.yml::apply`
  for tp1/tp4. (`vm-pc01` + `pc01` survive — no provider in v0.3.)
- **Runtime externals (new):** none beyond v0.2. Dashboard reuses
  the existing `kubectl` / `talosctl` / `tpi` binaries via the
  `Commander` seam.
- **Docs:**
  - `docs/nostos-guide.md` (NEW; canonical operator guide).
  - `.submodules/nostos/AGENTS.md` (NEW; agent invariants).
  - `nostos/docs/<vendor>-<model>.md` (NEW; per-platform playbooks).
  - `nostos/README.md` updated to point at the guide.

## What This Is Not (Non-Goals for v0.3)

- **Not an MCP server.** Stream C8 explicitly defers to v0.4. The v0.3
  CLI is the contract.
- **Not new provisioners.** No `redfish`, `proxmox`, `usb`,
  `rpi-imager`. They keep their separate openspec changes
  (v0.2 non-goals 7.1-7.4).
- **Not an autopilot.** v0.3 dashboard is a *read-only status board*.
  Action dispatch (`[r]einstall`, `[d]elete`, `[i]dentify`) and the
  guided-fix dispatcher land in v0.4 against the same dispatch seam.
- **Not a Tailscale device cleaner.** B4 deferred to v0.4 (security).
- **Not `cluster upgrade`.** The dashboard's `u` action shows a preview
  diff and exits; no actual upgrade. Mutation lands in v0.4.
- **Not multi-cluster.** Dashboard reads exactly one kubeconfig context
  and one nostos `config.yaml`.
- **Not a credential vault, not a Tailscale-authkey rotator.** Same
  posture as v0.2.
- **Not a Kubernetes resource manager.** ArgoCD remains the GitOps
  surface; nostos only reads its state.
- **Not 4-vendor playbook coverage.** v0.3 ships playbooks for the
  two boxes actually in the lab (`dell-optiplex-3080m`,
  `turing-rk1`). `generic-amd64` and `raspberry-pi-5` slip to v0.4.
