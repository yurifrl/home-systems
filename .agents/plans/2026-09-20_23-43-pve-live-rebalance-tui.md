# Proxmox live-rebalance TUI — design plan (no code)

A precedence layer over Crossplane's `crossplane-proxmox` VMs: a TUI to start/stop VMs and rebalance CPU / memory / GPU across Proxmox hosts, with a canary-style lifecycle (apply live → promote to git → abort back to Crossplane).

## Context (facts this design stands on)

- Today: git (public chart + private values overlay) → ArgoCD → chart templates **raw `EnvironmentVM` MRs** (no XR/composition) → provider-proxmox-bpg → PVE API. Crossplane is the sole writer to PVE and reconciles continuously — any out-of-band change gets reverted.
- Crossplane annotations already in play: `crossplane.io/external-name` = bare vmid ("100"). ArgoCD `ignoreDifferences` already used (ISO URL field). GPU exclusivity currently guarded in git (`templates/00-guards.yaml` fails the render).
- Hosts are small and tight: pc01 = Ryzen 5 3400G (4C/8T), 15.57 GiB, iGPU `0000:01:00.0` exclusive. Rebalancing is a recurring manual dance (done by hand today, 2026-09-20).
- Memory/core changes on a running VM require a VM restart unless hotplug is enabled per-VM. Talos guest sees new memory only after reboot.
- nostos = Go CLI (`go run`, `--log-json`, state dirs, op:// secrets) for Talos node lifecycle. The cluster API is reachable from the workstation (`api.k8s.lan`), PVE API reachable directly (192.168.68.112:8006, creds already materialized via ESO `proxmox-creds`).

## The one design insight

Crossplane is a reconcile loop: `git → (chart/composition) → MR spec → provider → PVE`. An override that takes precedence **without being undone** must intervene at exactly one of three joints:

1. **Below the loop** — pause the MR, write PVE directly.
2. **Inside the loop** — feed a live delta into the composition so the loop itself computes the override.
3. **At the spec** — patch the MR and blind ArgoCD from reverting it.

Every other idea re-implements one of these. The three options below are exactly these three joints.

## Option A — Bypass lane ("canary gate"): pause + direct PVE writes

- TUI talks to the **PVE API directly** (same auth shape the provider uses). Before changing a VM it sets `crossplane.io/paused: "true"` on that VM's `EnvironmentVM`. Crossplane stops reconciling it → **cannot undo anything**.
- Change applies in one API call; the VM-level guard (GPU exclusivity, host budget) is enforced by the TUI from **live PVE state** (`/nodes/{node}/qemu` + config), which is more truthful than the git-side render guard.
- **Abort/fallback** = remove the annotation → next reconcile converges to git. Crossplane never fought, so no fight to unwind.
- **Promote** = TUI writes the new numbers into the private values repo and pushes (ArgoCD syncs), then un-pauses. Canary metaphor holds: apply = canary, promote = git write-back, abort = un-pause.
- Ready-made: `crossplane.io/paused` is a stable core annotation; Go client `github.com/bpg/proxmox` — the exact client the provider imports.
- Risk: while paused, Crossplane is blind to **all** fields of that VM (disks, ISO, cloud-init drift invisible). Mitigate: pause is a session-scoped state, TUI shows a loud "PAUSED" badge, and promote/abort clears it.
- Chart changes: **zero**. Works today with raw MRs.

## Option B — Spec overlay: patch the MR + ArgoCD ignoreDifferences

- TUI patches the MR spec fields (cpu/memory/started/onBoot/hostpci) in-cluster; Crossplane sees the new spec and executes it — single-writer preserved, audit in MR events.
- ArgoCD must be stopped from self-healing those fields back: `ignoreDifferences` jsonPointers on the crossplane-proxmox Application.
- **Verdict: advise against.** The blindness is structural and permanent: those fields never self-heal again, so a genuine drift in git (real config change) also stops converging silently. Abort is also awkward (un-ignoring + manual re-render), worse than A's single annotation. Listed for completeness; not the pick.

## Option C — Policy layer: Allocation object + composition functions

- Introduce a small XR (`ProxmoxVM`) + XRD + composition that emits the existing `EnvironmentVM` MRs (mechanical refactor of the chart; also gives one place for defaults and turns `00-guards.yaml` into composition-level validation).
- Live delta object: a `ConfigMap` (later: CRD) `proxmox-allocation` with `{vmid: {cores, memoryMiB, started, gpu}}`. Composition pipeline = **function-environment-configs** (pulls the ConfigMap) + **function-patch-and-transform** (overlay onto each VM; absent keys fall through to the XR's git values). Both ready-made, zero custom function code.
- Semantics are the cleanest of the three: git XR = baseline, Allocation = live delta, **delete the delta → baseline restored by the very next reconcile**. Crossplane can never "undo" it because the delta is part of its own desired-state computation. ArgoCD sees the XR in-sync (XR untouched) — zero fight anywhere, no permanent blindness.
- Cross-object policy (host budget Σ, GPU single-owner) can later move from the TUI into a tiny admission webhook — the only option where "precedence" is a real policy layer, not a switch.
- Cost: the XR refactor + two function revisions pinned in the chart. Crossplane v1.20 runs composition functions GA — no flag needed.

## Comparison

| Dimension | A pause+bypass | B patch+ignore | C allocation+functions |
|---|---|---|---|
| Who writes PVE | TUI directly | Crossplane | Crossplane |
| Abort/fallback | remove annotation (instant) | un-ignore + manual restore | delete delta (next reconcile) |
| Git-truth during override | stale (badge+promote mitigate) | stale, silently forever | stale but reconcilable |
| ArgoCD behavior | unchanged | permanently blind on those fields | unchanged, zero fight |
| Chart change today | none | small (ignoreDifferences) | XR refactor |
| Future policy layer | no | no | yes |
| Works for any PVE host w/o cluster | yes | no | no |

**Recommendation: C is the lab's end-state; A is the standalone tool's core mode and day-1 behavior; B never.** The standalone ships with the PVE-direct driver (works on any Proxmox with zero k8s), and the lab additionally gets the Crossplane driver.

## TUI design (charm-stack)

- Stack: **Bubbletea v2 + Bubbles + Lipgloss + Huh** (already the house stack), Go, runs via `go run` like nostos.
- Three panes: VM list (color by state: running green / stopped gray / paused amber / reboot-required badge) · per-host budget bars (cores/mem/GPU vs **caps auto-derived from the PVE API** `/nodes/{node}/status`, overridable in config) · detail form (Huh).
- The rebalance interaction is the core UX: since host Σ is fixed, the form **offers the donor** — "+2 cores needs −2 elsewhere: [give to ▾]"; memory same; **GPU is an exclusive radio** across the VMs of that host (current owner shown; changing owner warns that both VMs must be stopped — IOMMU can't hot-move).
- Keys: `j/k` move · `s` start/stop · `e` edit · `enter` diff+confirm · `p` promote · `u` un-pause/abort · `q`. Submit always shows a before/after diff; no silent writes.
- Reboot-awareness: reads live VM config; core/memory changes on a running VM show "reboot required" and offer stop/start now or apply-on-next-boot (the exact behavior hit today).
- Drivers behind one interface: `Read(host) → Apply(diff)`:
  - `pve-direct` (A): PVE API + pause annotation via kubeconfig.
  - `crossplane-envconfig` (C): writes the Allocation ConfigMap; guard rails still read PVE live state.
- Promote is a pluggable hook (`promote = "exec …"` → values-repo commit+push), defaults to printing the YAML block to paste if no repo configured.

## Standalone vs nostos — verdict

**Build standalone; add `nostos proxmox` as a thin passthrough shim.** Working name: `pvedial`.

- Charter: nostos is Talos **node** lifecycle and the recovery-critical bootstrap tool — you want it boring, minimal-dep, no interactive TUI code paths in rescue scenarios. VM shape-juggling is **host** lifecycle, different cadence, different audience.
- OSS surface: nobody owns a "deadly simple PVE rebalance TUI"; charm + bpg client make it a weekend project with real adoption. Features (budget bars, GPU lease, promote-to-git) are demo-able.
- Dependency direction: the standalone can adopt nostos conventions (go run, --log-json, state dir) without depending on it; nostos exec-shims it version-pinned like the other submodules. You still type `nostos proxmox`.
- Fair counterpoint: in-nostos means one binary, one credential flow, zero pinning dance. If this were strictly internal adhoc, that wins. But adhoc internal features are exactly what shouldn't live in a bootstrap tool — and the goal here is explicitly generic/plug-and-play.

## Credentials & config UX

- Auth ladder (first-match wins):
  1. `--endpoint https://pve.lan:8006 --token <user@realm!id=secret>` — PVE **API token**, recommended over `root@pam` (which is what the provider uses today); least-privilege custom role: `VM.PowerMgmt`, `VM.Config.CPU`, `VM.Config.Memory`, `VM.Config.HW`, `VM.Config.Disk`, `Datastore.Audit`, `Sys.Audit`.
  2. `--from-kube` — read the existing ProviderConfig's secret (`proxmox-creds` shape) so the lab needs zero new secret management.
  3. 1Password `op://` reference resolved via `op read` (nostos convention) or OS keychain via interactive `login` flow (gh-style).
- Config: `~/.config/pvedial/config.toml` — named profiles (endpoint, insecure, caps override, promote hook, driver). TUI lists **all nodes of the PVE cluster** from one endpoint (`/cluster/status`) — one connection string = plug-and-play for future hosts; budgets are **per host**, auto-derived from the API.
- Safety: dry-run diff by default; audit log `~/.local/state/pvedial/audit.log` (+ `--log-json`); k8s write path (annotations/ConfigMaps) scoped by a narrow Role.

## Ready-made inventory

`crossplane.io/paused` annotation · `crossplane.io/external-name` (=vmid) mapping · function-environment-configs · function-patch-and-transform · function-go-templating (alternative merge) · `github.com/bpg/proxmox` Go client · charm-stack (Bubbletea v2/Bubbles/Lipgloss/Huh) · PVE `pveum` API tokens · ArgoCD `ignoreDifferences` (why B is tempting and why it's a trap).

## Steps (implementation order, still no code)

1. Scaffold standalone repo (`pvedial`): Go + charm-stack, config.toml + auth ladder, PVE read-only mode (list VMs/nodes, live caps).
2. Rebalance UX: budget bars, Huh form with donor-selection, GPU radio, diff+confirm, reboot-required handling — applying via PVE API with a `--pause-via-kube` opt-in for Crossplane-managed VMs (option A).
3. Lab integration: switch `crossplane-proxmox` chart to XR+XRD+composition (option C) — XRD, XR instances replacing today's three templates, composition with env-configs+patch-and-transform reading `proxmox-allocation`; `pvedial` gains the `crossplane-envconfig` driver.
4. Promote flow: hook writes private values overlay, pushes, un-pauses/deletes delta; abort path documented (`nostos proxmox --abort` semantics = remove annotation / delete delta).
5. nostos shim: `proxmox` subcommand exec'ing pinned `pvedial`; AGENTS/README contract updates both sides.

## Verification

- Option C lab proof: create `proxmox-allocation` delta → MR spec changes within one reconcile → PVE reflects it; delete delta → reconcile returns to git values; ArgoCD app stays Synced throughout.
- Option A proof: pause annotation stops provider reconcile (edit MR while paused, observe no PVE change); unpause converges back to git.
- TUI proof: on pc01, +1 core to one VM offers −1 from the other; GPU radio refuses two owners; running-VM memory change shows reboot badge; audit log records each apply.
- Multi-host proof: second PVE node appears with its own budget bars from a single endpoint.

## Deep dive (2026-09-21): C vs A for this cluster

Facts gathered from the live repo/cluster that reshape the comparison:

- Crossplane is **v2.3.4**, XFN already enabled (`xfn.enabled: true`) — composition functions are the *only* composition mode in v2, so C aligns with the installed world.
- **Decisive structural fact:** today the chart renders raw `EnvironmentVM` MRs and the ArgoCD app tracks them directly with `selfHeal: true`. Any in-cluster spec mutation on those MRs is therefore option B (ArgoCD reverts it). The XR layer is not a nicety — **it is the only mechanism that moves the MRs out of ArgoCD's diff** (composed children are shown in the resource tree and health-checked, but not spec-diffed against git). A tiny controller patching raw MRs (C′) hits the same ArgoCD fight and collapses into B. C stands or falls with the XR refactor.
- The repo already uses the B mechanism twice, narrowly: iso-pipeline rotates the download URL, cloud-init-render patches EnvironmentFile — both resolved via `ignoreDifferences` on single leaf fields. B for allocation would mean permanently blinding `cpu/memory/started/onBoot/hostpci` on every VM — the most safety-critical fields. B is now doubly dead.
- **Control-plane fragility is a design input:** core runs `--max-reconcile-rate=1 --sync-interval=30m` with pods pinned to `192.168.68.91:6443` to dodge the flaky two-LAN ClusterIP VIP; documented lease-flap incidents (2026-08-15). Implications:
  - In C, a delta ConfigMap edit does **not** wake the XR — the TUI must also bump the XR to force recompose, and end-to-end latency is gated on control-plane health and the provider queue. Expect seconds-to-minutes, not sub-second.
  - During a cluster outage, C cannot act at all. A can act on PVE but the tool must **refuse** when it cannot set the pause annotation (an unpauseable change gets reverted on recovery — Crossplane and the providers are equally dead without the apiserver, but they resume on recovery and would revert). Graceful-degradation rule for the A driver: no kube, no write.
- Migration risk of C is the Orphan dance, runbook-ordered: install XRD+composition → flip existing VM MRs to `deletionPolicy: Orphan` → delete raw VM MRs (PVE untouched; vmid adoption via `crossplane.io/external-name` is already proven machinery) → sync chart with XR instances → new MRs adopt the running VMs. One careful maintenance window; rollback by reversing (git history keeps the raw templates).
- Blast radius: one composition serves all VM XRs; a bad function/patch stalls every VM's reconcile at once. Mitigation: pin function image versions; v2 `CompositionRevision` + per-XR `compositionRevisionRef` lets composition changes themselves be canaried per-VM.
- Security inversion: the C driver needs only a kubeconfig (PVE `root@pam` creds never leave the cluster; ProviderConfig is ESO→1Password). A needs PVE write creds on the laptop plus kube for the pause. Multi-admin future favors C; single-operator + fragile-cluster present favors A.
- "Contributors must speak Crossplane": in C, every VM-shape change (even in git) flows through XRD schema + composition + functions. This team is already deep-Crossplane (gcp/cloudflare/proxmox providers, ESO), so the marginal concept cost is modest — but the VM-shape knob is touched *frequently* (the 2026-09-20 dance) and the substrate is *variable*; frequent knob on fragile substrate is the coupling to avoid.

### Verdict (sharpened)

**Ship A (standalone `pvedial`, `pve-direct` driver) now; treat C as an upgrade path, not a fork.** Trigger C only on a concrete need: multi-admin RBAC'd access, in-cluster policy/admission for budgets+GPU, or in-cluster audit. The driver interface and the delta-object design (ConfigMap now → CRD later) carry over unchanged, so the A investment is not throwaway. B stays dead; C′ (controller over raw MRs) is B with extra code.

## Standalone product (2026-09-21): the TUI is the product

**Pitch (one line):** "A beautiful TUI to allocate resources to your Proxmox VMs. See every host, move cores and gigabytes with keys, land changes in seconds."

Crossplane is a detail under the fold — an optional backend that keeps the tool from fighting GitOps. It appears in marketing as one footnote line: "Plays nice with GitOps — promote changes back to git, abort anytime." Never in the demo.

### The slides

1. **Main screen (the GIF):** full-screen charm-stack TUI. Left: hosts with live capacity bars (CPU threads, RAM) — allocated vs used, sparklines. Right: VMs grouped by host, status dots, current cores/RAM. All keyboard, vim keys, works over SSH.
2. **The move:** select VM → allocation panel (cores stepper, RAM slider with live headroom — "+4 GiB → pve1 has 5.2 free ✓", GPU radio) → confirm → progress → toast. The 2026-09-20 "give everything to talos" dance becomes 30 seconds: stop workstation, bars free up, slide allocation to talos, done.
3. **Footnote:** GitOps line above. Nothing else.

### Down the stack (layers)

- **L0 — PVE API client:** full read, VM config writes, start/stop/migrate.
- **L1 — Planner:** the guards live here — host budgets (+2 needs −2 donor logic), GPU exclusivity, reboot-required predicates, disk growth-only.
- **L2 — Write backends:** `pve-direct` (default, zero prerequisites — this is the demo path) / gitops-aware backend (pause-or-override + promote; crossplane adapter lives HERE and only here).
- **L3 — Modules:** resource domains as plugins. v0: cpu/mem + start/stop. Next: disk resize, VM migrate-between-hosts (budget bars make this gorgeous), PCI/GPU, network, snapshots.

### Module seam (so disk resizing plugs in without touching anything)

```
Read(node, vm) → ModuleState          // current, capacity, live usage
Fields()      → form schema           // TUI auto-renders the huh form
Plan(from,to) → [Action], warnings    // reboot? donor needed? guard trips?
Apply(actions) → progress events
```

Host bars aggregate ModuleStates (disk module adds storage bars). Forms render from schema — a new module registers and the TUI renders it with zero TUI changes. Config enables/disables modules.

### Distribution + wedge

- GIF on top of the README (vhs over the real TUI, built early — if the GIF isn't undeniable, nothing else matters). r/homelab + r/selfhosted + Proxmox forum; "k9s/btop for Proxmox allocation" is the one-line orientation.
- nix flake, homebrew, goreleaser binaries, single static binary, `pvedial --profile home` one-command start; PVE API token auth (least-priv token, not root@pam password).
- MVP: cpu/mem module + host bars + pve-direct backend. The crossplane backend ships later, behind a flag, invisible unless you have GitOps.
- Dogfood loop = this lab: every real allocation change goes through the TUI from day one.

### N machines, zero clutter (2026-09-21)

Model: **connections → nodes** (a PVE cluster exposes all its nodes through any member's API — so "N machines" is usually one endpoint, N nodes; standalone nodes are separate connections). Chrome adapts to what exists:

- **1 node:** no switcher, no tabs — the VM list IS the screen. Host line = one full-width header ("pve1 · Ryzen 3400G · 12.3/15.6 GiB ▓▓▓░"). A single-machine user never learns the word "cluster."
- **2–9 nodes:** one-line tab strip (or slim sidebar), per-node mini bars; switch with `[`/`]`, `1-9`. Selected node's VMs fill the screen; others exist only in the strip.
- **Many / multi-cluster:** fuzzy switcher (`s`), plus an "all" aggregate view for cross-node rebalance. Unreachable node = dim dot in the strip, never an error screen.

Rules: one goroutine per connection, async — connecting never blocks the UI, sparklines keep ticking on the live ones. Switching = pointer flip, no reconnect. Cross-node donor logic: same-node allocation only; cross-node = the migrate module's path (target picker shows which node has room — that's the stunning demo). Config: `[[hosts]]` TOML entries; zero-config first run prompts and writes the profile; one host → never asked again.

### Self-containment: one CLI, components inside (2026-09-21)

No editions, no modes-as-products. One binary, componentized:

```
pvedial
├── TUI        the product — allocation across N nodes, modules render here
├── backend    pluggable write path behind one interface
│   ├── pve-direct   default — writes PVE API directly; zero prerequisites
│   └── crossplane   active once `pvedial install` ran — writes DialOverride + XR bump
├── install    ships OUR crossplane side (Go embed → applied to cluster):
│               provider-proxmox + ProviderConfig (secret = the token minted at
│               login, no ESO), XRD, Composition (refs our function OCI image),
│               function-dial, DialOverride CRD, RBAC — then adopts existing VMs
│               (external-name = vmid). User authors zero YAML.
├── discover / login / adopt   zero-config plumbing (LAN probe :8006, mint
│               least-priv token from root creds once, KUBECONFIG detection)
└── modules    cpu/mem (v0), disk, migrate, gpu… — the plugin seam
```

Backend activation is automatic, not a user choice: no install → pve-direct; install present (marked by a small state file + CRD existence check) → crossplane. Same TUI, same modules, same discovery either way — reads always come live from PVE; only the write path swaps. Existing-XR shops with their own compositions: `install --function-only` (add our function step, keep their shapes).

Minimum denominators, restated as requirements not editions: pve-direct needs Proxmox. crossplane backend needs a cluster with Crossplane v2 — everything above that is shipped by `install`.

Zero-config ladder: 1) LAN probe finds PVE hosts, one [Y/n]. 2) `pvedial login`: root@pam pasted once → we mint `pvedial@pve` + least-priv token, discard root creds; config written by us. 3) kubeconfig found + install run → done. 4) New VMs/nodes = live reads, `pvedial adopt` repeatable for gitops mode.

Guard: install must detect foreign ownership (VM claimed by another composition/XR) and refuse adoption rather than fight — detach is an explicit guided step.

## Out of Scope

- VM provisioning/deletion, disk resize, ISO handling (Crossplane chart stays owner).
- A browser/web UI; admission webhook policy enforcement (later, optional).
- PVE cluster HA/fencing, storage rebalancing, non-QEMU (LXC) containers.
