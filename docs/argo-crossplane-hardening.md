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

## Placement policy

Control-plane placement is an **exception, never a default**. It is allowed
only for a small, bounded singleton when all four conditions hold:

1. It is necessary to recover or operate the control plane itself.
2. It has explicit CPU/memory requests **and** limits.
3. It does not run a heavy cluster-wide LIST/WATCH/reconcile loop.
4. Its manifest documents why it needs the control plane and the capacity risk.

The node's local API path can make such a component more reliable in this
stretched cluster, but control-plane CPU, memory, disk I/O, and network are
finite and shared with kube-apiserver, etcd, kubelet, and CNI. Providers,
databases, telemetry, and normal workloads stay off it. First approved
exception: crossplane-rbac-manager (100m/256Mi request, 500m/512Mi limit),
because it supplies provider RBAC/SafeStart and formerly crashed through the
cross-LAN Service VIP.

## Workstreams

### A. Crossplane core reliability

- [x] **A1. rbac-manager: same remedy as core** — `rbacManager.leaderElection:
      false` + `extraEnvVarsRBACManager` direct-apiserver, in
      `k8s/applications/crossplane.yaml`. *(done 2026-08-15, 38884ab6)*
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

## macintel01 host-level CPU attribution (card home-systems-cex.3, 2026-08-16)

The 97% reading in the postmortem is reproducible now. Read-only Talos
diagnostics over a 221s window (load avg 28-39 on 8 CPUs) attribute the
host-level CPU as follows:

| consumer | cores | note |
|---|---|---|
| kube-apiserver | ~2.2 | serving all clients + etcd fanout |
| **tailscaled** | **~1.2** | **host-level only; invisible to pod metrics** |
| etcd | ~1.1 | raft agreement latency (see below) |
| cilium-agent | ~0.66 | |
| kube-controller-manager | ~0.37 | |
| kubelet | ~0.31 | |
| containerd, /sbin/init (Talos) | ~0.33 | system daemons, not pods |
| tail (scheduler, envoy, coredns, operator, metallb, crossplane, ...) | ~1.6 | each <0.1 cores |

Sum ≈ 7.8 of 8 cores — matches the 97% load. The "unaccounted" host CPU is
**tailscaled (1.2 cores) plus the Talos system daemons (init/containerd/kubelet
at ~0.7 cores)**; all the rest is pod-metrics-visible.

**Root cause of tailscaled burn:** ext-tailscale logs show continuous
connection-tracker churn against stale/unreachable peers (`derp-N does not know
about peer [ArzyN], removing route`, `open-conn-track: timeout opening ...
online=no, lastseen=254h`, `open-conn-track: flow TCP got RST by peer`) and
repeated disco re-pins. The 2026-07-12 Tailscale↔Cilium endpoint-recursion
postmortem is the same class. This is a *network health* symptom, not a sizing
one.

**etcd is latent:** `apply request took too long ... 217-315ms` warnings on
simple reads (`agreement among raft nodes before linearized reading`),
consistent with cross-site etcd peer links over the Tailscale mesh.

### Bounded next step (no control-plane workloads added)

1. **Read-only:** `talosctl -n <cp> logs ext-tailscale | grep -E
   'derp.*removing route|open-conn-track: timeout|disco: node .* now using'` on
   macintel01/dell01 to confirm the stale-peer churn correlates with the tailscaled
   CPU spikes; then decide whether to flush the stale offline peers from the
   Tailscale coord server (user action on tailnet admin console).
2. **Later (separate card):** reduce etcd raft RTT between control-plane nodes —
   the two members are cross-site over Tailscale; consider co-locating or adding
   a third voter so no single site is a quorum minority. Do NOT add workloads
   to the control plane to do this.

Verified: `talosctl processes` sampled twice 221s apart; loadavg 28.46→39.34;
etcd slow-apply warnings present. No nodes/workloads restarted or mutated.

## Status log

- 2026-08-15: Incident recovered. Submodule checkout disabled for Argo repo
  server (833666ea). Stale pc01 pin removed live + ProviderRevision recreated;
  storage provider stable on dell01. MRAP activations added as workaround
  (67124422, aea72ff1). Broad `*.upbound.io` exclusion live (aea72ff1) —
  pending B1 verification and B5 decision. Starting A1 (rbac-manager).
- 2026-08-15 (later): A1 shipped and verified — rbac-manager rolled to a new
  pod (dell01) with LEADER_ELECTION=false + direct apiserver env; 0 restarts
  after the 767-restart loop. crossplane app Synced/Healthy. Next: A2
  (SafeStart verification) and B1 (glob exclusion verification).
