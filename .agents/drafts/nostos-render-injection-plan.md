# Plan: nostos render → emit 3 bootstrap inline manifests

Status: design / research (NO Go edits yet)
Date: 2026-06-06
Owner: Yuri
Related: `.agents/drafts/nostos-cluster-bootstrap-controller.md` (§3, §4.3, §4.4, §5, §10)

---

## 0. Summary of the change

Today `nostos/templates/dell01.yaml` hand-stuffs into `cluster.*`:

- `extraManifests:` → 2 GitHub raw URLs (`manifests/argocd.yaml`, `manifests/applications.yaml`)
- `inlineManifests:` → 5 blobs:
  1. `namespace-argocd`
  2. `namespace-external-secrets`
  3. `namespace-1password`
  4. `secret-op-credentials` (1Password Connect Secret, namespace `1password`)
  5. `cluster-secret-store-onepassword` (ESO `ClusterSecretStore`)

Goal: `nostos render` instead **synthesizes** exactly THREE inline manifests from a
new `bootstrap:` block in `config.yaml` and appends them to `cluster.inlineManifests`:

1. **(a) root `op-credentials` Secret** — keeps its `op://` refs so
   `secrets.ResolveTemplate` still resolves them.
2. **(b) `nostos-bootstrap-config` ConfigMap** — the serialized `bootstrap:` block,
   read by the in-cluster controller.
3. **(c) controller bundle** — `ServiceAccount` + `ClusterRole` + `ClusterRoleBinding`
   + `Deployment` (image `ghcr.io/yurifrl/nostos-bootstrap:<tag>`).

And REMOVE the 5 hand-written inline blobs + 2 extraManifest URLs from the template.
The controller (designed separately) takes over namespaces, ESO, ClusterSecretStore,
Cilium, ArgoCD, and app-gen at runtime.

---

## 1. Proposed `bootstrap:` config struct

New file `internal/config/bootstrap.go` (or appended to `config.go`). Wired into the
root `Config` struct.

```go
// Bootstrap is the optional cluster-bootstrap tier. When present, `nostos render`
// synthesizes the three inline manifests (root Secret, config ConfigMap, controller
// bundle) and appends them to cluster.inlineManifests. When absent (nil), render
// behaves exactly as today (templates own their own inlineManifests).
type Bootstrap struct {
	// Cilium is the CNI tier the controller installs first.
	Cilium BootstrapCilium `yaml:"cilium" validate:"required"`
	// Argocd is the GitOps engine the controller installs after ESO is valid.
	Argocd BootstrapArgocd `yaml:"argocd" validate:"required"`
	// Repos are the user GitOps repos; the controller generates one root ArgoCD
	// Application per entry (app-of-apps). At least one is required.
	Repos []BootstrapRepo `yaml:"repos" validate:"required,min=1,dive"`
	// Namespaces are created by the controller before ESO/ArgoCD (replaces the
	// 3 hand-written namespace inline blobs). Order-independent.
	Namespaces []string `yaml:"namespaces,omitempty" validate:"dive,hostname_rfc1123"`
	// ControllerImage pins the nostos-bootstrap controller image. The Deployment
	// emitted by render references repo:tag. Tag MUST already exist in the
	// registry before any cluster boots with this config (see Risks §4).
	ControllerImage BootstrapImage `yaml:"controller_image" validate:"required"`
	// RootSecret describes the irreducible 1Password Connect Secret that render
	// emits with op:// refs intact (resolved by secrets.ResolveTemplate).
	RootSecret BootstrapRootSecret `yaml:"root_secret" validate:"required"`
}

// BootstrapCilium pins the Cilium version and optional Helm-style values that
// the controller renders into embedded manifests (no Helm at runtime).
type BootstrapCilium struct {
	Version string `yaml:"version" validate:"required"`
	// Values is an opaque map passed through to the controller verbatim (stored
	// in the config ConfigMap). nostos does not interpret it; the controller does.
	Values map[string]any `yaml:"values,omitempty"`
}

// BootstrapArgocd pins the ArgoCD version the controller installs.
type BootstrapArgocd struct {
	Version string `yaml:"version" validate:"required"`
}

// BootstrapRepo is one user GitOps repo the controller turns into a root app.
type BootstrapRepo struct {
	URL      string `yaml:"url"      validate:"required,url"`
	Path     string `yaml:"path"     validate:"required"`
	Revision string `yaml:"revision,omitempty"` // default "HEAD" applied by controller
}

// BootstrapImage is a repo + tag pair for an OCI image.
type BootstrapImage struct {
	Repo string `yaml:"repo" validate:"required"` // e.g. ghcr.io/yurifrl/nostos-bootstrap
	Tag  string `yaml:"tag"  validate:"required"` // e.g. v0.1.0 (must exist in registry)
}

func (i BootstrapImage) Ref() string { return i.Repo + ":" + i.Tag }

// BootstrapRootSecret describes the one irreducible Secret. Keys map to Secret
// data keys; values are op:// (or sops://, file://) refs left intact through the
// text/template pass and resolved by secrets.ResolveTemplate. Values must already
// be base64-encoded in the backend, because they land in Secret.data (NOT
// stringData) — this matches today's secret-op-credentials blob exactly.
type BootstrapRootSecret struct {
	Name      string         `yaml:"name"      validate:"required"` // op-credentials
	Namespace string         `yaml:"namespace" validate:"required"` // 1password
	Data      map[string]Ref `yaml:"data"      validate:"required,min=1"`
}
```

