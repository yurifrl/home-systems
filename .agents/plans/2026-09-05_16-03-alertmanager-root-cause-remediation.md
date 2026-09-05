# Alertmanager root-cause remediation

Clear the firing criticals by fixing their root causes (CNPG degraded, pc01 resilience,
image-updater arg bug, hermes backup token, chronic API slowness), then cut alert noise.
pc01 recovered since the analysis (all 7 nodes Ready) — its workstream is now prevention,
not restore. Data window of the original analysis: 2026-08-30 → 09-05.

## Context

- Alertmanager `vmalertmanager-vmks` (monitoring): all alerts are `severity=critical`; 0 silences.
- Rules live in `k8s/charts/support-cluster/templates/monitoring/` (e.g. `control-plane.yaml`,
  `nic-offload-fix.yaml`); all changes ship via git → ArgoCD (never `kubectl apply`).
- Relevant postmortems: `.agents/postmortems/2026-08-16-crossplane-finalizer-retry-storm/PM.md`,
  `2026-08-18-pc01-containerd-pleg-dead/PM.md`, `2026-08-10-longhorn-daemonsets-excluded-from-dell01/PM.md`.
- Standing rule: no node favorites or exclusions — Kubernetes schedules everywhere; nodes must
  degrade gracefully (evict/stop admitting) instead of dying.

## Steps

### 1. databases/pg (CNPG) — live fire: pg-3 timeline split + `buzz` down

