## Context

v0.2 (`openspec/changes/nostos-v02-provisioners/`) shipped the
`Provisioner` interface (`.submodules/nostos/internal/provisioner/`),
the `tpi` provider (`internal/provisioner/tpi/`), the PXE refactor
(`internal/provisioner/pxe/`), Scrubber-wrapped events
(`internal/provisioner/redact/`), per-node flock
(`internal/provisioner/flock/`), and a real test harness with
`Commander` / `Clock` seams.

v0.2 acceptance was met. End-to-end runs against tp1/tp4 surfaced four
defects (catalogued in v03-brief.md Stream A). Operators report that
the prose-only CLI plus the still-shipping `taskfiles/turing.yml`
recipes are now the dominant friction. AI agents driving the lab
through Claude Code report inability to discover flags, parse status,
or preview destructive operations.

This change closes those gaps. It does not add new provisioners and
does not introduce a new transport (MCP); both are kept on the v0.4
roadmap.

## Goals / Non-Goals

**Goals (v0.3):**
- Bug list from v0.2 closed (Stream A).
- `taskfiles/turing.yml` retired in favor of `nostos` (Stream B).
- Every `nostos` subcommand drivable from an AI agent: JSON output,
  schema introspection, dry-run on every mutation, structured errors
  (Stream C, principles 1-7 from `ai-friendly-cli/SKILL.md`).
- A live, interactive `nostos dashboard` (Bubble Tea v2) that doubles
  as live documentation, plus a `--once --output json` headless mode
  (Stream D).
- A canonical PXE+TPI operator guide at `docs/nostos-guide.md`
  (Stream E).

**Non-Goals (v0.3 — see proposal.md "What This Is Not"):**
- MCP server (v0.4).
- New provisioners (redfish/proxmox/usb/rpi-imager).
- Dashboard guided-fix dispatcher beyond 1:1 CLI mapping.
- `cluster upgrade` mutation; only preview in v0.3.
- Multi-cluster / multi-kubeconfig.

## Definitions