Root `Config` gains:

```go
type Config struct {
	Cluster   Cluster         `yaml:"cluster" validate:"required"`
	Secrets   Secrets         `yaml:"secrets" validate:"required"`
	Bootstrap *Bootstrap      `yaml:"bootstrap,omitempty"` // nil => legacy behavior
	Nodes     map[string]Node `yaml:"nodes,omitempty" validate:"dive"`
}
```

`*Bootstrap` is a pointer so a nil block = today's behavior (templates keep owning
their inline manifests; render does not synthesize). `validator.WithRequiredStructEnabled()`
already dives into non-nil structs, so the `validate:"required"` sub-fields only fire
when `bootstrap:` is present.

### Example `config.yaml` addition

```yaml
bootstrap:
  cilium:
    version: 1.16.5
    values:
      kubeProxyReplacement: true
      k8sServiceHost: 192.168.68.100
      k8sServicePort: 6443
  argocd:
    version: 7.7.7
  repos:
    - url: https://github.com/yurifrl/home-systems.git
      path: k8s/applications
      revision: main
  namespaces: [argocd, external-secrets, 1password]
  controller_image:
    repo: ghcr.io/yurifrl/nostos-bootstrap
    tag: v0.1.0
  root_secret:
    name: op-credentials
    namespace: 1password
    data:
      1password-credentials.json: op://kubernetes/op-credentials/OP_CREDENTIALS_JSON
      token: op://kubernetes/op-credentials/OP_CONNECT_TOKEN
```

---

## 2. How `Render` emits the 3 inline manifests

### 2.1 Where in the pipeline

`registry.Render` (in `internal/registry/registry.go`) currently runs:

1. `Get(cfg, name)` → node
2. `os.ReadFile(tmplPath)` → raw template body
3. `renderTemplateBody(body, cfg, node)` → Go `text/template` pass (only `.InstallImage`)
4. `secrets.BuildBackends(cfg)` → backends
5. `secrets.ResolveTemplate(templated, backends)` → resolve `op://`/`tailscale://`
6. write to `state/configs/<file>.yaml`

The bootstrap injection must happen **between step 3 and step 5** — i.e. produce the
3 manifests as YAML text and splice them into the rendered template body **before**
`secrets.ResolveTemplate` runs, so the root Secret's `op://` refs get resolved by the
SAME existing pass. No new secret resolution code.

Concretely, insert a new step 3.5:

```go
templated, err := renderTemplateBody(string(body), cfg, node)
if err != nil { ... }

// NEW: synthesize bootstrap inline manifests and merge into cluster.inlineManifests.
if cfg.Bootstrap != nil {
	templated, err = injectBootstrapManifests(templated, cfg)
	if err != nil {
		return "", fmt.Errorf("inject bootstrap manifests for node %q: %w", name, err)
	}
}

backends, err := secrets.BuildBackends(cfg)
// ... ResolveTemplate(templated, backends) resolves op:// inside the injected Secret
```

