## 1. Stream A — v0.2 bug fixes

- [ ] 1.1 Add `MaxWaitMaintenance() time.Duration` to the `Provisioner`
  interface in `.submodules/nostos/internal/provisioner/provisioner.go`
  (additive). Default zero in PXE; 30 min in tpi. Acceptance:
  `internal/provisioner/provisionertest/compliance.go` table-driven
  test asserts both providers' values; orchestrator unit test asserts
  the effective deadline is `max(opts.WaitMaintenanceDeadline,
  prov.MaxWaitMaintenance())`.
- [ ] 1.2 Honor the per-provisioner deadline in
  `internal/cluster/orchestrate.go::Install` (replace the literal at
  the `withDefaults` block, lines ~64-65, with a `max(...)` call
  after the provisioner is constructed). Acceptance: tp1 install
  end-to-end with no `--wait-deadline` flag completes without manual
  `talosctl apply-config --insecure`.
- [ ] 1.3 Add `--wait-deadline <duration>` flag on `nostos node install`.
  Override wins. Acceptance: `nostos node install tp1 --wait-deadline
  5m` exits with `ErrTimeout` (exit code 5) within 5 min on a node
  intentionally not powered.
- [ ] 1.4 Rewrite `internal/provisioner/tpi/image_cache.go` per design
  D2: stream-hash on download into `*.part`, atomic `Rename`, write
  `digests.json` ONLY after rename returns nil, re-hash on cache hit
  before trusting the recorded digest. Acceptance: property test
  (≥100 iterations) sends `SIGKILL` to a child during download;
  parent observes no `*.raw.xz` and no `digests.json` write; only
  orphan `*.part` allowed. GC of `*.part > 24h` runs at the start of
  every Ensure.
- [ ] 1.5 New `internal/provisioner/tpi/bmc_preflight.go` exposing
  `PreflightBMC(ctx, host, user, pass) error` per design D3. Three
  typed errors `ErrBMCUnreachable` / `ErrBMCAuth` / `ErrBMCVersion`
  with stable `Code` strings. Acceptance: table-driven test over
  `httptest.Server` covers unreachable, 401, 200-but-old-version,
  200-current. Each maps to its expected typed error.
- [ ] 1.6 Wire `PreflightBMC` into `tpi.Preflight` BEFORE invoking
  `tpi --version`. Acceptance: against a bogus host, the resulting
  error names "BMC at <host> unreachable" not the OS errno; fixture
  test asserts `errors.Is(err, ErrBMCUnreachable)`.
- [ ] 1.7 Intercept errno 6 from in-flight `tpi flash` after a passing
  pre-flight; wrap with `ErrBMCUnreachable` and a "BMC dropped during
  flash" detail. Acceptance: FakeCommander scripts errno 6 from
  `flash`; resulting error is `ErrBMCUnreachable` with detail
  containing "during flash".
- [ ] 1.8 Pin a regression test for v0.2 Bug #2 (`.yaml` empty filename
  for tpi nodes). Acceptance: loading `nostos/config.yaml` with the
  `tp1` entry yields a non-empty rendered filename whose basename
  matches `tp1.yaml`.

## 2. Stream B — Hygiene

- [ ] 2.1 Replace `taskfiles/turing.yml::flash`, `download`,
  `install-talos`, `get` recipes with bodies that print
  `deprecated: use 'task nostos:install NODE=<name>'` and `exit 1`.
  Acceptance: `task turing:flash` exits 1 with the message; CI grep
  asserts no recipe under `turing:` shells `tpi flash` or `talosctl
  apply-config` directly.
- [ ] 2.2 Replace `taskfiles/talos.yml::apply` lines for tp1 / tp4 with
  thin wrappers around `nostos node install`. Leave the `vm-pc01`
  line and `pc01-install` recipe untouched (no provider yet).
  Acceptance: `task talos:apply` no longer runs `talosctl
  apply-config` against 192.168.68.107 / .114; runs `nostos node
  install tp1 && nostos node install tp4` instead.
- [ ] 2.3 Document `kubectl delete node talos-76w-r75` as a one-shot
  cleanup step in `docs/nostos-guide.md` Section 7 (Recovery).
  Acceptance: `grep talos-76w-r75 docs/nostos-guide.md` returns at
  least one hit, in the recovery section.
- [ ] 2.4 **DEFERRED to v0.4 — see §7.13.** `nostos cluster cleanup`
  for Tailscale offline devices. Security review surfaced
  device-spoofing concerns that require allow-listing + tagged-
  namespace checks before mutation is safe. v0.3 documents the
  manual `tailscale device remove` workflow in `docs/nostos-guide.md`
  Section 7 only.

