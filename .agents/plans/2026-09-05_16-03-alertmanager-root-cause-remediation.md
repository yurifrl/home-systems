# Alertmanager root-cause remediation

Clear the 14 firing criticals by fixing their 5 root causes (pc01 down, CNPG degraded,
image-updater arg bug, hermes backup token, chronic API slowness), then cut alert noise.
Data window: 2026-08-30 → 09-05 (vmsingle holds no older alert history).

## Context

- Alertmanager `vmalertmanager-vmks` (monitoring): 14 active alerts, **all critical**, 0 silences.
- History from `vmsingle` TSDB `ALERTS{alertstate="firing"}`: retention 14d but earliest data 08-30.
- Rules live in `k8s/charts/support-cluster/templates/monitoring/` (e.g. `control-plane.yaml`,
  `nic-offload-fix.yaml`); all changes ship via git → ArgoCD (never `kubectl apply`).
- Relevant postmortems: `.agents/postmortems/2026-08-16-crossplane-finalizer-retry-storm/PM.md`,
  `2026-08-18-pc01-containerd-pleg-dead/PM.md`, `2026-08-10-longhorn-daemonsets-excluded-from-dell01/PM.md`.

## Steps

### 1. pc01 (GPU worker) down — upstream root of ~6 alerts

- [ ] 1.1 Check Proxmox host `192.168.68.101` reachability, then VM console for `talos-pc01`
      (PM-2026-08-18: host mgmt IP was unreachable while the VM kept running — check host first,
      simplest explanation first).
- [ ] 1.2 Restore the VM (restart / fix host networking). Watch kubelet: `kubectl get node pc01 -w`.
- [ ] 1.3 When Ready: `kubectl uncordon pc01`, confirm `nic-offload-fix` DS pods Running on pc01
      (fix from PM-2026-07-05 vxlan-tx-checksum-offload), confirm cilium/istio-cni/kube-proxy on
      pc01 go 1/1.
- Verify: `KubeNodeNotReady`/`KubeNodeUnreachable`, `NicOffloadFixNotRunning`×2, kube-system
  `KubePodNotReady`×2 clear; `provider-gcp-iam` (pinned to amd64 workers per
  `k8s/charts/crossplane-providers/templates/runtimeconfig.yaml`) becomes schedulable.

### 2. databases/pg (CNPG) — data-loss risk + `buzz` down

- [ ] 2.1 `kubectl -n databases get cluster pg -o yaml` — confirm placement pressure:
      `k8s/applications/postgres.yaml` nodeAffinity excludes macarm01/macintel01/rpi01, leaving
      tp1/tp4/pc01; pc01 down leaves 3 instances competing for 2 nodes.
- [ ] 2.2 Diagnose flapping replicas: `kubectl -n databases logs pg-3 --previous | tail -50`,
      check timeline split (pg-3 pg_controldata shows `shut down in recovery`, TimeLineID 9).
      pg-2 (0/1, 12 restarts), pg-3 (0/1, 198 restarts).
- [ ] 2.3 `kubectl -n databases exec pg-1 -- psql -Atc '\l' | grep buzz` — determine whether the
      `buzz` database exists on the primary. If missing: lost replica/timeline split is the root
      cause → restore from CNPG backup or Longhorn snapshot, or recreate the DB and let buzz
      migrate (check buzz chart for init/migration behavior first).
- [ ] 2.4 After pc01 returns (step 1), re-check pg-2/pg-3 reach Ready 3/3; if pg-3 keeps crashing
      on a stale timeline, reinit the replica via CNPG (bootstrap from backup) instead of
      restarting it.
- Verify: `kubectl -n buzz rollout restart deploy/buzz` → 1/1; CNPG cluster `3/3 Ready`;
  `KubePodCrashLooping(databases)` clears.

### 3. argocd-image-updater crashloop — 5,874 restarts, one-line fix

- [ ] 3.1 Root cause: pod args are `["--metrics-bind-address=:8443","run"]` and the container
      prints usage + exits (flag unsupported by image v1.2.1, chart 1.2.4 from
      `k8s/applications/argo-image-updater.yaml`). Find which chart value renders that arg:
      render locally with `helm template` (render-only, allowed) using the same valuesObject.
- [ ] 3.2 Fix in `k8s/applications/argo-image-updater.yaml` valuesObject (drop the flag or use
      the flag this version supports, e.g. metrics bind config for the CRD model), commit + push.
- [ ] 3.3 Wait for ArgoCD sync (`kubectl -n argocd get application argo-image-updater
      -o jsonpath="{.status.operationState.message}"` for sync errors).
- Verify: pod 1/1 Running with stable RESTARTS counter; `KubePodCrashLooping(argocd)` clears.

### 4. hermes memory-backup — token lost access to `yurifrl/hermes-memory`