Only inject on **controlplane** nodes that perform cluster-init. Gate:
`if cfg.Bootstrap != nil && node.Role == "controlplane"`. (Talos applies inline
manifests at cluster-init only; workers never run them, so emitting them on workers
is harmless but pointless — gate to keep output clean. Multi-controlplane: all
controlplanes carry identical inline manifests; whichever inits first wins, the rest
are no-ops. See Risks §4 idempotency.)

### 2.2 How to merge into `cluster.inlineManifests`

The template body is multi-document YAML (machine + ExtensionServiceConfig, split by
`---`). Naive string concatenation is fragile. Two viable approaches; recommend B.

- **Approach A (string splice):** locate the `inlineManifests:` key under `cluster:`
  in the first document and append rendered list items with correct indentation.
  Brittle (indentation, key may be absent). Rejected.

- **Approach B (structured re-marshal of the first doc):** parse only the FIRST YAML
  document into a `yaml.Node` (or `map[string]any`), navigate `cluster.inlineManifests`,
  append the 3 generated entries, re-marshal that document, and re-join with the
  remaining documents (the `ExtensionServiceConfig`) unchanged. Use `yaml.v3` which is
  already a dependency. This keeps op:// strings intact as plain scalars (they pass
  through marshal untouched) so step 5 still resolves them.

  Implementation sketch in new file `internal/registry/bootstrap.go`:

  ```go
  // injectBootstrapManifests parses the first YAML document of the rendered
  // template, appends the 3 synthesized inline manifests under
  // cluster.inlineManifests, and re-joins all documents. op:// refs inside the
  // root Secret are preserved as scalars for secrets.ResolveTemplate.
  func injectBootstrapManifests(body string, cfg *config.Config) (string, error) {
      docs := splitYAMLDocuments(body)        // split on "\n---\n" boundaries
      var first map[string]any
      if err := yaml.Unmarshal([]byte(docs[0]), &first); err != nil { ... }

      cluster, _ := first["cluster"].(map[string]any)
      if cluster == nil { return "", fmt.Errorf("template has no cluster: block") }

      existing, _ := cluster["inlineManifests"].([]any)
      manifests := append(existing,
          rootSecretManifest(cfg.Bootstrap.RootSecret),     // (a)
          bootstrapConfigManifest(cfg.Bootstrap),           // (b)
          controllerBundleManifest(cfg.Bootstrap),          // (c)
      )
      cluster["inlineManifests"] = manifests

      out, err := yaml.Marshal(first)
      // re-join: out + "---\n" + docs[1:] ...
  }
  ```

  Each `*Manifest` helper returns a `map[string]any{"name": ..., "contents": ...}`
  where `contents` is a YAML string (Talos inlineManifest schema = `{name, contents}`).

### 2.3 (a) Root Secret — keep op:// intact

`rootSecretManifest` builds the `contents` string by **templating a constant**, NOT by
base64-ing anything. The op:// values go in verbatim under `data:` (matching today's
blob), so when `secrets.ResolveTemplate` runs in step 5 it replaces them with the
already-base64-encoded values stored in 1Password. The op backend (`internal/secrets/op.go`)
returns the raw `op read` output untouched, exactly as today.

```go
func rootSecretManifest(rs config.BootstrapRootSecret) map[string]any {
	var b strings.Builder
	b.WriteString("apiVersion: v1\nkind: Secret\ntype: Opaque\nmetadata:\n")
	fmt.Fprintf(&b, "  name: %s\n  namespace: %s\ndata:\n", rs.Name, rs.Namespace)
	// stable key order for idempotent output
	keys := sortedKeys(rs.Data)
	for _, k := range keys {
		fmt.Fprintf(&b, "  %s: %q\n", k, rs.Data[k].String()) // value is op://...
	}
	return map[string]any{"name": "secret-op-credentials", "contents": b.String()}
}
```

Critical invariant: the op:// strings must survive the `yaml.Marshal` in 2.2 as plain
scalars. Because `contents` is a single multi-line string, the op:// URIs are inside a
string value and Marshal will not mangle them; `secrets.URIPattern` then matches them in
the final text. (URIPattern stops at quotes/whitespace, so `"op://..."` matches the
inner URI correctly.)

### 2.4 (b) bootstrap-config ConfigMap — serialize the bootstrap block

`bootstrapConfigManifest` marshals the `Bootstrap` struct (minus the RootSecret data,
which is secret) to YAML and embeds it as a ConfigMap value. The controller reads this.

