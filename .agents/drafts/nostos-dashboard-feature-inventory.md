# nostos Dashboard — Feature Inventory (for rebuild)
A complete record of what the deleted `nostos dashboard` did, so we can start over with a clean design.

> Note: This is a draft to organize ideas and scope before implementation. Reconstructed from the package structure, `engine.go` signatures, embedded playbooks, and the schema/AGENTS descriptions observed before deletion. Items marked ❓ need confirmation against git history (`git show HEAD:<path>`).

## Goal
Capture every feature of the old dashboard so the rewrite loses nothing intended, and so we can decide explicitly what to keep, drop, or redesign.

## What it was
- One command: `nostos dashboard` — a **single-pane Bubble Tea v2 TUI** for "cluster + nodes + ArgoCD apps".
- Marked idempotent; `StdoutSchema` referenced `dashboard.snapshot`.
- Had a non-interactive escape hatch: **`--once --output json`** emits a single snapshot (for scripting/automation).

## Feature areas (by package)

| Package | Feature |
|---------|---------|
| `engine` | Orchestrates `BuildSnapshot(ctx, cfg, Options)` — assembles the whole dashboard state from discovery + health + upstream + config |
| `snapshot` | The aggregate data model: per-node state, checks, discoveries, upstream diff, `AggregateState`; had a JSON schema (`dashboard.snapshot`) |
| `discovery` (+ `mdns`) | Discovers nodes on the network via **mDNS** (finds machines not yet/again in config) |
| `match` | Reconciles discovered nodes against `config.yaml` — classifies **known** vs **unknown/orphan** ("zombie" nodes) |
| `health` | Per-node **health checks** (reachability, Talos/cluster status) |
| `upstream` | **Upstream Talos version diff** — compares each node's current version against latest available (`upstreamDiff(current, versions)`) |
| `playbooks` | Hardware-specific **embedded playbooks** (markdown) keyed by board type |
| `actions` | Actionable operations / **imperative next-step** suggestion (`imperativeFor(state, checks)` → what to do next) |
| `cache` | Caches snapshots (avoid re-probing every render) |
| `tui` | The interactive rendering layer (Bubble Tea v2 / Lipgloss v2) |

## Concrete capabilities (engine signatures)
- `BuildSnapshot(ctx, cfg, Options) → snapshot` — full state build
- `sortedKnown([]snapshot.Node)` — ordered list of known/configured nodes
- `appendOrphans(unknown, orphan []snapshot.Discovery)` — surface discovered-but-unconfigured nodes
- `upstreamDiff(current string, versions) → snapshot.UpstreamDiff` — version drift per node
- `imperativeFor(state, checks) → string` — "what should the operator do next" hint

## Embedded playbooks (hardware guides)
- `dell-optiplex-3080m.md` (dell01 / amd64 control plane)
- `generic-amd64.md`
- `raspberry-pi-5.md`
- `turing-rk1.md` (tp1 / tp4 / arm64 workers)

## Inferred view (single pane showed)
- Cluster summary + health
- Per-node rows: configured nodes, their version, health, and upstream drift
- Discovered/orphan ("zombie") nodes not in config
- An imperative "next action" line
- (ArgoCD apps surface per the schema description) ❓ confirm what ArgoCD data was shown

## Open Questions
- ❓ Exact ArgoCD integration — what app data/states were displayed?
- ❓ Interactive actions — could you trigger installs/cleanup from the TUI, or read-only?
- ❓ Keybindings and layout of the single pane
- ❓ Refresh/poll cadence and caching TTL
- ❓ Which fields `--once --output json` emitted (the `dashboard.snapshot` schema shape)
- ❓ `Options` fields passed to `BuildSnapshot`

## Rebuild Notes
- Reuse the new `internal/upgrade` planner/catalog/TUI patterns (Bubble Tea v2) for consistency.
- mDNS discovery + config matching is the distinctive bit — decide if the rewrite keeps live discovery or goes config-only.
- Playbooks were embedded markdown — decide if they stay in the dashboard or move to a `docs`/`playbook` command.
- To fully verify any ❓ before rebuild: `git show HEAD:.submodules/nostos/internal/dashboard/<file>`.