- [ ] 1.1 Remove node favorites/exclusions from `k8s/applications/postgres.yaml`: delete the
      `nodeAffinity: NotIn [macarm01, macintel01, rpi01]` block (lines ~56–69, comment "Keep
      instances OFF the flaky roaming VMs"). Keep `enablePodAntiAffinity` + hostname
      `topologyKey` (that's spread, not exclusion). If a replica was flaky on a node, the fix is
      the node (see step 2), not a scheduling ban. Commit + push, let ArgoCD sync.
- [ ] 1.2 pg-3 (0/1, 201 restarts, pg_controldata `shut down in recovery`, TimeLineID 9) is a
      stale-timeline replica stuck in a restart loop — reinit it from backup via CNPG
      (`.spec.bootstrap` reinit / delete PVC + pod) instead of letting it keep crashing.
- [ ] 1.3 Diagnose pg-2 (0/1, 12 restarts, no restart in 6h = stuck, not flapping): check
      readiness logs and replication status vs pg-1.
- [ ] 1.4 `kubectl -n databases exec pg-1 -- psql -Atc '\l' | grep buzz` — does the `buzz` DB
      exist on the primary? If missing: restore from CNPG/Longhorn backup, or recreate and let
      buzz migrate (check the buzz chart for init/migration behavior first), then
      `kubectl -n buzz rollout restart deploy/buzz`.
- Verify: CNPG `readyInstances: 3`; buzz 1/1; `KubePodCrashLooping(databases)` + buzz crashloop
  alerts clear.

### 2. pc01 — prevention: degrade gracefully under resource pressure, never die

pc01 is back (Ready, system pods 1/1, `provider-gcp-iam` scheduled). The failure mode recurs
(PM-2026-08-18: containerd hang → PLEG death → NotReady, trigger unknown; kubelet defaults evict
far too late, so the node hangs instead of shedding load). Goal: when resources run out, kubelet
evicts pods and stops scheduling — the node stays Ready.

- [ ] 2.1 Edit `nostos/templates/talos-pc01.yaml` kubelet section: add tuned
      `evictionHard` + `evictionSoft` (memory.available ~10%, nodefs/imagefs ~15%) and
      `systemReserved` + `kubeReserved` sized to the node's system daemons, so pressure is
      detected and shed before containerd/systemd starve.
- [ ] 2.2 `nostos render pc01 && nostos apply pc01` (pc01 is nostos-managed now — the legacy
      `talos/` dir no longer exists); confirm the kubelet picks it up (`Allocatable` on
      `kubectl describe node pc01` drops by the reserved amounts) and the node rolls without
      going NotReady.
- [ ] 2.3 Audit workloads on pc01 for declared resource requests/limits (GPU workloads,
      longhorn engine images) — kubelet can only stop admitting when admission reflects real
      usage.
- Verify: next resource squeeze (or forced stress test) produces pod evictions + `KubeEvicted`-style
  signal, not `KubeNodeNotReady`; pc01 survives where it previously hung.

### 3. argocd-image-updater crashloop — 5,897 restarts, one-line fix

- [ ] 3.1 Root cause: pod args `["--metrics-bind-address=:8443","run"]` → container prints usage
      and exits (flag unsupported by image v1.2.1; chart 1.2.4 from
      `k8s/applications/argo-image-updater.yaml`). Find the value that renders that arg via
      `helm template` (render-only).
- [ ] 3.2 Fix `k8s/applications/argo-image-updater.yaml` valuesObject (drop the flag or use one
      this version supports), commit + push, wait for ArgoCD sync, check
      `operationState.message` for errors.
- Verify: pod 1/1 with stable RESTARTS; `KubePodCrashLooping(argocd)` clears.

### 4. hermes memory-backup — verify the token chain before rotating anything

The PAT was recreated on 2026-08-10 (PM-2026-08-10) with Contents: R/W — it *should* have
permission. Find which link broke instead of blindly rotating.

- [ ] 4.1 Capture the current error: `kubectl -n hermes logs hermes-0 -c memory-backup
      --previous | tail -20` (last seen: clone "Repository not found" → `hctl` crashes on missing
      `/backup/hermes-memory/.gitignore`).
- [ ] 4.2 Audit the chain: diff the `GH_TOKEN` in Secret `hermes-env` (ns `hermes`, synced by ESO
      from 1Password item `hermes-env`) against the 1Password value (drift check); with that
      token call `GET /user` (identity — PM-2026-08-16 caught it authenticating as `mrag23`)
      and `GET /repos/yurifrl/hermes-memory` (Permissions block). If token + repo check out,
      look at the `git-login` init container / active gh account instead.
- [ ] 4.3 Fix the broken link (rotate PAT in 1Password only if it truly lacks Contents: R/W;
      recreate the repo if renamed), force ESO resync
      (`kubectl -n hermes annotate externalsecret hermes force-sync=$(date +%s) --overwrite`),
      restart the container.
- Verify: clone+push in memory-backup logs; hermes-0 4/4; `HermesDown` +
  `KubePodCrashLooping(hermes)` clear.

### 5. Chronic control-plane slowness — `KubeAPIServerSlow` + `ControlPlaneAPIUnreachable` fired daily all window

- [ ] 5.1 Follow `.agents/postmortems/2026-08-16-crossplane-finalizer-retry-storm/PM.md`:
      macintel01 CPU, kube-apiserver handler timeouts, stuck CRD finalizers (Proxmox
      ProviderConfig / EnvironmentFile / EnvironmentVM / EnvironmentDownloadFile, GCS bucket
      `functions-src` needing forceDestroy).
- [ ] 5.2 Clean the 5-day-old Error'd workflow pods in `crossplane-system`
      (`crossplane-proxmox-iso-pipeline`, `crossplane-proxmox-workstation-cloud-init-render`).
      (`provider-gcp-iam` is Running again since pc01 returned — no scheduling work needed.)
- Verify: `probe_duration_seconds{job="probe/monitoring/control-plane-probe"} < 2` sustained
  (was 4.4s); `ControlPlaneAPIUnreachable` stops firing daily.

### 6. obs pinned pods Pending 26d — missing control-plane toleration

Root cause: `k8s/charts/obs/restreamer.yaml` + `cloudflared.yaml` intentionally pin to
macintel01 (only node on the OBS LAN — legitimate hardware dependency) and tolerate
`unschedulable`, but macintel01 now also carries the `node-role.kubernetes.io/control-plane`
taint, which they don't tolerate → perma-Pending.

- [ ] 6.1 Add `- key: node-role.kubernetes.io/control-plane` toleration to both files (keep the
      targeted-toleration comment style), commit + push, let ArgoCD sync.
- Verify: restreamer + cloudflared-obs Running on macintel01; `KubePodNotReady(obs)`×2 clear;
  obs.syscd.live serves through the tunnel.

### 7. Alert hygiene — every alert actionable, no warnings tier

- [ ] 7.1 VMAlertmanager inhibit rule: `KubeNodeUnreachable` inhibited by `KubeNodeNotReady`
      (same node fires both).
- [ ] 7.2 Scope `NicOffloadFixNotRunning`
      (`k8s/charts/support-cluster/templates/monitoring/nic-offload-fix.yaml`) to Ready nodes,
      e.g. `... > 0 unless on(node) (kube_node_status_condition{condition="Ready",status="true"} == 0)`
      — today it fires purely because a node is down.
- [ ] 7.3 Actionability audit, no new severity tier: every alert must be something a human can
      and should act on, or it gets fixed at the rule or deleted. Concretely:
      `Zigbee2MQTTDown` (`for: 5m`) flapped daily all window with no action taken — raise the
      `for` so only sustained outages page, or delete it; apply the same test to every
      daily-firing alert (permanent red = invisible red).
- Verify: rule diff renders; next pc01-style outage pages once per cause, not five times.

## No action needed (verified self-resolved)

- pc01 recovery itself: already Ready; only prevention remains (step 2).
- `GCPBudgetKillSwitch` / `GCPBudgetThresholdExceeded` (08-30→09-01): budget ratio now 0.
- `CiliumNodeConnectivityDown` / `DNSResolutionFailing`: 1-day blips on 08-30, not recurring.

## Verification (whole plan)

1. Alertmanager API: `curl -s localhost:9093/api/v2/alerts | jq length` → 0 (or only unrelated).
2. `kubectl get pods -A | grep -vE "Running|Completed"` → nothing crashlooping for days.
3. History check after 48h: `max_over_time(ALERTS{alertstate="firing"}[48h])` shows no repeat of
   ControlPlaneAPIUnreachable / HermesDown / KubePodCrashLooping(argocd) / CNPG crashloops.

## Out of scope

- vmsingle data window oddity (retention 14d, oldest data 08-30; vmsingle restarted 09-02).
- pc01 hardware replacement decisions.