```go
func bootstrapConfigManifest(b *config.Bootstrap) map[string]any {
	// Serialize only the non-secret bootstrap config the controller needs.
	pub := struct {
		Cilium     config.BootstrapCilium `yaml:"cilium"`
		Argocd     config.BootstrapArgocd `yaml:"argocd"`
		Repos      []config.BootstrapRepo `yaml:"repos"`
		Namespaces []string               `yaml:"namespaces"`
	}{b.Cilium, b.Argocd, b.Repos, b.Namespaces}
	cfgYAML, _ := yaml.Marshal(pub)

	var c strings.Builder
	c.WriteString("apiVersion: v1\nkind: ConfigMap\nmetadata:\n")
	c.WriteString("  name: nostos-bootstrap-config\n  namespace: kube-system\ndata:\n")
	c.WriteString("  config.yaml: |\n")
	c.WriteString(indentLines(string(cfgYAML), "    ")) // 4-space block indent
	return map[string]any{"name": "nostos-bootstrap-config", "contents": c.String()}
}
```

Namespace `kube-system` so it exists before the controller-created namespaces.
RootSecret name/namespace MAY be included (non-secret metadata) so the controller knows
where to find the Secret; the secret VALUES are never in the ConfigMap.

### 2.5 (c) controller bundle — Deployment + RBAC

`controllerBundleManifest` emits a single inline manifest whose `contents` is a
multi-document YAML string: `ServiceAccount` + `ClusterRole` + `ClusterRoleBinding` +
`Deployment`. Template it from a Go `const` with `fmt.Sprintf`/`text/template`,
substituting `image: <repo:tag>` from `cfg.Bootstrap.ControllerImage.Ref()`.

Key Deployment properties (from controller design §4.1/§8): `hostNetwork: true`,
toleration for `node.kubernetes.io/not-ready` and control-plane taints,
nodeSelector control-plane, single replica, mounts/reads the
`nostos-bootstrap-config` ConfigMap + the root Secret. Namespace `kube-system`.

```go
const controllerBundleTmpl = `apiVersion: v1
kind: ServiceAccount
metadata:
  name: nostos-bootstrap
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: nostos-bootstrap
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]   # near cluster-admin; scope down post-bootstrap (Risk §4.9)
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: nostos-bootstrap
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: nostos-bootstrap}
subjects:
  - {kind: ServiceAccount, name: nostos-bootstrap, namespace: kube-system}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nostos-bootstrap
  namespace: kube-system
spec:
  replicas: 1
  selector: {matchLabels: {app: nostos-bootstrap}}
  template:
    metadata: {labels: {app: nostos-bootstrap}}
    spec:
      hostNetwork: true
      serviceAccountName: nostos-bootstrap
      nodeSelector: {node-role.kubernetes.io/control-plane: ""}
      tolerations:
        - {key: node.kubernetes.io/not-ready, operator: Exists, effect: NoSchedule}
        - {key: node-role.kubernetes.io/control-plane, operator: Exists, effect: NoSchedule}
      containers:
        - name: controller
          image: {{ .Image }}
          args: ["--config=/etc/nostos/config.yaml"]
          volumeMounts:
            - {name: config, mountPath: /etc/nostos}
      volumes:
        - name: config
          configMap: {name: nostos-bootstrap-config}
`
```

The `image:` is the only render-time substitution; everything else is static. Render
this template with `text/template` inside `controllerBundleManifest` and wrap the result
as `{name: "nostos-bootstrap-controller", contents: <rendered>}`.

### 2.6 Output ordering

Append order into `inlineManifests`: (a) root Secret, (b) config ConfigMap, (c) controller
bundle. Talos applies inline manifests in document order, but the controller is what
actually orders runtime work — it won't start reconciling Cilium/ESO until its config +
secret are present, both of which land before it. Keep deterministic ordering (and stable
map-key sorting in 2.3/2.4) so re-renders are byte-identical (idempotency, Risk §4).

---

## 3. What to delete from templates + back-compat

### 3.1 Delete from `nostos/templates/dell01.yaml` (parent repo)

Under `cluster:` remove entirely:

- `extraManifests:` (both GitHub raw URLs)
- `inlineManifests:` (all 5 blobs: 3 namespaces, `secret-op-credentials`,
  `cluster-secret-store-onepassword`)