Terms added or sharpened in v0.3 (v0.2's glossary still applies):

- **BMC pre-flight**: the deterministic three-probe sequence
  (TCP-connect, TLS handshake, authenticated HTTP GET) executed by
  `tpi.Preflight` before any `tpi` subprocess is invoked. Returns one
  of `ErrBMCUnreachable` / `ErrBMCAuth` / `ErrBMCVersion` / nil.
- **MaxWaitMaintenance**: optional per-Provisioner default deadline
  for `WaitMaintenance`. Returns 0 if the provider has no opinion.
  The orchestrator picks `max(opts.WaitMaintenanceDeadline,
  prov.MaxWaitMaintenance())`. Operator `--wait-deadline` flag
  overrides both.
- **Headless dashboard**: `nostos dashboard --once --output json`. Runs
  every check tier exactly once, emits a single JSON snapshot to
  stdout, exits 0 on completion (snapshot may still describe a
  BROKEN cluster — exit code reflects "I ran", not cluster health).
  An optional `--exit-nonzero-on-broken` flag opts into the obvious
  CI semantics.
- **Refresh tier**: the dashboard polls in two tiered cadences —
  **fast** (5s; ICMP / apid / Tailscale presence / k8s api liveness)
  and **slow** (5min; etcd quorum, ArgoCD sync, schematic match,
  upstream version diff). UI keystrokes are processed synchronously
  in the Bubble Tea event loop — there is no separate sub-second UI
  tier and no 30s-vs-5m split. (Reduced from v0.3-draft 4 tiers per
  critic review.)
- **Action contract**: every dashboard keybinding maps to either (a)
  a read-only refresh / view, or (b) a config-patch emission to
  stdout (no write). v0.3 dashboard MUST NOT shell out and MUST NOT
  mutate cluster state. Mutating actions (`r`/`d`/`i`) ship in v0.4
  through the same dispatch seam used by the CLI.
- **Plan (dry-run)**: typed value populated by every mutation. JSON
  shape: `{command, target, would_execute:[{op, argv, env_keys,
  effect}], reversible}`. When `--dry-run` is set, the mutation
  populates the Plan, prints it, and returns **without invoking any
  subprocess** (`Commander` records ZERO calls). Re-running the same
  command without `--dry-run` MUST produce an actual execution
  sequence that is a (sub)sequence of the planned `would_execute`
  list. (Pinned per tests review §7.)
- **Aggregate state**: one of `ALL_GREEN`, `DEGRADED`, `BROKEN`,
  `UNCONFIGURED`, `TRANSITIONING`. `UNCONFIGURED` is set when
  `len(config.nodes) == 0`. `TRANSITIONING` is set when any per-node
  flock is held by an in-flight `node install` / `cluster upgrade`
  (a single offline node mid-reflash MUST NOT read as `BROKEN`).
- **Field mask**: `--fields=a,b,c` projection applied to JSON / NDJSON
  output. Unknown fields fail closed with a structured error naming
  the unknown field.

## Decisions

### D1. Per-provisioner WaitMaintenance deadline (Stream A1)

**Decision: per-provisioner `MaxWaitMaintenance() time.Duration`, not
a cluster-wide bump.**

Cluster-wide bump (raise the orchestrator default to 30 min) is
simpler but punishes PXE installs — `dell01` reaches maintenance in
~3 minutes; a 30 min ceiling means failure detection is a wall-clock
half-hour away when the BIOS is misconfigured. Per-provisioner is one
extra interface method and keeps PXE failure-fast.

Implementation:
- Add `MaxWaitMaintenance() time.Duration` to `Provisioner` interface
  (`internal/provisioner/provisioner.go`).
- Default-zero in PXE; 30 min in tpi.
- Orchestrator (`internal/cluster/orchestrate.go::Install`, the block
  around line 64-65) resolves the effective deadline as
  `max(opts.WaitMaintenanceDeadline, prov.MaxWaitMaintenance())`.
- The CLI flag `--wait-deadline` (added in this change) overrides
  both with a strict greater-than check, so an operator can shorten
  for debugging.
- Backwards-compat: existing v0.2 callers compile (interface
  extension; tests/fakes need a one-line method).

### D2. Image cache TOFU race (Stream A2)

**Decision: explicit two-phase commit — `*.part` temp file, atomic
`os.Rename`, then digest record written and `fsync`'d in the same
critical section. Temp files found at startup are garbage and SHALL
be deleted (no recovery / no resume).**

Flow per `Ensure`:

1. On entry, GC: delete any `*.part` in the cache dir that is not
   covered by an active flock. (No 24h heuristic — either it's
   under our flock, or it's garbage.)
2. If `<image>.raw.xz` exists AND `digests.json[<image>]` exists,
   recompute the sha256 of the file before returning the path. If
   the recomputed digest differs from the recorded digest, treat the
   record as untrusted, delete both file and record, fall through.
3. Acquire flock; download to `<image>.raw.xz.part`, stream-hashing.
4. Close + `fsync` the part file.
5. `os.Rename(part, final)`.
6. In the SAME flock and BEFORE returning, write the digest record to
   `digests.json.tmp`, `fsync`, `os.Rename` to `digests.json`.
7. Release flock.

Crash windows:

- Crash before rename (5): no final file exists; orphan `*.part`
  cleaned at next startup. No digest record was ever written.
- Crash between rename (5) and digest record (6): final file exists
  with no record. Next `Ensure` re-hashes (step 2 falls through to
  step 3? — no: with no record, step 2 deletes the orphan final and
  redownloads). The on-disk file with no record is treated as
  garbage. **No half-trusted file ever survives.**
- Crash after digest record write: steady state.

Rejected: any "resume from `*.part`" logic. The disk is cheap, the
logic is not. Critic review §4.10 + security review §2: a resume
path introduces a TOCTOU between byte-count check and hash check.

TOFU on **first** download remains gated behind a build tag (dev /
test only). Production keeps v0.2's "fail closed without a pinned
digest in `cfg.Cluster.ImageDigests`" stance.

Implementation reference points:
- `.submodules/nostos/internal/provisioner/tpi/image_cache.go:55-140`
  is rewritten end-to-end. The `digestStore` writeback moves into
  the post-rename block, behind a `fsync`+`Rename` of the record
  file itself.
- A property test (`tasks.md` 1.4) issues real `SIGKILL` against
  child processes performing downloads and asserts none of
  {final-without-record, mismatched-record, partial-final} ever
  survive.

### D3. BMC error clarity (Stream A3)

**Decision: pre-flight TCP+HTTP probe with three typed errors,
scoped strictly to the *configured* BMC host. No internal-IP or
LAN scanning. Probes are rate-limited.**

Security review §3 surfaced two abuse modes:
- A pre-flight that walks `host:443` for any host in `nostos/config`
  is a LAN-scanning fingerprint and may trip IDS rules on hardened
  networks.
- Repeated 401s against a BMC are a credential-stuffing signature.

Mitigations baked in:
- `PreflightBMC` accepts a single `host` string sourced exclusively
  from `cfg.Nodes[<name>].BMC.Host`. It MUST NOT iterate over a
  `/24` or any other range. Internal-IP detection (RFC1918) is NOT
  a special case — the rule is simply "only the configured host".
- Probes are rate-limited to **at most one BMC pre-flight per host
  per 5 seconds**, enforced via an in-process token bucket keyed by
  host. Repeated failures back off (5s → 30s → 5min, capped). The
  dashboard does NOT include a BMC pre-flight in either refresh
  tier; pre-flight runs only as part of `node install` / explicit
  `nostos node check <name>` (v0.4).
- BMC credentials never appear in scrubbed logs (existing v0.2
  Scrubber covers this; tests assert).

Implementation in `.submodules/nostos/internal/provisioner/tpi/bmc_preflight.go`:

```
func PreflightBMC(ctx context.Context, host, user, pass string) error
```

Three sequential probes; first failure short-circuits:

1. `net.DialTimeout("tcp", host+":443", 2s)` → on timeout/refused,
   `ErrBMCUnreachable{Host: host, Cause: err}`.
2. `tls.Dial(... InsecureSkipVerify=true)` then `http.Get("https://host/")`
   with basic auth → on 401/403, `ErrBMCAuth{Host}`. On other
   non-2xx, `ErrBMCUnreachable` (treats odd 5xx as "BMC is broken,
   not specifically auth").
3. Parse the BMC version string from the response (Turing Pi exposes
   `/api/bmc/info`; behind a feature flag we widen to Redfish later).
   If < minimum, `ErrBMCVersion{Got, Want}`.

Errors implement `Is`/`As` so `errors.Is(err, ErrBMCAuth)` works.
Each carries a stable `Code string` for the structured-error JSON
(see Stream C5):

- `ErrBMCUnreachable.Code = "bmc_unreachable"`
- `ErrBMCAuth.Code = "bmc_auth"`
- `ErrBMCVersion.Code = "bmc_version"`

The `tpi.Preflight` method (currently in `tpi/tpi.go`) calls
`PreflightBMC` BEFORE invoking `tpi --version`, so a wrong host fails
in the right place. The existing `Device not configured` errno from
the `tpi` binary is also intercepted: if the pre-flight passed but
`tpi flash` returns errno 6, we wrap it with `ErrBMCUnreachable`
plus "BMC dropped during flash; check power / network".

### D4. Where `dashboard.toml` lives (Stream D)

**Decision: XDG (`$XDG_CONFIG_HOME/nostos/dashboard.toml`, falling
back to `~/.config/nostos/dashboard.toml`).**

Alternatives considered:

- `nostos/state/dashboard.toml` (project-local): pulls per-operator
  UI state into git, conflicts on shared repos. Rejected.
- `~/.nostos/dashboard.toml`: violates XDG. Rejected.

The file holds **operator-private** preferences only:
- `hidden_devices = ["00:11:22:33:44:55", ...]`
- `last_view = "nodes" | "checks" | "apps"`
- `theme = "dark" | "light"`
- `expanded_node = "tp1"`

It does NOT hold cluster identity, secrets, or anything reproducible
from `nostos/config.yaml`. Loss of the file is non-fatal.

A `nostos dashboard config path` subcommand prints the resolved path
(driven by C2 schema for discoverability).

### D5. Dashboard checks: hardcoded registry (Stream D)

**Decision: hardcoded check registry in v0.3. The plugin / pluggable
model is deferred entirely — not even a "clean seam" — to v0.4.**

(Critic review §6: pick one. We picked hardcoded.) A plugin
architecture for ~12 builtin checks is a year of API churn to
solve a problem we don't have. v0.4 will redesign with concrete
use cases (third-party probe? per-tenant? per-ArgoCD-app?) rather
than speculating now.

Implementation:

```
type CheckID string
const (
    CheckEtcdQuorum     CheckID = "etcd_quorum"
    CheckK8sAPI         CheckID = "k8s_api"
    CheckTailscale      CheckID = "tailscale_online"
    CheckArgoSync       CheckID = "argo_sync"
    CheckTalosVersion   CheckID = "talos_version_drift"
    CheckSchematic      CheckID = "schematic_match"
    CheckImageDigests   CheckID = "image_digests_pinned"
    CheckPerNodeICMP    CheckID = "node_icmp"
    CheckPerNodeApid    CheckID = "node_apid"
    CheckPerNodeKubelet CheckID = "node_kubelet_ready"
    CheckPerNodeTSReg   CheckID = "node_tailscale_registered"
    CheckPerAppHealth   CheckID = "argo_app_health"
)

type Check struct {
    ID        CheckID
    Tier      Tier            // fast/medium/slow/very-slow
    Run       func(ctx, snap *State) Result
    Severity  Severity        // info/warn/error
    DocsAnchor string         // links into docs/nostos-guide.md
}
```

`internal/cli/dashboard/checks.go` registers each. The headless
`--once` runs every check exactly once and serializes `Result` per
check.

### D6. Schema subcommand vs `--describe` flag (Stream C2)

**Decision: subcommand `nostos schema [<command-path>]`, NOT a flag.**

A `--describe` flag conflicts with subcommand-level flags during
parsing (cobra treats it as a global flag, requiring synchronization
across every leaf command). A subcommand reflects the entire tree
once and emits a stable JSON shape. It also composes with `--output
ndjson` for streaming.

Output shape (informal):

```
{
  "command": "nostos node install",
  "summary": "...",
  "args":   [{"name":"name","type":"string","required":true,"validation":"node-name"}],
  "flags":  [{"name":"reinstall","type":"bool","default":false,"description":"..."},
             {"name":"output","type":"string","default":"text","enum":["text","json","ndjson"]}],
  "exit_codes": [{"code":0,"meaning":"success"},
                 {"code":2,"meaning":"validation"},
                 {"code":3,"meaning":"locked"},
                 {"code":4,"meaning":"node_already_ready"},
                 {"code":5,"meaning":"timeout"},
                 {"code":6,"meaning":"bmc_error"},
                 {"code":7,"meaning":"image_digest_mismatch"}]
}
```

`nostos schema` (no arg) emits the full tree as a JSON array.
`nostos schema --output ndjson` streams one command per line.

The schema is reflection-built from cobra metadata plus a tiny
side-table for `enum` and `validation` (because cobra does not
encode those). That side-table lives in
`internal/cli/schema/annotations.go`.

### D7. Discovery probe ownership and fan-out

Probes live in `internal/discovery/`:

- `arp.go` — reads `/proc/net/arp` on Linux, `arp -a` on macOS via
  `Commander`. ARP is passive — no L2 storms.
- `icmp.go` — concurrent fan-out, semaphore cap **32 in flight**.
  Uses `golang.org/x/net/icmp` with raw-socket fallback to
  unprivileged ICMP on Linux. Timeout per probe = 1s.
- `mdns.go` — wraps `github.com/grandcat/zeroconf` (MIT). Single
  query for `_workstation._tcp` + `_smb._tcp` over a 3s window.
- `talos_maint.go` — `talosctl --insecure -n <ip> version` via
  `Commander`, 2s timeout per IP.
- `tailscale.go` — reuses `internal/secrets/tailscale.go` OAuth client
  to call `GET /api/v2/tailnet/-/devices`.
- `argocd.go` — uses the in-cluster `kubectl` against the operator
  kubeconfig (`talos/kubeconfig` is what the repo ships, but the
  dashboard reads `KUBECONFIG`/`~/.kube/config` — the v0.3 guide is
  explicit about this).

Each probe returns `Device{IP, MAC?, Hostname?, Tailscale?, Talos?,
BMCRole?, DiscoveredAt time.Time, ProbeID string}`. The match layer
aggregates and dedups by MAC > IP > Tailscale-100.x.

### D8. Action contract and dispatch (Stream D)

**v0.3 ships READ-ONLY.** The dashboard MUST NOT shell out, MUST NOT
mutate cluster state, and MUST NOT write to disk except for the
local `dashboard.toml` (hidden devices, theme).

The dispatch seam (`internal/cli/dispatch/`) is built in v0.3 and
used only by the CLI. v0.4 wires the dashboard's `r`/`d`/`i` keys
to the same seam, guaranteeing CLI ↔ TUI ↔ (future) MCP all see the
same `Plan` (C4) and `Error` (C5) values.

Action mapping (v0.3 read-only set):

| Key | Behavior                                                      |
|-----|---------------------------------------------------------------|
| n   | Emit a config patch on stdout for an unknown row (no write)   |
| H   | Toggle hidden-device visibility (capital H; no collision)     |
| s   | Open the embedded living-docs panel for the selected row      |
| u   | Run `nostos cluster upgrade --dry-run` and show the preview   |
| /   | TUI-local filter                                              |
| ?   | TUI-local **curated** help (NOT auto-generated from schema)   |
| g   | Open the relevant section of `docs/nostos-guide.md` read-only |
| q   | Quit                                                          |

Contextual footer (changes per selected row type):
- unknown row → `[n]ame`
- orphan row → read-only (no row-level action in v0.3)
- known/healthy row → read-only

Deferred to v0.4 (with this exact ordering):
- `[i]dentify` — prefer **Redfish chassis-LED → NIC packet flood →
  `tpi` UART** in that order. (UX review §3.4: Redfish is the most
  visually unambiguous; UART is the fallback.)
- `[r]einstall` — dispatches `nostos node install --reinstall`.
- `[d]elete` — dispatches `nostos cluster cleanup --apply --device`
  with two-keystroke confirm.
- Guided-fix dispatcher behind capital `G`.

### D9. Per-vendor playbook structure

Playbooks ship as **`embed.FS`**-baked Markdown inside the binary
(security review §4: no filesystem read at runtime, no path-traversal
vector, no markdown-with-raw-HTML). Operator overlays at
`nostos/docs/<vendor>-<model>.md` are merged on read — the embedded
base always provides a known-good fallback. Glamour is configured
in **strict-ANSI mode** with raw-HTML rejection to neutralize
ANSI-injection from operator-edited overlays.

Each playbook is expected to contain these sections (the `s` action
renders whichever sections exist; missing sections degrade
gracefully):

```
# <Vendor> <Model>

## Hardware
## BIOS / firmware
## BMC / OOB (if applicable)
## nostos config snippet
## Recovery
## References
```

**v0.3 ships exactly two embedded playbooks:** `dell-optiplex-3080m`
and `turing-rk1` — the actual hardware in this lab. `generic-amd64`
and `raspberry-pi-5` are deferred to v0.4 with the rest of the
per-vendor surface. (Critic review §4.5: don't ship docs for
hardware we don't run.)

### D10. Dashboard idle behavior, cancel, and snapshot

- `tea.WithAltScreen` is mandatory. Quitting (`q`) restores the
  previous terminal state.
- Background commands run as `tea.Cmd`s that publish typed messages
  to the model's `Update`. No goroutine writes the model directly.
- **No general dashboard-state snapshot file.** Bubble Tea cold-start
  is fast enough that a 100 ms snapshot-replay path is theatre, not
  UX (critic review §4.4). The ONLY persisted artifact is
  `~/.cache/nostos/upstream-versions.json` (24h TTL), which is a
  network-cost optimization for the slow-tier internet diff — not a
  UI cold-start optimization.
- Headless `--once` short-circuits `tea.Program` entirely: build
  state, run all check tiers serially, marshal, exit.
- **Empty cluster** (`len(config.nodes) == 0`): aggregate state is
  `UNCONFIGURED`; body is replaced by the four-step CTA from the
  proposal.
- **Missing kubeconfig**: a top-level warning row sits above the
  inventory pane ("kubeconfig not found at $KUBECONFIG /
  ~/.kube/config — cluster checks disabled"). Probes that do not
  need kubeconfig (ARP, ICMP, mDNS, Tailscale, BMC, talos
  maintenance) still run.

### D11. Cluster cleanup safety (Stream B4) — DEFERRED to v0.4

B4 (`nostos cluster cleanup` for offline Tailscale devices) is
deferred to v0.4. Security review §5 surfaced spoofing concerns
that are non-trivial to resolve in MVP scope:

- The OAuth tag-scoping model on Tailscale's side does not give us
  a per-device confirmation that the device record we are about to
  DELETE corresponds to hardware *we* control — an attacker who can
  enroll a device with a similar hostname and cycle it offline can
  borrow our DELETE for free.
- A two-keystroke confirm + an explicit operator-curated allow-list
  + a tagged-namespace check are needed before this is safe.

v0.3 documents the existing manual workflow (`tailscale logout`,
`tailscale device remove`, admin console deletion) in
`docs/nostos-guide.md` Section 7. v0.4 ships the automated path.

### D12. CLI exit code catalog

**Decision: a 6-entry catalog. nostos-specific codes use 10-19 to
avoid POSIX collision.** Tests review flagged that proposed code 8
collides with shells that reserve <10 for signal-derived exits, and
that a 12-entry catalog had no agent-side justification.

Pinned in `AGENTS.md` and the schema:

| Code | Meaning             | Notes                                       |
|------|---------------------|---------------------------------------------|
| 0    | success             | dry-run preview also returns 0; payload carries `"status":"preview"` |
| 1    | generic error       | reserved fallback; prefer a specific code   |
| 10   | validation          | input rejected, schema mismatch             |
| 11   | network             | unreachable host, DNS, TLS handshake        |
| 12   | auth                | BMC auth, OAuth, kubeconfig context wrong   |
| 13   | conflict            | flock held, node-already-ready (use --reinstall), digest mismatch |
| 64   | usage               | cobra default; preserved                    |

Dry-run no longer occupies its own exit code (was 8, now folded
into 0 + payload `status:"preview"`). Sub-causes (BMC unreachable
vs auth vs version, digest mismatch vs unpinned) live in
`details.code` of the structured error and in the schema's per-
command exit-code table; they do NOT each get a unique top-level
exit number.

## Testing strategy

Lift the v0.2 strategy. Three seams already exist (`Commander`,
`Clock`, `secrets.Resolver`) plus the compliance suite at
`internal/provisioner/provisionertest/`. v0.3 additions:

- New `internal/discovery/discoverytest/` fakes for ARP / mDNS / etc.
- Bubble Tea v2 model tests via the model's pure `Update(msg) ->
  (Model, Cmd)` shape. No real terminal in CI.
- Headless dashboard golden test: run `--once` against a fake
  discovery + fake checks, assert the JSON snapshot.
- `image_cache_test.go` — kill-9 property test (see D2).
- `bmc_preflight_test.go` — table-driven tests over `httptest.Server`
  for unreachable / 401 / 200-but-old-version.
- `schema_test.go` — assert every cobra leaf command has a schema
  entry; assert every flag has a description; assert `enum` and
  `validation` annotations cover every typed flag.

Tier 1 / Tier 2 / Tier 3 split from v0.2 design D-Tests carries
over. v0.3 adds a Tier 1 "schema is exhaustive" gate.

## Open questions

- **Q1.** Default value for `MaxWaitMaintenance` on tpi: 30 min is
  conservative for RK1 in known-good firmware; should we publish a
  per-firmware-version map? **RESOLVED-DEFER:** measure during
  implementation PRs; v0.4 publishes the map if measurements
  warrant.
- **Q2.** Dashboard auth: what does the dashboard do when
  `kubeconfig` is missing or the operator's context is wrong?
  **RESOLVED:** top-level warning row, never silent skip; do NOT
  mirror the kubectl-context guard inside nostos (trust the operator
  wrapper documented in CLAUDE.md). See D10.
- **Q3.** mDNS dependency: `github.com/grandcat/zeroconf` is MIT but
  bumps the Go binary by ~1.5 MB. **RESOLVED:** acceptable; pin
  during PR review.
- **Q4.** Dashboard `g`-mode "fix it" buttons in v0.3 vs v0.4.
  **RESOLVED:** v0.3 dashboard is read-only. `g` opens the relevant
  guide section; mutation lands in v0.4. Confirmed in Non-Goals.
- **Q5.** ICMP raw-socket privilege on macOS. **RESOLVED:** macOS
  allows unprivileged ICMP via SOCK_DGRAM since 10.14; Linux needs
  `net.ipv4.ping_group_range` sysctl. Documented in the guide; do
  NOT silently fall back to ARP-only.
- **Q6.** `cluster cleanup` vs `secrets keys revoke` boundary.
  **RESOLVED-DEFER:** B4 deferred to v0.4 per D11; the boundary is
  redrawn when v0.4 lands.
- **Q7.** Headless dashboard exit code on BROKEN. **RESOLVED:**
  `--once` defaults to exit 0 ("I ran"); `--exit-nonzero-on-broken`
  opts CI users into exit 11 (network) or exit 13 (conflict)
  depending on dominant failure class. Cluster health is data, not
  exit code.
- **Q8.** `--fields` on `dashboard --once`. **RESOLVED:** yes,
  pinned in spec.
- **Q9.** Per-vendor docs location. **RESOLVED:** embedded in the
  binary via `embed.FS` (security review); operator overlays at
  `nostos/docs/<vendor>-<model>.md` are merged on read; the
  submodule keeps `AGENTS.md` only. v0.3 embeds two playbooks
  (`dell-optiplex-3080m`, `turing-rk1`); v0.4 adds the rest.

## D-Roadmap

- **v0.4** — Dashboard action handlers (`r`/`d`/`i` keys + guided-fix
  `G` mode) wired to the dispatch seam; B4 Tailscale cleanup with
  allow-listing + two-keystroke confirm; per-vendor playbooks
  (`generic-amd64`, `raspberry-pi-5`) plus operator-guide Sections
  5/6 (vendor playbooks, Tailscale OAuth deep-dive); `cluster
  upgrade --to <ver>` mutation; `secrets rotate`; MCP server (Stream
  C8) over the same dispatch seam; `redfish` provider as its own
  openspec change.
- **v1.0** — Stable CLI surface, vendored iPXE, Homebrew tap,
  container image, hardware test matrix (PXE + tpi nightly), 6+
  vendor playbooks, man pages.
