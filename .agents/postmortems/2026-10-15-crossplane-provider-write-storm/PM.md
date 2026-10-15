---
date: 2026-10-15
status: closed
incident_status: mitigated
sessions:
  - 01a13bf7-a671-741e-ac81-4cb5eaf51bdf
components:
  - crossplane
  - kube-apiserver
  - argocd
  - macintel01
  - external-secrets
  - longhorn
  - pc01
symptoms:
  - KubePodCrashLooping for buzz (1222 restarts / 4d), presenter (~6d, Bun "error: Unexpected }"), argocd-image-updater (leader-election exits)
  - NodeCPUCritical on macintel01 (100–104% sustained) with KubeAPIServerSlow and ControlPlaneAPIUnreachable
  - ArgoCD app stuck health=Degraded / op=Error: ".spec.template.spec.containers[name=relay].env: duplicate entries for key [name=REDIS_URL]"
  - buzz relay "Redis pub/sub error: Connection refused (os error 111)" every 30s; readiness 503 {"redis":false}
  - NodeFilesystemSpaceFillingUp pc01 /var 77.3% used
  - KubePersistentVolumeCriticalFull obsidian-config PVC 0.0% space left
  - external-secrets + cert-controller CrashLoopBackOff "Failed to update lease" (121+ restarts over 27h)
failure_mode: crossplane-provider-status-write-storm
affected_urls:
  - https://buzz.syscd.live
  - https://api.k8s.lan:6443
tk: [tk-kft, tk-gjx, tk-ndu, tk-ac9, tk-qpb]
memories: [crossplane-provider-write-storm-2026-10-15]
supersedes: []
related:
  - 2026-08-16-crossplane-finalizer-retry-storm
  - 2026-10-14-pc01-qemu-memory-overcommit-wedge
---

# Postmortem: provider-proxmox-bpg status-write conflict storm + stale GitOps fixes pile up as a 10-alert alertmanager backlog

- **Severity/Impact:** buzz relay down 3d10h+ (1222 crashloop restarts), presenter down ~6d
  (broken image published 2026-10-09), argocd-image-updater flapping; ESO crashlooping 27h+
  degraded secret sync cluster-wide; obsidian-config PVC full (writes failing); pc01 `/var`
  77% and predicted full within 24h; kube-apiserver on macintel01 pinned at ~3 cores with
  Handler timeouts. No data loss. The user surfaced it as "address alerts in alert manager"
  — 8 active alerts, every one real.
- **Root cause (one line):** crossplane-provider-status-write-storm — provider-proxmox-bpg
  v1.15.0's async-callback status writes conflict with its own reconcile loop on the three
  git-declared EnvironmentVMs (~90 resourceVersion bumps / 8s), saturating the 8-vCPU
  control-plane VM; the crashloops underneath were three independent stale fixes that had
  never reached the cluster (unmerged branch, broken image, missing REDIS_URL).

## What Happened

Three apps had been crashlooping for days behind alerts nobody actioned: **presenter**
because the 2026-10-09 teleprompter commit deleted the `app.delete(...)` route header from
`server/src/index.ts` (bare syntax error — Bun refuses to start; the broken `:latest` image
was built and deployed anyway); **buzz** because its GCS S3-interop git-conformance probe
went fatal on 2026-10-11 when the bucket's GCP billing account was disabled (403
`UserProjectAccountProblem`) — the approved fix (disable probe) landed in the values repo
and synced 2026-10-14 11:21Z, but the rollout wedged: image 0.1.0 gates readiness on Redis,
no `REDIS_URL` was configured, so the surge pod stayed 503 forever and the old ReplicaSet
kept its crashlooping pod at `replicas: 1`; **image-updater** because its leader-election
fix sat on an unmerged branch since 2026-09-05.