## 3. Stream C — AI-friendly CLI

- [ ] 3.1 Create `internal/cli/jsonio/` with `Encoder`, `NDJSONStreamer`,
  and `FieldMask`. Add `--output {text,json,ndjson}` global flag on
  the root command. Acceptance: `nostos status --output json` emits
  one JSON object; `nostos node list --output ndjson` emits one
  object per line; `--fields=name,ip` projects; unknown field fails
  with structured error code `validation` (exit 10).
- [ ] 3.2 Add `--output json` paths to: `status`, `node list`, `node
  show`, `node install` (event stream as NDJSON), `secrets test`,
  `dashboard --once`. (`cluster cleanup` deferred per 2.4.)
  Acceptance: schema test (3.4) catches any leaf command without an
  `output` flag entry.
- [ ] 3.3 Create `internal/cli/dryrun/` with typed `Plan` value per
  design "Definitions". Plumb `--dry-run` through every mutation:
  `node install`, future `cluster cleanup`, future `cluster upgrade`,
  future `secrets keys revoke`. **Acceptance (per tests review §7):**
  `nostos node install tp1 --dry-run --output json` emits a `Plan`
  JSON with `would_execute: [...]` listing every subprocess that
  WOULD run (argv template + env keys, never values). FakeCommander
  records **ZERO** invocations. Exit code is **0** with payload
  `"status":"preview"` (NOT a dedicated dry-run exit). Property:
  re-running the same command without `--dry-run` produces an
  execution sequence that is a (sub)sequence of the planned
  `would_execute`.
- [ ] 3.4 New `nostos schema [<command-path>]` subcommand at
  `internal/cli/schema/` per design D6. Reflection-built; side-table
  for `enum` / `validation`. Acceptance: `nostos schema --output
  json` emits an array of every leaf command with flags, args, exit
  codes; round-trip test asserts every cobra leaf has an entry.
- [ ] 3.5 `--fields=a,b,c` projection on `list` / `show` / `dashboard
  --once`. Acceptance: `nostos node list --fields=name,ip` returns
  objects with exactly those keys; `--fields=bogus` fails with
  structured error code `validation` (exit 10).
- [ ] 3.6 Structured errors. Define `internal/cli/errs/` with `Error{
  Code, Message, Details, Hint}`. Top-level cobra error handler
  emits JSON to stdout when `--output json`, prose to stderr
  otherwise. Hints under `--output json` go in the JSON `hint`
  field on stdout (stderr empty); under `--output text` hints go to
  stderr. Exit codes per design D12 (6-entry catalog, 10-19 range).
  Acceptance: `nostos node install nonexistent --output json` emits
  `{"error":true,"code":"validation","message":"...","details":{...}}`
  on stdout, exit 10.
- [ ] 3.7 Input hardening. Reject **all** ASCII control chars
  (0x00-0x1F + 0x7F) in node names, field-mask names, and any
  user-supplied string; node names additionally constrained to
  `^[a-z0-9][a-z0-9-]{0,62}$`. Reject `--config` paths containing
  `..` segments after lex-cleaning OR resolving (after symlink
  resolution) outside the operator's home or the repo root. Reject
  embedded query parameters and fragments in `op://` refs. Reject
  YAML inputs whose anchors resolve to filesystem paths. Acceptance:
  each rule has a unit test in `internal/config/validate_test.go`;
  4 fuzz targets (FuzzNodeName / FuzzOpRef / FuzzConfigPath /
  FuzzFieldMask) at 5s budget each find no ASCII bypass.