Same failure as PM-2026-08-10: fine-grained PAT returns "Repository not found" (= no
Contents:R/W access; fine-grained PATs 404 instead of 403), then `hctl` crashes on the missing
`/backup/hermes-memory/.gitignore`.

- [ ] 4.1 1Password item `hermes-env`: recreate the fine-grained PAT with Contents: Read and
      write on `yurifrl/hermes-memory` (config: `.submodules/home-systems-values/hermes/values.yaml`
      → `gitBackup.user/repo`; chart: `k8s/charts/hermes/values.yaml` `memoryBackup`).
- [ ] 4.2 Force ExternalSecret resync (`kubectl -n hermes annotate externalsecret hermes
      force-sync=$(date +%s) --overwrite`), then restart the container
      (`kubectl -n hermes exec hermes-0 -c memory-backup -- kill 1` or delete/restart pod).
- Verify: memory-backup clone+push succeeds in logs; hermes-0 4/4; `HermesDown` +
  `KubePodCrashLooping(hermes)` clear.

### 5. Chronic control-plane slowness — `KubeAPIServerSlow` + `ControlPlaneAPIUnreachable` fired daily for the whole window

- [ ] 5.1 Follow the runbook in
      `.agents/postmortems/2026-08-16-crossplane-finalizer-retry-storm/PM.md`: macintel01 CPU,
      kube-apiserver handler timeouts, stuck CRD finalizers (Proxmox ProviderConfig /
      EnvironmentFile / EnvironmentVM / EnvironmentDownloadFile, GCS bucket `functions-src`
      needing forceDestroy).
- [ ] 5.2 Clean the 5-day-old Error'd workflow pods in `crossplane-system`
      (`crossplane-proxmox-iso-pipeline`, `crossplane-proxmox-workstation-cloud-init-render`) and
      the Pending `provider-gcp-iam` (needs step 1 for a schedulable amd64 worker).
- Verify: `probe_duration_seconds{job="probe/monitoring/control-plane-probe"} < 2` sustained
  (was 4.4s); `ControlPlaneAPIUnreachable` stops firing daily.

### 6. obs pinned pods Pending 26d — missing control-plane toleration

Root cause: `k8s/charts/obs/restreamer.yaml` + `cloudflared.yaml` intentionally pin to
macintel01 (only node on the OBS LAN) and tolerate `unschedulable` — but macintel01 now also
carries the `node-role.kubernetes.io/control-plane` taint, which they don't tolerate → perma-Pending.

- [ ] 6.1 Add `- key: node-role.kubernetes.io/control-plane` toleration to both files
      (keep the targeted-toleration comment style), commit + push, let ArgoCD sync.
- Verify: restreamer + cloudflared-obs Running on macintel01; `KubePodNotReady(obs)`×2 clear;
  obs.syscd.live serves through the tunnel.

### 7. Alert hygiene — stop the noise so the next page is real

- [ ] 7.1 VMAlertmanager inhibit rule: `KubeNodeUnreachable` inhibited by `KubeNodeNotReady`
      (same node fires both today).
- [ ] 7.2 Scope `NicOffloadFixNotRunning` (`k8s/charts/support-cluster/templates/monitoring/nic-offload-fix.yaml`)
      to Ready nodes, e.g. `... > 0 unless on(node) (kube_node_status_condition{condition="Ready",status="true"} == 0)`
      — today it fires purely because pc01 is down.
- [ ] 7.3 Introduce a `warning` severity tier and demote the permanently-yellow families
      (probe flaps like `Zigbee2MQTTDown`) out of critical — nothing but criticals exists today.
- Verify: with pc01 down (if still unrepaired), only the pc01 node alerts fire — not the
  daemonset/ fallout.

## No action needed (verified self-resolved)

- `GCPBudgetKillSwitch` / `GCPBudgetThresholdExceeded` (08-30→09-01): budget ratio now 0;
  synthetic-budget exclusion already committed per `.agents/reports/2026-08-05-7d-alert-triage.md`.
- `Zigbee2MQTTDown`: pod Running (restarted 26m before analysis); intermittent probe flaps only.
- `CiliumNodeConnectivityDown` / `DNSResolutionFailing`: 1-day blips on 08-30, not recurring.

## Verification (whole plan)

1. Alertmanager API: `curl -s localhost:9093/api/v2/alerts | jq length` → 0 (or only unrelated).
2. `kubectl get nodes` → all Ready; `kubectl get pods -A | grep -vE "Running|Completed"` → only
   intentional entries (e.g. nothing crashlooping for days).
3. History check after 48h: `max_over_time(ALERTS{alertstate="firing"}[48h])` shows no repeat of
   ControlPlaneAPIUnreachable / HermesDown / KubePodCrashLooping(argocd).

## Out of scope

- vmsingle data window (retention 14d, oldest data 08-30; vmsingle restarted 09-02) — worth a
  look later, not part of this remediation.
- Any pc01 hardware purchase/replacement decisions.