Meanwhile **provider-proxmox-bpg v1.15.0** was hot-looping status writes on the three
git-declared `EnvironmentVM`s (`workstation`, `windows`, `talos-pc01`): every async-callback
status update lost a race against the reconcile loop ("cannot update status … the object has
been modified"), logging `callback failed` continuously — ~90 resourceVersion bumps in 8s.
That write storm drove kube-apiserver to ~3 cores and the control-plane VM to 100–104% CPU,
which in turn cost ESO its leader-election leases (27h+ crashloop) and made every API path
slow. The pc01 disk alert (image bloat from the 2-day VM-start loop of the sibling
incident) and the obsidian PVC filling to 0.0% rounded out the queue. Notably, the 2026-08-16
postmortem had closed with "API latency and high control-plane CPU persist from a separate
cause" — this storm is a candidate for that cause, though a real residual remains (below).

## Detection Gap (how we catch it next time)

- **What the user saw first:** the alertmanager backlog itself — the user asked to address
  it. Detection fired correctly (all critical alerts routed to Discord); what failed was
  *landing fixes*: two fixes were committed but never reached the cluster, and nothing
  watches that seam.
  - image-updater's fix lived on an unmerged branch for **5 weeks** — invisible to both
    ArgoCD and alerts.
  - buzz's `REDIS_URL` commit sat unsynced for **~17h** while ArgoCD showed
    `Synced` + a stale `operationState: Error` (the duplicate-env structured-merge diff
    error). `ArgoCDClusterCacheDown` only covers the cluster-cache seam, not per-app sync.
- **How we detect it before the user next time:** ArgoCD exports per-app sync metrics
  (scraped since the 2026-07-05 incident via `argocd-vmservicescrape.yaml`). Two uncovered
  signals: a failed sync operation (`argocd_app_sync_total{phase="Error|Failed"}`) and an
  auto-sync app that stays `OutOfSync` (reconcile loop broken → primary alert candidates).
- **Fix path once detected:** force refresh + fix the diff the ComparisonError names; for
  unmerged branches, the repo rule "don't leave hanging worktrees/branches" applies —
  audit task proposed in the plan.

## Mitigation (runbook — how to detect & fix this again)

**Provider write storm (macintel01 at 100%, provider pod burning 2+ cores):**

1. Confirm the storm:
   `kubectl logs -n crossplane-system -l crossplane.io/provider=provider-proxmox-bpg --since=5m | grep -c "callback failed"` —
   dozens/minute = storm. Signature error: `cannot update status of the resource
   virtualenvironmentvm... after an async update: Operation cannot be fulfilled ... the
   object has been modified`.
2. Measure churn:
   `kubectl get environmentvms -A -o yaml | grep -c resourceVersion` twice, 8s apart
   (≥30 bumps/interval = storm). `kubectl get events` won't show it; the provider log is
   the signal.
3. Stop it — pause the MRs (they are git-declared in
   `k8s/charts/crossplane-proxmox/templates/*.yaml`, so pause in git AND annotate live if
   the controller reconcile is starved):
   `kubectl annotate environmentvm <name> -n crossplane-system crossplane.io/paused=true`.
   Provider CPU drops from ~2.5 cores to ~3m within seconds. VMs keep running; only
   reconciliation stops.
4. Restarting the provider pod does NOT help — the loop resumed 2 minutes after a pod
   restart, with the same workspace IDs. It's a version bug (upjet async-callback race),
   not transient state. Upgrade the provider, then remove `crossplane.io/paused`.
5. After the pause, expect a ~10 min API drain window: handler timeouts and etcd write
   timeouts tail off, ESO leader elections re-stabilize. Verify etcd caught up with
   `nostos`/`talosctl etcd status` (raft index == applied index).
6. Standing condition, separate from any storm: apiserver burns ~3 cores with **zero**
   client traffic, a fresh apiserver pod rebaselines at ~18k goroutines (cluster-scale
   baseline, not a leak), steal=0, burn is system/softirq-bound, load avg ~41 on the
   8-vCPU UTM VM. It does not self-heal; sizing/migration is tracked separately.

**ArgoCD "fix committed but never landed":**

1. `kubectl get application <app> -n argocd -o jsonpath='{.status.operationState.message}
   {.status.conditions}'` — a `ComparisonError` naming a duplicate map key means the Helm
   render and your overlay both set the same env var; ArgoCD refuses to diff, and with
   stale `comparedTo` the app can still display `Synced`.
2. ArgoCD renders the chart's own `secretKeyRef` for `REDIS_URL` (optional) — supply the
   value in the **secret** (1Password item → ExternalSecret property), never as a
   `relay.extraEnv` literal alongside it.
3. Sync ops are forced with
   `kubectl patch application <app> -n argocd --type merge -p '{"spec":{"operation":{"sync":{}}}}'`.
4. Chart-injected env vars resolve at **pod start**: after the secret finally syncs, the
   27h-old pod still has the old (empty) env — restart the deployment.

**Syncs fail fleet-wide under saturation ("failed to download openapi"):**

- Signature in `kubectl -n argocd logs argocd-application-controller-0`:
  `Apply failed ... dryRun=true message="error validating data: failed to download openapi
  ... request canceled (Client.Timeout or context cancellation while reading body)"`,
  repeated across different apps and resources; the sync operation stays `Running`
  cycling retries, then `retried 5 times` and the op fails.
- Cause: kube-apiserver on macintel01 streams `/openapi/v2` (22MB) at ~1.4MB/s under its
  standing system/softirq load; ArgoCD's client-side dry-run validation downloads that
  schema per apply and times out mid-body. `ServerSideApply=true` does NOT bypass it
  (the client still validates first). Fresh-from-workstation fetch took 16s — the
  controller's client timeout is shorter than that.
- Fix per app (or on the parent to cover all): add `Validate=false` sync option —
  server-side admission validation still applies. Applied 2026-10-15 to
  `support-cluster` (git) and the parent `applications` (live CR).
- The parent `applications` Application (directory-source of `k8s/applications/`, auto
  sync+selfHeal) is **out-of-band** — it is not declared anywhere in git (it cannot be:
  its source dir would recursively contain it). Live-patch its CR; it is the source of
  truth for that app. Fleet-wide `Validate=false` rollout not yet done — consider it when
  macintel01 sizing lands (tk-qpb).

**ESO collateral (crashloop "Failed to update lease"):** ESO loses leader election whenever
the API degrades; its backoff then outlives the recovery. Delete the pods once the API is
stable, and restart the webhook if cert-controller logged fresh CA injection
(`healthz check failed` with a stale webhook pod).

**PVC full (obsidian-config 0.0%):** expand via git
(`k8s/applications/obsidian.yaml` → longhorn-single allows online expansion), then expect
one `VolumeResizeFailed` RPC timeout from the external-resizer before Longhorn accepts the
expand; `FileSystemResizeRequired` completes automatically via kubelet.

## Dead Ends

- **`relay.extraEnv` REDIS_URL literal** — collided with the chart's own optional
  `secretKeyRef` for the same name: ArgoCD's structured-merge diff rejected every sync for
  ~17h ("duplicate entries for key [name=REDIS_URL]") while the app displayed `Synced`.
- **Provider pod restart** — assumed transient state; loop resumed in 2 min with identical
  workspace IDs. Version bug, not state.
- **"18k goroutines = leak"** — restarted the apiserver; the fresh pod rebaselined at the
  same ~18k. It's this cluster's baseline.
- **"Host contention"** — CPU steal on macintel01 = 0 (UTM host is not oversubscribing);
  the burn is real guest work, system/softirq-bound.
- **argocd CLI over port-forward** — flaky (`error dial proxy`, `configmap "argocd-cm" not
  found`); kubectl patches of the Application CR do everything the CLI would.
- **`kubectl delete pod omniroute-...`** — already gone; that crashloop self-resolved.
- **`talosctl` for disk inspection** — no `df` command, and the talosconfig targets
  dell01 with "no request forwarding" to other nodes; `kubectl top`/`crictl`-free paths
  were used instead.
- **Longhorn attach for the new presenter pod** — `FailedAttachVolume ... context deadline
  exceeded` looked like a storage fault; the volume cycled detach→reattach on its own and
  attached cleanly. Self-healed, no action.

## Timeline

### 2026-09-05 → 10-09 (pre-incident, UTC)
- `09-05 21:19 -03` image-updater leader-election fix committed on branch
  `image-updater-args-fix` (54bb7cef) — **never merged**.
- `09-09 14:42Z` presenter teleprompter commit (c83b0f6) CI-builds and publishes a broken
  `:latest` (syntax error in `server/src/index.ts`); deploy crashloops from this point.

### 2026-10-11
- `~09:30Z` buzz pod evicted/restarted; GCS conformance probe now fatal (bucket billing
  disabled) → crashloop begins (1000 restarts over 3d10h).

### 2026-10-12 → 10-14 (sibling incident context)
- `10-12 12:20Z` pc01 VM 100 start-loop begins (see 2026-10-14 pc01 postmortem); provider
  reconciles against an unreachable host for 2 days — plausible storm trigger, unproven.

### 2026-10-14
- `~09:50Z` macarm01 rejoined Ready after ~10h (context: ESO pods rescheduled onto it).
- `11:21Z` buzz app synced the conformance-probe fix (values repo); rev6 rollout begins.
- `~12:00Z` buzz surge pod starts; readiness 503 `{"redis":false}` — REDIS_URL unset, dials
  `redis://127.0.0.1:6379`; rollout wedges with old RS crashlooping at replicas=1.
- `19:41Z` `NodeFilesystemSpaceFillingUp` fires for pc01 `/var` (22.7% left).
- `~19:45Z` User: "address alerts in alert manager". Sweep finds 8 active alerts: pc01
  disk, macintel01 CPU saturated, API slow/unreachable, crashloops (image-updater,
  presenter, buzz).
- `~19:50Z` kube-apiserver at 3017m; `provider-proxmox-bpg` at 2173m on the same node.
- `~20:05Z` Diagnosis: presenter Bun syntax error; buzz readiness gated on Redis; both
  fix branches identified (one never merged).
- `~20:25Z` Pushed eacfff94 (buzz REDIS_URL via extraEnv — later reverted) + 2064cc03
  (cherry-picked image-updater fix to main).
- `~20:30Z` image-updater synced → Running, 0 restarts (stable from here on). buzz ArgoCD
  shows stale `comparedTo`; forced refresh; sync op fails: duplicate REDIS_URL diff error.

### 2026-10-15
- `~13:20Z` Duplicate-env diagnosis complete: revert literal, add `REDIS_URL` property to
  the ExternalSecret, add the field to the 1Password `buzz` item; pushed 38bc5df4.
- `~13:37Z` ESO refresh attempts land during API degradation; secret not yet updated.
- `13:57Z` presenter fix (25f0a9a) CI-builds clean (`:latest` rebuilt).
- `14:21Z` Forced buzz sync via `spec.operation` patch (previous op showed Error since
  14:01; ComparisonError cleared after refresh).
- `~14:25Z` Provider pod restart: conflict loop resumes within 2 min (same workspaces);
  RV churn measured ~90/8s; argocd-controller identified as the (legitimate) competing
  Apply writer on the git-declared EnvironmentVMs.
- `~14:32Z` Paused all three EnvironmentVMs: git 4df62139 + live annotations (controller
  reconcile starved, so ArgoCD couldn't land it). Provider 2475m → 3m; RVs frozen; handler
  timeouts stop; etcd raft index == applied index.
- `14:31Z` buzz sync op completes; ExternalSecret spec updated on cluster.
- `14:43–14:52Z` ESO controller + cert-controller restarted (27h crashloop backoff reset);
  leader election stabilizes after the API drain; webhook restarted for fresh CA.
- `14:54Z` `buzz-secrets` gains `REDIS_URL` (ESO sync).
- `14:55Z` buzz rollout restart → **Ready 14:58Z**, "Redis pub/sub connected".
- `~15:00Z` presenter rollout: Longhorn attach hiccup self-heals (detach→reattach); new
  image pulled; **Ready ~15:07Z**.
- `~15:05Z` obsidian-config PVC alert (0.0%): expanded 20→40Gi via git 3ad4ba50 + live
  patch; one resizer RPC timeout, then expand + filesystem resize complete (cap=40Gi).
- `~15:15Z` Final sweep: remaining alerts are stale pod refs (auto-expiring), the
  obsidian pair (resolving), omniroute (self-resolved), and the standing macintel01 trio.
- `~15:30Z` apiserver still ~3 cores at zero client traffic; apiserver pod restarted —
  rebaselines at ~18k goroutines; load avg 41; steal=0 → standing condition booked
  (macintel01 sizing), storm paused pending provider upgrade.