- [ ] 3.8 Create `.submodules/nostos/AGENTS.md` documenting non-obvious
  invariants per brief Stream C7. Sections: required sequences,
  exit-code catalog (mirrors design D12), idempotency guarantees,
  "always pass --reinstall when re-flashing", "always run `nostos
  secrets test tailscale` before `node install` after editing
  secrets config", "run `nostos doctor` before any install on a new
  machine" (note: `doctor` is v0.4; AGENTS.md says so). Acceptance:
  the file exists, every documented invariant has a corresponding
  test or schema entry referenced inline.
- [ ] 3.9 (DEFERRED to v0.4) MCP server surface. Recorded in §7 below.

## 4. Stream D — `nostos dashboard` TUI (READ-ONLY MVP)

**Framing: live status board, not autopilot.** Action handlers
(`r`/`d`/`i` + guided-fix `G`) are deferred to v0.4 (§7.2 / 7.14).

- [ ] 4.1 Add `internal/cli/dashboard/model.go`: Bubble Tea v2 `Model`
  with `Update(msg) (Model, Cmd)` shape. Use `tea.WithAltScreen`.
  Acceptance: pure-`Update` table tests for keybindings (no real
  terminal); `?` opens help, `q` quits, `/` enters filter, arrows
  navigate the node list, capital `H` toggles hidden-device
  visibility (NOT lowercase `h`), capital `G` opens the relevant
  guide section read-only.
- [ ] 4.2 Layout per brief D1 with Lipgloss v2 + Bubbles v2 (table,
  viewport, textinput, help, spinner). Symbols use `✓ ⚠ ✗ ?` plus
  bracket variants `[OK] [WARN] [FAIL] [?]` under `NO_COLOR=1` /
  non-TTY / `--ascii`. Top bar carries one of 5 aggregate states
  (`ALL_GREEN`, `DEGRADED`, `BROKEN`, `UNCONFIGURED`, `TRANSITIONING`)
  plus an imperative line when DEGRADED/BROKEN. Acceptance: golden
  snapshot tests using *subsequence shape matchers* (per tests
  review §9; not byte-equal) over fixtures for each of the 5 states
  plus `NO_COLOR` rendering.
- [ ] 4.3 Discovery package `internal/discovery/` with `arp`, `icmp`,
  `mdns`, `talos_maint`, `tailscale`, `argocd`, `bmc` probes per
  design D7. Concurrency cap 32 in flight on ICMP; 1s per-probe
  timeout. **BMC probe scoped strictly to the configured BMC host
  per node — NO `/24` walking, NO internal-IP discovery.** BMC
  pre-flight rate-limited to one probe per host per 5s with
  exponential backoff on failure. Acceptance:
  `internal/discovery/discoverytest/` fakes drive a deterministic
  `Devices` slice; integration test on local loopback covers the
  live ICMP path with `t.Skip` if raw socket is refused; rate-limit
  test uses fake `Clock` to assert no second probe within 5s.
- [ ] 4.4 Match layer per brief D3 (MAC > IP > Tailscale-100.x).
  Buckets: `known` / `orphan` / `unknown`. Hidden-devices filter
  reads `~/.config/nostos/dashboard.toml` per design D4. Capital
  `H` toggles visibility. Acceptance: table-driven matcher test
  covers MAC-only / IP-only / Tailscale-only / collisions.
- [ ] 4.5 Health checks per design D5. **Hardcoded** registry keyed
  by `CheckID` (no plugin seam in v0.3). Two tiers: **fast** (5s)
  and **slow** (5min). Aggregate-state computation:
  - `UNCONFIGURED` if `len(config.nodes) == 0`.
  - `TRANSITIONING` if any per-node flock is held.
  - `BROKEN` if any check returns severity `error` AND no flock is
    held for the affected node.
  - `DEGRADED` if any check returns severity `warn`.
  - `ALL_GREEN` only when every check is success AND there are
    configured nodes.
  - **Empty cluster MUST NOT read as `ALL_GREEN`.**
  - **Missing kubeconfig surfaces a top-level warning row;** probes
    that don't need kubeconfig still run.
  Acceptance: each check has a stub + per-check unit test; `--once`
  runs every check exactly once and records `Result`; aggregate-
  state truth table covers all 5 states including the empty-cluster
  and missing-kubeconfig scenarios.
- [ ] 4.6 Diff with internet: `internal/dashboard/upstream/` with
  `~/.cache/nostos/upstream-versions.json` (24h TTL). Probes:
  factory.talos.dev/versions, OCI HEAD for charts, registry HEAD
  for images. Acceptance: cache hit short-circuits HTTP; expired
  cache refetches; offline laptop falls back to last cache.
  (No general dashboard-state snapshot file ships in v0.3 — only
  this upstream-versions cache. Critic review §4.4.)
- [ ] 4.7 Action dispatcher `internal/cli/dispatch/` shared by CLI
  (and v0.4 dashboard). v0.3 wires only the CLI surface; v0.4 wires
  the dashboard `r`/`d`/`i` keys to the same seam. Acceptance:
  FakeCommander script drives `nostos node install --reinstall tp1`
  through the dispatch seam; the seam returns the same `Plan`
  / `Error` shapes that v0.4 dashboard will consume. (No TUI-
  driven mutation in v0.3.)
- [ ] 4.8 Headless mode `nostos dashboard --once --output json` per
  design D-Definitions. Optional `--exit-nonzero-on-broken`.
  Acceptance: golden test against a fake `State` produces a stable
  JSON snapshot; default exit is 0 regardless of cluster health
  (payload carries the truth); `--exit-nonzero-on-broken` flips
  exit to 11 (network) or 13 (conflict) keyed off the dominant
  failing check's class. Snapshot includes `aggregate_state`,
  `kubeconfig_present` (bool), `cluster`, `nodes[]`, `checks[]`,
  `apps[]`, `upstream_diff`, `generated_at`.
- [ ] 4.9 **Removed.** No general dashboard-state snapshot file in
  v0.3 (was: `~/.cache/nostos/dashboard-state.json` for <100ms
  cold-start). Bubble Tea cold-start is fast enough on its own.
  See critic review §4.4 + D10.
- [ ] 4.10 Living-docs viewer: `s` action opens a Glamour-rendered
  Markdown panel sourced from an embedded `embed.FS` keyed by
  `<vendor>-<model>`, merged with operator overlay at
  `nostos/docs/<vendor>-<model>.md` if present. Glamour configured
  in **strict-ANSI mode** with raw-HTML rejection. Acceptance:
  missing playbook degrades gracefully ("create one with `nostos
  docs init`"); raw `<script>` tag in overlay is stripped, not
  rendered; ANSI escape sequences in overlay are neutralized.
- [ ] 4.11 Ship default playbooks **embedded** at
  `internal/cli/dashboard/docs/dell-optiplex-3080m.md` and
  `internal/cli/dashboard/docs/turing-rk1.md` (both in `embed.FS`).
  These are the only two boxes actually in this lab.
  `generic-amd64` and `raspberry-pi-5` are **deferred to v0.4**
  (§7.15). Acceptance: both files exist, are embedded, and contain
  the section headings the renderer expects.
- [ ] 4.12 `nostos docs edit <vendor>-<model>` opens `$EDITOR` against
  the operator-overlay path `nostos/docs/<vendor>-<model>.md`,
  creating from the embedded base if absent. `nostos docs init`
  creates a stub from template. Acceptance: argv assertion via
  `Commander`; written file is operator-editable, the embedded
  base is never touched.
- [ ] 4.13 **Deferred to v0.4.** Action handlers `r` (reinstall),
  `d` (delete), `i` (identify; ordering: Redfish chassis-LED → NIC
  packet flood → `tpi` UART). v0.3 dashboard MUST NOT shell out and
  MUST NOT mutate cluster state. Recorded in §7.14.

## 5. Stream E — Operator guide (install + recover only)

v0.3 ships **Sections 0-3 + 7 + 9 only**. Per-vendor playbooks
(§5) and Tailscale OAuth deep-dive (§6) are deferred to v0.4 along
with the rest of the per-vendor surface.

- [ ] 5.1 Create `docs/nostos-guide.md` with sections 0-3 + 7 + 9.
  TOC + section anchors. Include an explicit **"Do not commit"
  list** at the top: home-network IPs, BMC default credentials,
  OAuth client IDs and secrets, MAC addresses, `op://` vault paths,
  kubeconfig client certs. Acceptance: every command in the guide
  runs (smoke test extracts code blocks tagged `bash` and
  shellchecks them; does NOT execute against the cluster).
