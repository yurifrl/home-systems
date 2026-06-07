# Design: nostos in-cluster bootstrap controller (`nostos-bootstrap`)

Status: draft
Date: 2026-06-06
Owner: Yuri
Related: `.agents/drafts/nostos-pxe-reliable-ai-friendly.md`, nostos submodule

---

## 1. Problem

Today a fresh `talos-default` cluster comes up via a pile of **loose YAML
hand-stuffed into the Talos machineconfig** (`nostos/templates/dell01.yaml`):

- `extraManifests:` → 2 GitHub raw URLs (argocd, app-of-apps)
- `inlineManifests:` → 5 blobs (3 namespaces, the 1Password Connect secret, the
  ESO `ClusterSecretStore`)
- and the CNI (Cilium) isn't wired at all — it's installed **manually** by hand
  (`kubectl apply` of a rendered manifest).

Consequences:
- **No single source of truth** — bootstrap state is smeared across the
  machineconfig, GitHub raw URLs, and a human running kubectl.
- **Not self-healing** — Talos applies inlineManifests at cluster-init only;
  nothing reconciles drift or recovers a half-bootstrap.
- **No observability** — there's no one place to see "is the cluster bootstrap
  healthy and why."
- **Fragile ordering** — CNI must exist before anything else can schedule, but
  nothing enforces that.

## 2. Goals / non-goals

**Goals**
- One declarative source (`config.yaml`) for everything required to get a
  cluster from *bare apiserver* to *ArgoCD reconciling the user's repo*.
- A **self-healing** in-cluster controller that reconciles the bootstrap tier
  continuously (not a run-once Job) and **logs very well** — a **single place to
  look** for bootstrap health.
- **Config-driven app generation**: the user sets their GitOps repo (+ paths) in
  config; the controller **generates the ArgoCD Application(s)** that point at
  that repo. The user never hand-writes the root app.
- **Deterministic ordering**: CNI → secrets/ESO → ArgoCD → generated apps.
- Reduce the machineconfig to the **one irreducible secret** + the controller.

**Non-goals**
- Replacing ArgoCD. The controller hands off to ArgoCD and never manages the
  user's individual apps.
