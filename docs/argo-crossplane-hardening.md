# Argo CD × Crossplane hardening plan

Working document tracking the improvements from the 2026-08-15 incident
(submodule sync error → discovered every app jammed by a dead Crossplane
conversion webhook). Check items off as they land; keep the status log current.

## The failure chain (why every app went Unknown)

Four links, all had to fail. Fixing any one of them shrinks the blast radius;
fixing the first eliminates the class.

1. **Two-LAN Service VIP unreliability (root).** Connections to the kube-proxy
   VIP `10.96.0.1` time out from nodes on the wrong LAN. Already documented and
   patched for providers (`gcp-storage-pinned`) and crossplane core
   (`extraEnvVarsCrossplane`), but NOT for `crossplane-rbac-manager`, which
   crash-looped for 3+ days (700+ restarts, "leader election lost") on rpi01.
2. **Stale ProviderRevision.** Crossplane does not re-render a revision's
   Deployment when its `DeploymentRuntimeConfig` changes. The live revision kept
   the old `kubernetes.io/hostname: pc01` pin long after git moved to arch
   affinity; when pc01 died, the storage conversion webhook lost its only
   endpoint.
3. **rbac-manager down poisons provider recovery.** A new ProviderRevision gets
   its RBAC from rbac-manager. Without it, the provider lacks CRD-watch
   permission → **SafeStart is disabled** → the provider demands every kind it
   owns → crashes against the intentionally-minimal MRAP allow-list. With
   SafeStart on, providers tolerate inactive MRDs and the whole
   "activate CRDs until it stops crashing" class disappears.
4. **Argo CD global cache coupling (upstream design).** One unlistable CRD
   fails the whole cluster cache; every Application goes Unknown/ComparisonError
   (argo-cd #20828, #4155, design discussion #13239). This also creates the
   bootstrap deadlock: the fix for Argo ships through Argo.

## Workstreams

### A. Crossplane core reliability

- [ ] **A1. rbac-manager: same remedy as core** — `rbacManager.leaderElection:
      false` + `extraEnvVarsRBACManager` direct-apiserver, in
      `k8s/applications/crossplane.yaml`. *(in progress, this session)*
- [ ] A2. Verify SafeStart stays enabled after A1: provider logs must NOT show
      "SafeStart capability will be disabled"; delete/recreate of a
      ProviderRevision must come up with RBAC within seconds.
- [ ] A3. Reconsider the two MRAP activations added during the incident
      (`objectaccesscontrols.storage.gcp.m.upbound.io`,
      `bucketiampolicies.storage.gcp.upbound.io`). They were workarounds for
      SafeStart being off; nothing in git consumes them. Note: Crossplane
      forbids Active→Inactive, so removal means deleting the CRDs by hand —
      only do this deliberately.
- [ ] A4. Recover or formally drain pc01 (`node.kubernetes.io/out-of-service`).
      amd64 capacity is currently dell01 only — every amd64-pinned webhook is
      one node failure away from a repeat.

### B. Argo × Crossplane integration (align with the official guide)

Ref: https://docs.crossplane.io/latest/guides/crossplane-with-argo-cd/

- [ ] B1. **Verify glob support in `resource.exclusions` apiGroups.** Live
      config now uses `apiGroups: ["*.upbound.io"]` (commit aea72ff1) while an
      older comment claimed apiGroups match exactly. One is wrong. Test: with
      the storage provider scaled down, confirm the Argo controller cache still
      syncs. If globs don't match, the cluster is unprotected against the next
      webhook outage — rewrite as explicit group list.
- [ ] B2. Set `application.resourceTrackingMethod: annotation` in argocd-cm
      (recommended by Crossplane; avoids label-length and tracking issues).
- [ ] B3. Add Crossplane health customizations (`resource.customizations` Lua
      for `*.upbound.io` / `*.crossplane.io`) so Provider/MR health is visible
      in Argo even where diffing is excluded.
- [ ] B4. Exclude `ProviderConfigUsage` (all provider groups) per the guide.
- [ ] B5. Decide the exclusions end-state. Upstream guidance: broad provider
      exclusions defeat GitOps management of MRs. Counterpoint for this
      cluster: MRs sync via ServerSideApply and Crossplane owns reconciliation,
      so the cost is visibility, not correctness. Either keep the broad
      exclusion **documented as a deliberate availability trade** or narrow to
      webhook-converted groups only. Record the decision here.
- [ ] B6. Evaluate `ARGOCD_K8S_CLIENT_QPS` (default 50 → 300) for this
      CRD-heavy cluster; watch apiserver load after (single control plane).

### C. Conversion-webhook availability

- [ ] C1. Run 2 replicas of webhook-serving providers (storage, iam) across
      dell01+pc01 once pc01 is back. **Precondition:** verify the GCP provider
      version serves `/convert` on non-leader replicas (upjet fixed this class
      in provider-upjet-aws v2.7.0; confirm for provider-upjet-gcp v2.5.4
      before trusting replicas>1).
- [ ] C2. Pin provider package versions deliberately; treat provider upgrades
      as availability changes (webhook readiness/registration fixes land in
      provider releases). Renovate PRs for providers get a manual gate.
- [ ] C3. Runbook: temporary, exact-GVK exclusion as incident containment
      (never permanent group-wide masking without decision B5).

### D. Lifecycle layering (sync waves)

- [ ] D1. Audit waves: crossplane core (-2) → crossplane-providers →
      provider readiness → ProviderConfig/credentials → managed resources
      (crossplane-gcp). Add explicit waves where missing so provider
      infrastructure is Ready before MR syncs, avoiding install/upgrade races.

### E. Detection (alert before "everything is Unknown")

- [ ] E1. Alert: any `provider-*` webhook Service in crossplane-system with
      zero ready EndpointSlices for >5m.
- [ ] E2. Alert: `Provider.pkg` Healthy=False for >10m.
- [ ] E3. Alert: crossplane / rbac-manager restart storms (restarts > 5 in 1h).
      rbac-manager restarted 700+ times over 3 days with zero signal.
- [ ] E4. Alert: Argo application-controller cluster cache sync failures
      (`argocd_app_reconcile` stalls / controller log error rate).
- [ ] E5. Dashboard/alert on apiserver conversion webhook metrics
      (`apiserver_crd_conversion_webhook_duration_seconds` errors/latency).
- [ ] E6. Drift check: ProviderRevision pod template vs its
      DeploymentRuntimeConfig (link 2 was silent drift; a periodic diff catches
      it).

## Status log

- 2026-08-15: Incident recovered. Submodule checkout disabled for Argo repo
  server (833666ea). Stale pc01 pin removed live + ProviderRevision recreated;
  storage provider stable on dell01. MRAP activations added as workaround
  (67124422, aea72ff1). Broad `*.upbound.io` exclusion live (aea72ff1) —
  pending B1 verification and B5 decision. Starting A1 (rbac-manager).