- [ ] 5.2 Section 0 (What this gets you) + Section 1 (Hardware
  checklist with BIOS settings for `dell-optiplex-3080m` and
  `turing-rk1` only). Acceptance: peer review checklist signed in
  the PR description.
- [ ] 5.3 Section 2 (First-time setup): `nostos init`,
  `nostos.yaml` walkthrough, secrets setup (1Password, OAuth).
- [ ] 5.4 Section 3 (PXE+TPI flow): every step from `nostos node
  install dell01` (PXE) and `nostos node install tp1` (TPI) to
  Ready, with sample output and what to expect at each stage.
  References design D3 BMC pre-flight errors inline.
- [ ] 5.5 **Deferred to v0.4 — §7.15.** Section 5 (per-vendor
  playbooks). v0.3 ships only the embedded `dell-optiplex-3080m`
  and `turing-rk1` content via the dashboard `s` action.
- [ ] 5.6 **Deferred to v0.4 — §7.15.** Cross-link plumbing for
  Section 5.
- [ ] 5.7 **Deferred to v0.4 — §7.15.** Section 6 (Tailscale OAuth
  deep-dive). v0.3 keeps Tailscale setup at the README/quickstart
  level inside Section 2.
- [ ] 5.8 Section 7 (Recovery): node won't boot, BMC unreachable,
  etcd quorum lost, Tailscale revoked all keys, manual cleanup of
  `talos-76w-r75` (per §2.3), manual `tailscale device remove`
  workflow (replaces deferred B4).