- Managing day-2 app config (that stays in the user's git repo, owned by ArgoCD).
- Being a general operator framework — scope is strictly cluster bootstrap.

## 3. Architecture

```
config.yaml  ──nostos render──►  machineconfig:
  (bootstrap:)                     ├─ inlineManifest: op-credentials Secret  (root secret, irreducible)
  cilium: {...}                    ├─ inlineManifest: bootstrap-config ConfigMap (rendered from config.yaml)
  argocd:  {version, ...}          └─ inlineManifest: nostos-bootstrap Deployment + SA/RBAC
  repos:   [{url, path}]                         │
  namespaces: [...]                              ▼
                                   nostos-bootstrap controller (self-healing, hostNetwork)
                                     reconcile loop, ordered:
                                       1. Cilium (CNI)          ──► nodes Ready
                                       2. ESO + ClusterSecretStore (uses root secret)
                                       3. ArgoCD
                                       4. generate ArgoCD Application(s) from config → point at user repo
                                     then keep tiers 1-4 healthy; expose status + logs
                                               │
                                               ▼
                                   ArgoCD ──► syncs everything else from the user's repo
```

### 3.1 Three tiers (the cleanup)

| Tier | Lives in | Contents |
|---|---|---|
| **Config (source of truth)** | `config.yaml` `bootstrap:` | Cilium values, ArgoCD version, user repo URL(s)+path(s), root namespaces |
| **nostos-owned (irreducible, machineconfig)** | rendered inline | the one root 1Password Connect Secret + bootstrap ConfigMap + the controller Deployment/SA/RBAC |
| **git / ArgoCD** | user's repo | all actual apps; ArgoCD owns them |

Today's 5 inline blobs + 2 extra URLs + manual Cilium **collapse into**:
`config.yaml` → 1 root secret + 1 ConfigMap + 1 controller.

## 4. Components

### 4.1 The controller binary (`cmd/bootstrap` in the nostos repo)
- A **self-healing reconcile loop** (Deployment, single replica, leader-elect
  ready for future HA). Runs `hostNetwork: true`, tolerates the
  `node.kubernetes.io/not-ready` taint, pins to control-plane — so it can come
  up **before the CNI exists** (same trick Cilium's own DaemonSet uses).
- Reads desired state from the **bootstrap-config ConfigMap** (rendered from
  `config.yaml`) + the root Secret.
- Applies bootstrap tiers via **server-side-apply (client-go)** against embedded,
  parameterized manifests. **No Helm at runtime** (keeps the image small — see
  issue #2).
- Reconciles on an interval + on watch; **idempotent**; recreates anything
  missing (self-heal).
- **Logging**: structured (JSON + human), one line per reconcile decision, plus a
  rolled-up **status** written to a `ConfigMap`/CR (`BootstrapStatus`) so "a
  single place to look" = `kubectl logs deploy/nostos-bootstrap` and/or
  `kubectl get bootstrapstatus`.

### 4.2 Image + CI
- New tiny **multi-arch** image (amd64 + arm64) → `ghcr.io/yurifrl/nostos-bootstrap`.
- Static Go binary, distroless/scratch base.
- CI builds + pushes on tag; tag is pinned in `config.yaml`/machineconfig.

### 4.3 nostos render changes
- New `bootstrap:` block in `config.yaml`.
- `nostos render` emits exactly three inline manifests: root Secret (op:// resolved),
  bootstrap ConfigMap (serialized `bootstrap:` block), controller Deployment+RBAC
  (image tag from config).
- Remove the 5 hand-written inline blobs + 2 extraManifest URLs from templates.

### 4.4 App generation
- `config.yaml` lists the user's repo(s): `repos: [{url, path, revision}]`.
- The controller **generates one ArgoCD root Application per repo entry**
  (app-of-apps style) pointing at the user's repo/path. The user manages the
  actual app manifests **in their repo**; the controller only creates the root
  app(s). Single config knob → apps appear.

## 5. Bootstrap ordering (the reconcile sequence)

1. **Cilium** — apply; wait until all nodes report `Ready` (CNI up).
2. **ESO + `ClusterSecretStore`** — apply CRDs + operator; create the store using
   the root Secret; wait until store is `Valid`.
3. **ArgoCD** — apply; wait until the API/server is healthy.
4. **Generate apps** — render ArgoCD Application(s) from `config.repos`, apply.
5. **Hand off** — ArgoCD reconciles the rest. Controller keeps tiers 1-4 healthy
   and reports status; it never touches tier-5 apps.

Each step gates the next (wait + timeout + retry). Failures are logged with
cause and surfaced in `BootstrapStatus`; the loop retries (self-heal).

## 6. Self-healing model & boundary with ArgoCD

- Controller **exclusively owns** Cilium, ESO+store, ArgoCD, and the generated
  root Application(s). ArgoCD does **not** manage these → no split-brain.
- ArgoCD owns everything in the user's repo.
- "Self-heal" = if Cilium/ESO/ArgoCD/root-app drift or vanish, the controller
  re-applies from config. Upgrades happen by changing `config.yaml` → re-render →
  re-apply machineconfig → controller reconciles (see issue #5/#7 on the
  config-update loop).

## 7. Observability ("single place to look")

- `kubectl logs deploy/nostos-bootstrap -f` — ordered, structured reconcile log.
- `kubectl get bootstrapstatus -o yaml` (or a status ConfigMap) — per-tier state
  (`Cilium: Ready`, `ESO: Valid`, `ArgoCD: Healthy`, `Apps: Synced`), last error,
  last reconcile time.
- Optional: Prometheus metrics endpoint for the VM/Grafana stack.

## 8. Potential issues / risks

1. **CNI/scheduling paradox** — the controller pod can't get a CNI IP because it
   *is* installing the CNI. Must run `hostNetwork: true` + not-ready toleration +
   control-plane node-selector. Verified-possible (Cilium DS does it); must be
   built exactly so or it never schedules.
2. **"Small" vs. capability** — installing Cilium/ArgoCD well wants Helm; Helm is
   heavy. Decision baked in: **embed rendered manifests + client-go SSA, no Helm
   at runtime.** client-go still adds ~tens of MB; "super small" is relative
   (single static binary, ~30-50MB image), not tiny.
3. **Root-secret irreducibility** — the 1Password Connect secret must still be
   nostos-injected into the machineconfig; can't reach zero loose secret YAML,
   only **one**.
4. **Multi-arch + registry reachability pre-CNI** — image must be multi-arch
   (mixed fleet). Pulled over host network before CNI; public ghcr is fine, a
   **private** image needs pull creds = another root secret. Offsite rpi01 pulls
   over Tailscale at boot — confirm the tailnet route is up before CNI.
5. **Config-update loop is heavier than git** — because bootstrap config rides in
   the machineconfig, changing it = `nostos render` + re-apply machineconfig +
   controller reconcile. Fine for rare bootstrap-tier changes; **do not** put
   fast-moving app config here (that's why apps stay in the user's git repo).
6. **Controller vs ArgoCD overlap** — must keep a hard boundary (controller owns
   tiers 1-4, ArgoCD owns tier 5). If the user's repo also defines Cilium/ArgoCD,
   they'll fight. Document/guard against it.
7. **Version coupling** — image tag pinned in machineconfig; bumping the
   controller = re-render + re-apply every control-plane. CI must build/push/tag
   the image in lockstep with nostos releases.
8. **Single replica SPOF on bootstrap** — on a single-control-plane cluster the
   controller runs on dell01; if dell01 is down, no self-heal. Acceptable at
   bootstrap; leader-election hook lets it scale when rpi01 joins.
9. **Powerful SA** — controller needs near-cluster-admin (CNI, CRDs, RBAC).
   Scope tightly; consider a narrower role once bootstrapped.
10. **Talos applies inline at init only** — the controller Deployment survives as
    a normal workload after init, but if it's ever fully deleted, only a
    machineconfig re-apply (or talosctl upgrade-k8s) recreates it. Document the
    re-trigger.
11. **"Is the controller worth it?"** — Talos extraManifests already fetch
    argocd+app-of-apps by URL. The controller earns its keep via **ordering +
    wait-for-Ready + self-heal + config-driven app-gen + single-pane status**.
    If those weren't required, "Cilium as an extraManifest URL + ArgoCD does the
    rest" would be far less to build. We are building it because self-heal +
    single source + observability are the explicit goals.

## 9. Open decisions

- **Status surface**: lightweight (a status `ConfigMap`) vs. a real
  `BootstrapStatus` CRD (+ CRD lifecycle). CRD is nicer UX, more to maintain.
- **App-gen granularity**: one root app-of-apps per repo entry (simplest) vs.
  generating individual `Application`s from a config list.
- **Cilium upgrade UX**: via `config.yaml` + machineconfig re-apply (consistent,
  heavier) vs. letting ArgoCD adopt Cilium for day-2 (splits ownership — not
  recommended given the self-heal boundary).
- **Reconcile interval / watch**: poll interval value; whether to watch ArgoCD
  health to re-drive ordering.

## 10. Dependency / sequencing (critical)

The nostos-render injection is the **terminal** step, not the first one, because
the YAML nostos injects into the Talos machineconfig **references the controller
image**. So the chain is strictly:

1. **nostos repo published on GitHub** — nostos is currently a local submodule
   (`.submodules/nostos/`). It must exist as a real GitHub repo so it can host
   the `cmd/bootstrap` source **and** the CI that builds + pushes the multi-arch
   image to `ghcr.io/yurifrl/nostos-bootstrap`. **No repo → no image → nothing to
   inject.**
2. **Image built + pushed** (multi-arch, tagged) — the tag the injected YAML
   pins must actually exist in ghcr before any cluster boots with it.
3. **Only then** can `nostos render` inject the controller Deployment (+ config
   ConfigMap + root Secret) into Talos, because the Deployment's `image:` must
   resolve at pull time.

## 11. Migration plan (today → this)

1. **Publish the nostos repo to GitHub** (prerequisite for everything — see §10).
2. Build `cmd/bootstrap` + image + CI (multi-arch ghcr). Validate on a throwaway
   Talos VM cluster end-to-end (bare → ArgoCD reconciling).
3. Add `bootstrap:` schema to `config.yaml`; teach `nostos render` to emit the
   3 inline manifests; delete the 5 hand-written blobs + 2 extra URLs.
4. Cut over on a rebuild: `nostos up dell01` → controller bootstraps Cilium →
   ESO → ArgoCD → generated apps; verify single-pane status green.
5. Retire the manual `nostos/manifests/cilium/*` stopgap (keep one rendered
   break-glass copy if desired).
6. Wire rpi01 the same way (offsite); verify multi-arch + Tailscale-pre-CNI pull.

---

### Decisions captured from this discussion
- Controller is **self-healing + logs very well + single place to look** (not a
  one-shot Job).
- Apps are **generated from config**, pointing at the **user's repo** (user
  configures the repo; we create the root app).
- Controller **owns bootstrap ordering**; goal is guaranteed bootstrap + one
  place to look.
