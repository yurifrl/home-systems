## ADDED Requirements

### Requirement: `nostos dashboard` SHALL provide a single-pane interactive READ-ONLY cluster surface

The system SHALL provide an interactive Bubble Tea v2 dashboard at
`nostos dashboard` that displays cluster state, node state, ArgoCD
application state, and discovery results in a single split-pane layout.
The dashboard is a **live status board, not an autopilot**: in v0.3 it
MUST NOT shell out, MUST NOT mutate cluster state, and MUST NOT write
to disk except for `~/.config/nostos/dashboard.toml` (operator-private
UI prefs). Mutating action handlers (`r`, `d`, `i`, capital `G`) ship
in v0.4 against the same dispatch seam used by the CLI.

#### Scenario: Default invocation opens a live dashboard

- **WHEN** an operator runs `nostos dashboard`
- **THEN** the terminal switches to alternate-screen mode
- **AND** the dashboard renders an initial frame from in-process state
  (no on-disk dashboard-state cache; Bubble Tea cold-start is fast
  enough by itself)
- **AND** background probes refresh the model on the documented
  cadence (**fast=5s** for liveness checks; **slow=5min** for
  upstream-internet diff and slow cluster checks)
- **AND** pressing `q` exits and restores the prior terminal state

#### Scenario: Dashboard surfaces one of five aggregate states

- **WHEN** the dashboard renders
- **THEN** the top bar shows exactly one of `ALL_GREEN`, `DEGRADED`,
  `BROKEN`, `UNCONFIGURED`, or `TRANSITIONING`
- **AND** `UNCONFIGURED` is set when `len(config.nodes) == 0`
- **AND** `TRANSITIONING` is set when any per-node flock is held by
  an in-flight `node install` / `cluster upgrade`
- **AND** `BROKEN` is set when any check returns severity `error`
  AND no flock is held on the affected node
- **AND** `DEGRADED` is set when any check returns severity `warn`
- **AND** `ALL_GREEN` is set only when every check returns success
  AND there are configured nodes AND there are no orphan or unknown
  devices
- **AND** an empty cluster MUST NOT render as `ALL_GREEN`

#### Scenario: Empty cluster shows an explicit CTA

- **WHEN** `len(config.nodes) == 0`
- **THEN** aggregate state is `UNCONFIGURED`
- **AND** the body is replaced by a four-step checklist:
  (1) `nostos init`, (2) `nostos secrets test tailscale`, (3) plug a
  node in, (4) press `n` on the discovered MAC
- **AND** the dashboard does NOT render a green bar

#### Scenario: Missing kubeconfig surfaces a top-level warning

- **WHEN** `$KUBECONFIG` is unset and `~/.kube/config` does not exist,
  OR the kubeconfig is unreadable
- **THEN** a top-level warning row above the inventory pane reads
  "kubeconfig not found — cluster checks disabled"
- **AND** probes that do not need kubeconfig (ARP, ICMP, mDNS,
  Tailscale, BMC, Talos maintenance) still run and contribute
  Results
- **AND** the dashboard MUST NOT silently skip kubeconfig-dependent
  checks; their `Result` carries `severity:warn,
  reason:"kubeconfig_unavailable"`

#### Scenario: Top-bar imperative line on degraded/broken state

