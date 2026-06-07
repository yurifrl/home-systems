# Plan: Productize nostos — strip lab specifics, "smart" config (no 1000-line file)

Status: draft
Date: 2026-06-06
Owner: Yuri
Repo: github.com/yurifrl/nostos (public)
Related: nostos-cluster-bootstrap-controller.md, nostos-render-injection-plan.md

---

## 1. What "productize" means here

nostos should let **anyone** provision a Talos + GitOps cluster, with:
- **zero of Yuri's lab specifics** baked into the code, and
- a **tiny** amount of config to stand up a cluster — not a hand-written
  thousand-line machineconfig per node.

The good news: nostos **already** separates **tool** (`.submodules/nostos`, the
Go module — what we pushed) from **lab data** (`nostos/` in home-systems:
`config.yaml` + per-node `templates/*.yaml`). The lab data was **not** pushed.
So "strip my things" is mostly removing hardcoded **defaults/fixtures** from the
tool — small. The hard, valuable part is the **config-burden** redesign.

## 2. The audit — what's actually leaking (tool repo only)

From scanning the pushed `yurifrl/nostos`:

| Leak | Location | Fix |
|---|---|---|
| `vault: kubernetes` | `internal/cli/init.go` default | placeholder (`my-vault`) or prompt |
| `192.168.68.100` (real CP IP) | `init.go` default + `boot_test.go` | placeholder (`10.0.0.10`) |
| `op://kubernetes/talos/...`, `op://kubernetes/turingpi/...` | `secrets_test.go`, `boot_test.go` | generic fixtures (`op://vault/item/field`) |
| `account: my.1password.com` | `init.go` | **keep** — generic default URL for all personal accounts (not identifying) |
| `yurifrl`, README "extracted from yurifrl/home-systems" | module path, README | module path stays (it's the repo); genericize README prose |

**None are secret values.** This scrub is ~1 hour of work. The real work is §3-§6.

## 3. The core problem: config burden

Today a node = a **hand-written ~200-line Talos machineconfig** template
(`dell01.yaml`). That does not scale to a product — nobody will hand-author Talos
YAML per node, per cluster. We must make the **common path tiny** while keeping
an **escape hatch** for power users.

### Design principle: progressive disclosure (three tiers of effort)
1. **Generated (default)** — user writes a *compact node spec* (~8 lines); nostos
   generates the full machineconfig from a built-in, versioned **base template**.
2. **Base + patches** — user supplies small **strategic-merge patches** (Talos
   config patches) layered on the base for their specifics (extra disks, hostDNS,
   kernel args). Tens of lines, not hundreds.
3. **Full template (power)** — user provides a complete machineconfig template
   (today's behavior). Always supported as the escape hatch.

So config size scales with how unusual your cluster is — most users live in tier 1.

### What a tier-1 cluster config looks like (target)
```yaml
cluster:
  name: home
  endpoint: https://10.0.0.10:6443      # or derived from first controlplane node
  talos_version: v1.13.3
  # cni/podSubnet/serviceSubnet/dnsDomain/kubePrism/features all DEFAULTED
secrets:
  backend: onepassword
  onepassword: { vault: my-vault }      # vault name is the ONE secret knob
bootstrap:                              # drives the in-cluster controller (other doc)
  repos: [{ url: https://github.com/me/gitops.git, path: apps }]
nodes:
  - { name: cp1, ip: 10.0.0.10, role: controlplane, arch: amd64, disk: /dev/nvme0n1, mac: "aa:bb:..." }
  - { name: w1,  ip: 10.0.0.11, role: worker,       arch: arm64, disk: /dev/mmcblk0, mac: "aa:bb:..." }
```
~20 lines for a 2-node cluster. Everything else is defaulted or generated.

## 4. The two big "smart" mechanisms

### 4.1 Built-in base machineconfig templates (generation, not authoring)
- Ship **base templates per role** (controlplane / worker), parameterized by the
  node spec (`name, ip, role, arch, disk, mac`) + cluster defaults.
- `nostos render` merges: **defaults → base template → per-node patches →
  (optional) full template override**. Uses Talos's own config-patch /
  strategic-merge semantics so it's predictable.
- Base templates are **versioned with nostos** and overridable per-cluster
  (drop a file in the data dir to replace the built-in).
- This is what kills the 1000-line problem: the 200 lines become *built-in*,
  the user writes the 8 that vary.

### 4.0 `nostos init` provisions secrets into 1Password (idempotent, never destructive)

Config size is acceptable; the real friction is secrets. So `nostos init`
**populates the vault for you**:
1. Ask for the vault + item path (where to store everything).
2. **Generate** the machine-generatable Talos secrets (cluster CA, token,
   secretbox key, etcd/aggregator CA, SA key, machine CA, cluster id/secret
   via `talosctl gen secrets`).
3. **Prompt** for the few human secrets (Tailscale OAuth, BMC creds).
4. **Write** them into 1Password via `op item create` / `op item edit`.
5. Write `config.yaml` with composed `op://` refs.

**HARD RULES (non-negotiable):**
- **Secret creation is IDEMPOTENT: create-if-absent only.**
- **NEVER delete a 1Password secret. NEVER overwrite/regenerate an existing
  one.** Re-running `init` must detect existing keys and leave them untouched
  (regenerating cluster CA/token = bricks the cluster; deleting anything in the
  vault is forbidden).
- On a field that already exists: skip it, log that it was kept, move on. Only
  missing fields are created.
- The 1Password-Connect *root* credential (for in-cluster ESO bootstrap) is a
  heavier setup (Connect server / service-account token); keep it a documented
  manual step or a separate `nostos init connect` — still create-if-absent.

This also means we can **keep full machineconfig templates as-is** and skip the
base-template generation work (§4.1) if a larger config is acceptable.

### 4.2 Secret schema + op:// path **convention** (user never writes op://)
Today every template hardcodes `op://kubernetes/talos/CLUSTER_TOKEN` etc. — that
is both a leak and a burden. Instead:
- nostos **defines the set of secret keys it needs** (a fixed schema):
  cluster CA/token/etcd-CA/sa-key, per-node machine CA, Tailscale authkey, BMC
  creds, the 1Password-Connect root secret, etc.
- nostos **composes** the op:// refs from a convention:
  `op://<vault>/<item>/<KEY>` where `<vault>` = config (one line), `<item>` =
  convention (e.g. cluster secrets in item `<cluster.name>`, node boot creds in
  item `<node.name>`), overridable.
- nostos ships **`nostos secrets schema`** (what keys/items you must create) and
  **`nostos secrets test`** (already exists) to validate the vault has them.
- The user creates **one or two documented 1Password items** with named fields;
  they never hand-write an op:// path. No vault/item names in any template.

## 5. `nostos init` / `nostos node add` — the onboarding wizard
- `nostos init` (exists) → interactive: cluster name, endpoint, Talos version,
  secret backend + vault, GitOps repo. Writes a minimal `config.yaml`. Defaults
  for everything else.
- `nostos node add` (exists) → wizard for the compact node spec.
- `nostos secrets schema --output text` → prints the exact 1Password items/fields
  to create (copy-paste checklist), so secret setup isn't guesswork.
- `nostos schema` (exists) already exposes every field + default for docs/UX.

## 6. Layering / override model (the contract)
```
built-in defaults  <  base template (per role)  <  per-node patches  <  full template
config.yaml values flow into all layers; op:// refs are composed, never authored.
```
- Everything has a default; the user only writes deltas.
- Power users can still drop a full template (current behavior) — nothing is lost.

## 7. What to strip / change in the repo (work items)
1. **Scrub fixtures/defaults** (§2): `init.go`, `secrets_test.go`, `boot_test.go`,
   README prose → generic placeholders.
2. **Move hardcoded bootstrap specifics to config**: the 2 GitHub URLs + 5 inline
   blobs in the example template are Yuri's — they become the `bootstrap:` block
   + controller (already planned in the bootstrap-controller doc).
3. **Add built-in base templates** (§4.1) under the nostos module (embedded),
   + the merge/patch pipeline in `render`.
4. **Add secret-schema + op:// composition** (§4.2) — define the key schema,
   compose refs, add `nostos secrets schema`.
5. **Genericize example data**: ship an `examples/` cluster (fake IPs/vault) that
   `nostos init` can scaffold from; keep Yuri's real lab data only in
   home-systems, never in the product repo.
6. **Docs**: README quickstart ("3 nodes in 20 lines"), the secret checklist.

## 8. Migration: Yuri's lab → consumer of the product
- Yuri's `home-systems/nostos/` becomes a **consumer** of nostos: convert his
  hand-written `templates/*.yaml` into compact node specs + a few patches
  (his specifics: Longhorn extra disk, hostDNS forward, exclude-from-LB label,
  dashboard kernel arg). Keep full-template escape hatch for anything awkward.
- His op:// paths (`op://kubernetes/talos/*`) become composed from
  `vault: kubernetes` + cluster name `talos-default` (or an explicit item map),
  so nothing lab-specific stays in templates.
- Validate parity: rendered machineconfig for dell01 (new path) must match the
  current working one (diff to zero meaningful delta) before cutover.

## 9. Risks / open questions
1. **Talos config surface is huge** — base templates can't anticipate every
   field. Mitigation: patches + full-template escape hatch; don't over-promise
   "config-only" for exotic setups.
2. **Generation parity** — the generated dell01 config must equal today's working
   one. Need a golden-file diff test before trusting generation in prod.
3. **op:// item convention vs reality** — Yuri's current items are `talos`,
   `turingpi`, `op-credentials` (not `<cluster.name>`/`<node.name>`). Either
   migrate the vault to the convention, or support an explicit `items:` map in
   config. **Decide.**
4. **How much to default vs expose** — defaulting CIDRs/ports/features is safe;
   defaulting Cilium values is opinionated. Ship a good default, allow override.
5. **Base-template versioning** — when nostos bumps a base template, existing
   clusters must opt in (don't silently change machineconfig). Tie to
   `talosctl upgrade-k8s` / explicit re-render + apply.
6. **Scope creep** — this is a real product effort. Sequence it: scrub first
   (cheap, unblocks "public is clean"), then base-templates + secret-schema, then
   migrate Yuri's lab last.

## 10. Proposed sequencing (beads)
1. **Scrub lab specifics from tool** (§2) — small, do first; makes the public repo clean.
2. **Secret schema + op:// composition** (§4.2) — removes op:// authoring + vault leak.
3. **Built-in base templates + merge/patch render pipeline** (§4.1) — the big one.
4. **`nostos init`/`node add`/`secrets schema` UX polish** (§5).
5. **examples/ + README quickstart + docs** (§7.5/6).
6. **Migrate home-systems lab to the new model w/ golden-diff parity** (§8).

Each is an epic-child; (1) is independent and ready now. (3) depends on nothing
but is the largest. (6) is last (depends on 1-4).
```
```