Render now synthesizes (a)/(b)/(c) into `cluster.inlineManifests`. Namespaces +
ClusterSecretStore + ArgoCD become the **controller's** job at runtime; the GitHub raw
URL extraManifests are replaced by the controller's config-driven app-gen (§4.4 of the
controller design).

Keep `cluster.network.cni.name: none` — still correct; the controller installs Cilium.

### 3.2 Migration / back-compat concern

- **Inline/extra manifests apply at cluster-init ONLY.** On an already-running cluster,
  changing them in the machineconfig does nothing until a fresh `nostos up` (etcd wipe +
  re-init). So this change is **only effective on a clean bootstrap**. The cutover is the
  rebuild described in controller design §11 step 4 (`nostos up dell01`). Document loudly.
- **Pointer-nil = legacy path.** Templates that still carry their own `inlineManifests`
  and have no `bootstrap:` block render exactly as today (tp1/tp4/rpi01 etc.). No forced
  migration of workers. Only templates of nodes whose cluster has a `bootstrap:` block AND
  role=controlplane get synthesized manifests.
- **Double-emission guard.** If a template still hand-writes `inlineManifests` AND
  `bootstrap:` is set, render would append on top (duplicates). Add a render-time check:
  if `cfg.Bootstrap != nil` and the template's first doc already contains
  `cluster.inlineManifests` or `cluster.extraManifests`, fail loud with a message telling
  the operator to delete the hand-written blocks. This catches a half-done migration.

---

## 4. Risks

1. **Ordering: image must exist before inject (controller design §10).** The Deployment
   pins `ghcr.io/yurifrl/nostos-bootstrap:<tag>`. If that tag isn't pushed before a node
   boots, the controller pod `ImagePullBackOff`s and bootstrap stalls forever (no CNI →
   nothing schedules anyway). Mitigation: render does not (cannot cheaply) verify registry
   presence; document the hard prerequisite, and consider an optional
   `nostos render --verify-image` that does a `HEAD`/manifest probe of the tag (skopeo/crane
   or a plain registry v2 API call) and fails render if absent. Default off (offline renders).

2. **Keeping the op:// root secret working.** The whole scheme relies on the injected
   Secret's op:// strings surviving the `yaml.Marshal` re-emit (2.2) as plain scalars so
   `secrets.URIPattern` still matches. Test: render with a stub backend and assert the
   resolved output contains the backend value, not the literal `op://`. Also assert values
   land under `data:` (base64 expected) NOT `stringData:` — the op item stores base64, same
   as today's `secret-op-credentials` blob. Getting this wrong silently breaks 1Password
   Connect on every fresh cluster.

3. **Multi-arch image ref.** Fleet is mixed amd64 (dell01) + arm64 (rpi01/tp*). The single
   `controller_image.tag` MUST resolve to a multi-arch manifest list, or arm64 controlplanes
   pull the wrong arch / fail. nostos render emits one `image:` string for all nodes; correctness
   is a CI concern (build+push manifest list), but render should NOT append `@sha256:` or per-arch
   suffixes. Document that the tag must be a manifest list. (Contrast: `Cluster.ImageDigests`
   pins per-arch Talos installer digests — do NOT mimic that for the controller image; rely on
   the manifest list.)