- [ ] 5.9 Section 8 (Dashboard): how `nostos dashboard` answers most
  of "what's broken" without reading the rest of the guide. Notes
  it is read-only in v0.3.
- [ ] 5.10 Section 9 (Reference): full CLI surface (auto-extract via
  `nostos schema --output json` and a small Markdown renderer),
  config schema, exit codes (mirror design D12 6-entry catalog,
  range 10-19), error catalogue. Acceptance: a doc-build step in
  `Taskfile.yml` regenerates the reference table from `nostos
  schema`; CI fails if the regenerated block diverges from what's
  checked in.

## 6. Tests

- [ ] 6.1 Compliance suite (`internal/provisioner/provisionertest/`)
  passes for `pxe` and `tpi` after the interface extension (1.1).
  Acceptance: green on both providers; no skips.
- [ ] 6.2 SIGKILL property test for `image_cache` (1.4). Acceptance
  per task.
- [ ] 6.3 BMC pre-flight table test (1.5). Acceptance per task.
- [ ] 6.4 Schema completeness gate: every cobra leaf command has a
  schema entry; every flag has a description; every `enum` /
  `validation` annotation is non-empty when present. Acceptance:
  CI Tier 1 step.
- [ ] 6.5 Dashboard golden snapshot tests (4.2). One per aggregate
  state.
- [ ] 6.6 Headless dashboard JSON golden (4.8).
- [ ] 6.7 Dispatcher contract test (4.7) — same FakeCommander script
  drives CLI and TUI through the same path.
- [ ] 6.8 Input hardening tests (3.7), including a 5s fuzz budget on
  node-name parsing.
- [ ] 6.9 Backwards-compat fixture: pin
  `nostos/config.yaml`. Acceptance: load → no error; tp1 / tp4 still
  parse; dell01 still parses with default `pxe`.
- [ ] 6.10 Integration test (build tag `integration && dashboard`)
  drives `--once --output json` against a kind-cluster + fake
  Tailscale. Nightly only.

## 7. Out of scope for v0.3 (each is its own future change)

- 7.1 MCP server surface (Stream C8) — v0.4. Same business logic,
  JSON-RPC over stdio. Reuses the `dispatch` seam (4.7).
- 7.2 Dashboard guided-fix dispatcher (capital `G` mode that does
  more than open the guide) — v0.4.
- 7.3 `cluster upgrade --to <ver>` mutation — v0.4. v0.3 ships the
  preview only.
- 7.4 `secrets rotate` (covers Tailscale authkey rotation) — v0.4.
- 7.5 `nostos doctor` (catalog + stub) — v0.4. AGENTS.md mentions it
  as the "before-install" check, but the implementation is v0.4.
- 7.6 Additional provisioners (`redfish`, `proxmox`, `usb`,
  `rpi-imager`) — separate openspec changes.
- 7.7 JSONL run log + `--resume` — v0.4 (held for tight v0.3 scope).
- 7.8 `inventory.db` (SQLite) — v0.4 (per v0.2 §7.6).
- 7.9 Drift detection + `nostos diff <node>` — v0.4 (per v0.2 §7.7).
- 7.10 Multi-cluster — post-v1.0.
- 7.11 Vendored iPXE / Homebrew tap / container image — v1.0.
- 7.12 Hardware test matrix expansion (>2 vendors) — v1.0.
- 7.13 `nostos cluster cleanup` for Tailscale offline devices
  (Stream B4) — v0.4. Security review surfaced spoofing concerns;
  ships with allow-listing + tagged-namespace check + two-keystroke
  confirm.
- 7.14 Dashboard mutating action handlers `r` (reinstall), `d`
  (delete), `i` (identify) — v0.4. v0.3 dashboard is read-only.
  When `i` ships, ordering is Redfish chassis-LED → NIC packet
  flood → `tpi` UART.
- 7.15 Per-vendor playbooks beyond `dell-optiplex-3080m` and
  `turing-rk1` (i.e., `generic-amd64`, `raspberry-pi-5`) and
  operator-guide Sections 5/6 — v0.4.