- **WHEN** the aggregate state is `DEGRADED` or `BROKEN`
- **THEN** the top bar carries one human sentence pointing the
  operator at the worst failed check (e.g. "Press G to open the
  recovery section for: Tailscale OAuth not configured")
- **AND** in v0.3, capital `G` opens the relevant section of
  `docs/nostos-guide.md` read-only (mutation lands in v0.4)

#### Scenario: Symbols are colorblind-safe

- **WHEN** the dashboard renders any check or node status
- **THEN** the symbol set is `✓ ⚠ ✗ ?` for color-capable terminals
- **AND** under `NO_COLOR=1`, non-TTY output, or `--ascii`, the
  bracket variants `[OK] [WARN] [FAIL] [?]` are used instead
- **AND** color is never the sole carrier of state information

#### Scenario: Unknown devices are surfaced in the inventory pane

- **WHEN** discovery finds a device on the configured network with
  no matching entry in `nostos/config.yaml`
- **THEN** that device appears in the inventory pane with a `?`
  marker (or `[?]` under NO_COLOR)
- **AND** when the row is selected, the contextual footer shows
  `[n]ame`
- **AND** pressing `n` emits a config patch on stdout for the
  operator to copy/apply (v0.3 emits only; v0.4 writes)

### Requirement: Dashboard discovery SHALL bind devices to nodes by MAC, IP, then Tailscale address

The system SHALL run an array of probes (ARP, ICMP, mDNS, Talos
maintenance API, Tailscale device list, ArgoCD Application list, BMC
discovery scoped to the *configured* BMC host per node) and SHALL
aggregate results into typed `Device{IP, MAC?, Hostname?, Tailscale?,
Talos?, BMCRole?, DiscoveredAt, ProbeID}` values.

The matcher SHALL bind `Device` to configured `Node` in priority MAC,
then IP, then Tailscale-100.x address. Each device SHALL fall into
exactly one of `known`, `orphan`, `unknown`.

#### Scenario: ICMP fan-out respects the concurrency cap

- **WHEN** discovery runs ICMP across a `/24`
- **THEN** at most 32 probes are in flight simultaneously
- **AND** each probe times out after 1 second
- **AND** failures do not propagate as panics; they degrade the
  per-host result to `unreachable`

#### Scenario: BMC discovery is scoped and rate-limited

- **WHEN** the dashboard runs BMC discovery
- **THEN** probes are issued ONLY against `cfg.Nodes[<name>].BMC.Host`
  values explicitly declared in the operator config
- **AND** the dashboard MUST NOT walk a `/24` or any subnet for BMCs
- **AND** each `(host)` is probed at most once per 5 seconds (token
  bucket); on failure, backoff is 5s → 30s → 5min, capped
- **AND** internal-IP detection (RFC1918) is NOT a special case;
  the rule is uniformly "only the configured host"

#### Scenario: Hidden devices toggle uses capital H

- **WHEN** `~/.config/nostos/dashboard.toml` contains a MAC in
  `hidden_devices`
- **THEN** the dashboard does not render a row for that device by
  default
- **AND** pressing capital `H` (NOT lowercase `h`) toggles
  visibility of hidden devices
- **AND** the device still counts in totals when hidden

### Requirement: Dashboard health checks SHALL be a hardcoded registry keyed by `CheckID`

The system SHALL maintain a hardcoded registry of checks identified by
typed `CheckID` constants. Each check SHALL declare `Tier`, `Severity`,
`DocsAnchor`, and a pure `Run(ctx, *State) Result` function.

v0.3 SHALL NOT expose a plugin interface or any other extension seam
for checks. Reconsideration is deferred entirely to v0.4 with concrete
use cases.

The check tier set is exactly two values: **fast** (5 second cadence;
liveness probes — ICMP / apid / Tailscale presence / k8s api) and
**slow** (5 minute cadence; etcd quorum / ArgoCD sync / schematic
match / upstream version diff).

#### Scenario: `--once` runs every check exactly once

- **WHEN** `nostos dashboard --once --output json` is invoked
- **THEN** every registered check runs exactly once (no skips, no
  duplicates) and contributes a `Result` to the snapshot
- **AND** the process exits 0 once the snapshot is emitted, regardless
  of cluster health
- **AND** with `--exit-nonzero-on-broken`, exit is 11 (network) or 13
  (conflict) keyed off the dominant failing check's class when the
  aggregate state is `BROKEN`

#### Scenario: Upstream-versions cache survives across runs

- **WHEN** the slow-tier upstream-version diff completes
- **THEN** the result is persisted to
  `~/.cache/nostos/upstream-versions.json` (24h TTL)
- **AND** the next run within 24h short-circuits the HTTP probes
- **AND** an offline laptop falls back to the last cache
- **AND** v0.3 SHALL NOT persist any other dashboard state to disk
  (no `dashboard-state.json`)

### Requirement: v0.3 dashboard action set SHALL be read-only with a contextual footer

The system SHALL expose only the following keybindings in v0.3, and
the contextual footer SHALL change per selected row type.

| Key | Behavior                                                      |
|-----|---------------------------------------------------------------|
| n   | Emit a config patch on stdout for an unknown row (no write)   |
| H   | Toggle hidden-device visibility (capital H; no collision)     |
| s   | Open the embedded living-docs panel for the selected row      |
| u   | Run `nostos cluster upgrade --dry-run` and show the preview   |
| /   | TUI-local filter                                              |
| ?   | TUI-local **curated** help (NOT auto-generated from schema)   |
| G   | Open relevant section of `docs/nostos-guide.md` read-only     |
| q   | Quit                                                          |

Contextual footer per selected row type:
- unknown row → `[n]ame`
- orphan row → read-only (no row-level action in v0.3)
- known/healthy row → read-only

The `[i]dentify`, `[r]einstall`, `[d]elete` keys and capital `G`
guided-fix dispatch are **deferred to v0.4**. When `[i]dentify`
ships, the implementation order SHALL be Redfish chassis-LED → NIC
packet flood → `tpi` UART (most visually unambiguous first).

#### Scenario: `?` help is curated, not derived from CLI schema

- **WHEN** the operator presses `?`
- **THEN** the help pane displays a curated, hand-written keymap
- **AND** the help pane does NOT enumerate every `nostos` CLI flag
- **AND** the keymap is decoupled from `nostos schema` output

#### Scenario: `u` upgrade is preview-only in v0.3

- **WHEN** the operator presses `u`
- **THEN** the dashboard runs `nostos cluster upgrade --dry-run` and
  shows the preview
- **AND** v0.3 SHALL NOT mutate cluster state from the `u` action
- **AND** the dispatch goes through `internal/cli/dispatch/`, the same
  seam v0.4 will use for mutating actions

### Requirement: Living-documentation pane SHALL render Markdown safely from `embed.FS` with optional operator overlay

The system SHALL render a Glamour-styled Markdown pane when the
operator presses `s` against a selected row. The base content is
sourced from an embedded `embed.FS` keyed by `<vendor>-<model>`.
Operator overlays at `nostos/docs/<vendor>-<model>.md` are merged on
read. Glamour SHALL be configured in **strict-ANSI mode** with raw-
HTML rejection. The embedded base MUST NOT be loaded from the
filesystem at runtime.

v0.3 SHALL ship exactly two embedded playbooks:
**`dell-optiplex-3080m`** and **`turing-rk1`** — the actual hardware
in this lab. Other vendors render a "create one with `nostos docs
init`" placeholder. `generic-amd64` and `raspberry-pi-5` are
deferred to v0.4.

#### Scenario: Missing playbook degrades gracefully

- **WHEN** the selected row's vendor/model has no embedded playbook
  AND no operator overlay
- **THEN** the pane displays "no playbook for this vendor; create one
  with `nostos docs init`"
- **AND** the dashboard does not crash

#### Scenario: ANSI injection in operator overlay is neutralized

- **WHEN** an operator overlay at `nostos/docs/<v>-<m>.md` contains
  raw ANSI escape sequences or raw HTML
- **THEN** Glamour's strict-ANSI mode strips ANSI control bytes
- **AND** raw `<script>`, `<iframe>`, and other HTML tags are
  rejected (not rendered, not silently dropped — visibly elided)
- **AND** the panel renders the safe subset

#### Scenario: Operator can edit an overlay in place

- **WHEN** the operator runs `nostos docs edit dell-optiplex-3080m`
- **THEN** `$EDITOR` opens against
  `nostos/docs/dell-optiplex-3080m.md`, creating from the embedded
  base if absent
- **AND** the embedded base file inside the binary is never modified

### Requirement: Headless `--once` mode SHALL emit a stable JSON snapshot

The system SHALL provide a non-interactive mode `nostos dashboard
--once --output json` that runs all check tiers serially, emits a
single JSON object, and exits. The shape SHALL include
`aggregate_state`, `kubeconfig_present`, `cluster`, `nodes[]`,
`checks[]`, `apps[]`, `upstream_diff`, `generated_at`. The shape is
part of the public contract and SHALL only change in major versions.

#### Scenario: `--fields` projection works in headless mode

- **WHEN** `nostos dashboard --once --output json
  --fields=aggregate_state,nodes.name`
- **THEN** the emitted JSON contains exactly those keys (with
  dot-notation projecting into arrays)
- **AND** unknown fields fail with `code=validation`, exit 10