4. **Idempotency.** `nostos render` is contractually idempotent (AGENTS.md table). Re-rendering
   must be byte-identical: (i) sort `root_secret.data` keys, (ii) sort/serialize the bootstrap
   config deterministically (yaml.Marshal of a struct is stable; avoid ranging maps without
   sorting — `cilium.values` is `map[string]any`, so marshal via yaml.v3 which sorts map keys, or
   accept its ordering as long as it's stable across runs), (iii) preserve template document order
   when re-joining. On a multi-controlplane cluster all controlplanes carry identical synthesized
   manifests; cluster-init applies once, the rest no-op — fine.

5. **Pre-CNI registry reachability.** Controller pod runs `hostNetwork: true` and pulls its image
   before any CNI. Public ghcr over the host network is fine on dell01's LAN; offsite rpi01 pulls
   over Tailscale at boot — confirm the tailnet route is up before the kubelet tries the pull
   (controller design §8.4). Render can't fix this; flag it in docs.

6. **`text/template` collision.** Templates already run through `text/template` with
   `missingkey=error` (`renderTemplateBody`). The controller bundle const contains `{{ .Image }}`
   — render THAT in a **separate** `text/template` invocation inside `controllerBundleManifest`,
   NOT in the node-template pass, so the two `{{...}}` namespaces never collide. The synthesized
   YAML is spliced AFTER `renderTemplateBody`, so its braces are never seen by the node pass.

---

## 5. File-by-file change list

| File | Change |
|---|---|
| `internal/config/config.go` | Add `Bootstrap *Bootstrap` field to root `Config` struct (yaml `bootstrap,omitempty`). |
| `internal/config/bootstrap.go` *(new)* | Define `Bootstrap`, `BootstrapCilium`, `BootstrapArgocd`, `BootstrapRepo`, `BootstrapImage` (+`Ref()`), `BootstrapRootSecret`. Validator tags. Optionally a `Bootstrap.Validate()` for cross-field checks (e.g. controller image tag non-`latest` warning). |
| `internal/config/config.go` (`Validate`) | When `c.Bootstrap != nil`, ensure `root_secret.data` values are `Ref` (scheme-checked already via `Ref.UnmarshalYAML`); optionally warn if image tag is `latest` (non-idempotent). |
| `internal/registry/registry.go` (`Render`) | Insert step 3.5: `if cfg.Bootstrap != nil && node.Role == "controlplane" { templated, err = injectBootstrapManifests(templated, cfg) }` BEFORE `secrets.ResolveTemplate`. Add double-emission guard (fail if template still hand-writes `inlineManifests`/`extraManifests` while `bootstrap:` set). |
| `internal/registry/bootstrap.go` *(new)* | `injectBootstrapManifests(body string, cfg *config.Config) (string, error)`; helpers `rootSecretManifest`, `bootstrapConfigManifest`, `controllerBundleManifest`; `splitYAMLDocuments`, `indentLines`, `sortedKeys`; the `controllerBundleTmpl` const. Uses `gopkg.in/yaml.v3` (already a dep) for parse/marshal of the first doc. |
| `internal/registry/bootstrap_test.go` *(new)* | Golden tests: nil bootstrap = unchanged template; non-nil = 3 manifests appended in order; op:// preserved through marshal then resolved by a stub backend; idempotent re-render (byte-identical); double-emission guard fires. |
| `internal/cli/schema` (registry of methods) | If `render`'s schema descriptor enumerates config knobs, add the `bootstrap:` block so `nostos schema render` documents it (consistency with AGENTS.md output contract). |
| `nostos/config.yaml` *(parent repo)* | Add the `bootstrap:` block (example in §1). |
| `nostos/templates/dell01.yaml` *(parent repo)* | DELETE `cluster.extraManifests` + `cluster.inlineManifests` (all 5 blobs). Keep `cni.name: none`. |
| `nostos/README.md` / `.submodules/nostos/AGENTS.md` *(docs)* | Document: bootstrap-tier change only takes effect on fresh cluster-init; image-must-exist-first prerequisite; multi-arch manifest-list requirement; re-trigger = `nostos up`. |

### Key real symbols referenced
- `registry.Render` (`internal/registry/registry.go`) — insertion point.
- `registry.renderTemplateBody` / `templateData{InstallImage}` — the existing text/template pass; bootstrap injection runs AFTER it.
- `secrets.ResolveTemplate` / `secrets.URIPattern` / `secrets.BuildBackends` — unchanged; resolves op:// in the injected Secret.
- `secrets.OnePasswordBackend.Resolve` (`internal/secrets/op.go`) — returns raw `op read` output (already base64 for the credentials), so `data:` stays correct.
- `config.Config`, `config.Ref` (+`UnmarshalYAML` scheme allowlist), `config.Config.Validate` — extended with `Bootstrap`.
- `config.Node.Role` — gate injection to `controlplane`.

---

## 6. Open questions (defer to controller design / Yuri)
- ConfigMap namespace `kube-system` vs a dedicated `nostos-system` (controller would have to create it first; `kube-system` always exists). Recommend `kube-system`.
- Whether `root_secret` should be generalized to a list (multiple root secrets) — start with one (irreducible) per controller design §8.3.
- Whether to ship a `--verify-image` flag now (Risk §4.1) or defer. Recommend defer; document the prerequisite.
- ClusterRole scope: ship wide (`*/*/*`) for v0, tighten post-bootstrap (Risk §4.9 of controller design).
```
