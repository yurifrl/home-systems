---
name: billing-killswitch
description: Deploy, debug, and recover the GCP billing kill-switch / Discord-alert Cloud Functions in home-systems, and unjam the crossplane-gcp ArgoCD app when its sync is blocked by Crossplane provider decay, conversion-webhook failures, or two-LAN node partitions. Use when a billing Discord alert looks stale/wrong, a function code change won't deploy, or crossplane-gcp is stuck OutOfSync/Unknown/Degraded.
---

# billing-killswitch — deploy & recover the GCP spend guard

Two Gen2 Cloud Functions live in `functions/`, triggered by the same budget
Pub/Sub topic (`billing-budget-alerts`):

- **`billing-discord`** — posts every threshold crossing to Discord (informational).
- **`billing-killswitch`** — at `KILL_RATIO` (default 1.0 = 100%) **DETACHES billing
  from the whole project** (`PUT .../billingInfo {billingAccountName:''}`). This is
  GCP's only hard stop; it is destructive and project-wide.

Both are deployed by Crossplane from `k8s/charts/crossplane-gcp/templates/billing.yaml`
(app: ArgoCD `crossplane-gcp`, project `syscd-443112`, region `us-east1`).

## How the deploy pipeline works (don't hack around it)

Code change → live function is a **content-addressed GitOps** flow. There is no
manual zip upload, no dry-run mode.

1. Push to `functions/**` → `.github/workflows/build-billing-function.yaml` runs.
2. CI auths to GCP via **WIF** (principalSet needs `roles/iam.workloadIdentityUser`
   on `billing-fn-deployer@…`; declared as `ServiceAccountIAMMember billing-fn-deployer-wif`
   in the private values). Missing binding → `iam.serviceAccounts.getAccessToken` 403.
3. CI stamps the git SHA into each function dir as a `VERSION` file, zips, uploads
   `gs://<SRC_BUCKET>/<fn>/<git-sha>.zip` (+ `latest.zip`), and `sed`s
   `sourceObject:` in `k8s/charts/crossplane-gcp/values.yaml` to the new object.
4. `main` is protected (PR required, 0 approvals), so CI lands the bump via a
   **self-merged PR** (`gh pr create` + `gh pr merge --squash`). Actions must be
   allowed to create PRs: `gh api -X PUT repos/yurifrl/home-systems/actions/permissions/workflow -f default_workflow_permissions=write -F can_approve_pull_request_reviews=true`.
5. ArgoCD syncs the new `sourceObject` → Crossplane rebuilds the Function from the
   new object → live code updates.

**Version in alerts:** each function reads its `VERSION` file at runtime and prints
`` `v<sha>` `` in every Discord message. Use it to tell which build fired — a stale
alert (e.g. old dry-run text) shows an old/absent version.

## Reading a Discord alert

- `GCP spend: … = N% (crossed …%) `v<sha>`` → `billing-discord`, informational.
- `🚨 KILL SWITCH TRIGGERING … v<sha>` → the hard stop is firing now.
- `☠️ KILL SWITCH COMPLETE` → billing detached. Re-link to recover:
  `gcloud billing projects link syscd-443112 --billing-account=000600-2E9369-0C8480`
- Any `(DRY RUN)` text = **old code** (dry-run was removed). Check the live function:
  `gcloud functions describe billing-killswitch --region=us-east1 --gen2 --project=syscd-443112 --format='value(buildConfig.source.storageSource.object,updateTime,serviceConfig.environmentVariables)'`
  Env should be `KILL_RATIO=1;LOG_EXECUTION_ID=true;PROJECT_ID=…` with **no** `KILL_DRY_RUN`.

**Re-trigger risk:** the switch is armed and evaluates ALL budgets on the topic
(incl. the manual console budget "My Budget", BRL 500). If spend is already >100%,
the next budget notification detaches billing. To prevent an immediate trip, raise
that budget above current spend (it's a manual console budget, NOT repo-managed).

## "My code change won't go live" — walk the chain

```bash
# 1. Did CI build + pin the new sha on main?
git show origin/main:k8s/charts/crossplane-gcp/values.yaml | grep sourceObject
gcloud storage ls gs://<SRC_BUCKET>/billing-killswitch/   # <sha>.zip present?
# 2. Does the Function MR match, and is it Ready?
kubectl get functions.cloudfunctions2.gcp.upbound.io billing-killswitch \
  -o jsonpath='{.spec.forProvider.buildConfig.source.storageSource.object} {range .status.conditions[?(@.type=="Ready")]}{.status}{end}{"\n"}'
# 3. Is the app actually syncing?
kubectl get application crossplane-gcp -n argocd \
  -o jsonpath='{.status.sync.status}/{.status.health.status} {.status.operationState.message}{"\n"}'
```
MR absent or app stuck → run the recovery playbook below.

## crossplane-gcp sync-jam recovery playbook

This app jams in a specific cascade. Diagnose in this order.

**A. Crossplane core missing.** Providers show `HEALTHY=True` but their runtime
Deployments/Services/CRDs are gone → core isn't reconciling.
```bash
kubectl get deploy -n crossplane-system | grep -E 'crossplane|rbac'   # core present?
```
If missing, sync the `crossplane` app (or wait for it). Core recreates provider runtimes.

**B. Provider revision decayed (stale-healthy).** A CRD or per-provider webhook
Service is gone but the revision still reports healthy, so establish is skipped.
Symptoms: `functions… CRD NotFound`, or `svc/provider-gcp-<x>` missing, or the
ArgoCD error `conversion webhook … the server could not find the requested resource`.
Force re-establish by deleting the ProviderRevision (MRs are `Orphan` → no cloud
resources touched; core recreates the revision + CRDs + Service + webhooks):
```bash
kubectl delete providerrevision provider-gcp-<x>-<hash>
```
Confirm the missing Service/CRD returns:
```bash
kubectl get svc -n crossplane-system provider-gcp-<x>
kubectl get crd functions.cloudfunctions2.gcp.upbound.io -o jsonpath='{.status.conditions[?(@.type=="Established")].status}'
```
If the **Provider** object itself is gone (not just the revision), sync the
`crossplane-providers` app — Providers are declared in
`k8s/charts/crossplane-providers/{values,templates/providers}.yaml`.

**C. CRD activation (MRAP).** In Crossplane v2, provider MRDs install **inactive**;
the `default` `ManagedResourceActivationPolicy` activates them into live CRDs
(`k8s/charts/crossplane-providers/templates/default-mrap.yaml`, globs in
`values.yaml: defaultActivations`). A new CRD you need live must have its glob there.
Verify: `kubectl get crd <crd> -o jsonpath='{range .spec.versions[*]}{.name}={.served}/{.storage} {end}'`.

**D. Two-LAN node partition (the big one).** This is a stretched cluster on
Tailscale (`100.x` InternalIPs). A worker on a different LAN than the apiserver can
get its pod network partitioned from the control plane — the pod is `Ready` but
apiserver→pod:9443 conversion-webhook calls time out / connection-refused, jamming
**every** ArgoCD app that touches those CRDs.
```bash
kubectl get nodes -o wide                          # NotReady / partitioned workers
# Prove reachability from a control-plane pod to a webhook pod:
kubectl run p-$RANDOM --rm -i --restart=Never --image=busybox:1.36 \
  --overrides='{"spec":{"nodeName":"macintel01"}}' -- nc -w4 -z <podIP> 9443
```
GCP providers are **amd64-only** (dell01/pc01/macintel01; ARM tp1/tp4/rpi01 can't
run them). If both amd64 workers are down/partitioned there is nowhere to run the
webhooks — this is a node/network recovery, not an app fix. The established pattern
is `DeploymentRuntimeConfig` pinning a fragile provider to a reachable node + direct
apiserver IP (see `templates/pinned-runtimeconfig.yaml`, `gcp-storage-pinned`, and
the `provider-gcp-iam` direct-apiserver pin). Do NOT cordon/drain nodes without asking.

**E. Stale ArgoCD cluster cache.** After re-establishing CRDs you'll see
`could not find version "v1beta2" … Version "v1beta2" is installed on the destination cluster`.
That's ArgoCD's cached API list, not a real problem. Rebuild it:
```bash
kubectl rollout restart statefulset/argocd-application-controller -n argocd
```

**F. repo-server render timeout.** `ComparisonError … failed to generate manifest …
context deadline exceeded` = the repo-server (often on slow ARM `tp1`) can't render
the 2-source Helm app within the 90s default. It warms up over retries; a durable
fix is pinning `argocd-repo-server` to an amd64 node or raising `ARGOCD_EXEC_TIMEOUT`.

## Hard rules

- GitOps only — **never `kubectl apply`** to change desired state. Commit → push → let ArgoCD sync.
- `deletionPolicy: Orphan` on these MRs: deleting a Crossplane MR/revision does NOT delete the GCP resource — safe for recovery.
- Never reintroduce a `dryRun`/`KILL_DRY_RUN` split across code/template/values. There is no dry-run.
- Node cordon/drain and any node/network remediation → ask the user first.

## Key facts

- Project `syscd-443112` (num `555680140769`), billing acct `000600-2E9369-0C8480`.
- Source bucket var: `GCP_FUNCTIONS_SRC_BUCKET` (repo var); objects `gs://…/<fn>/<sha>.zip`.
- Discord webhook: 1Password → ESO Secret `crossplane-system/billing-discord-webhook`
  key `webhook-url` → GCP Secret Manager → function env `DISCORD_WEBHOOK_URL`.
- Repo-managed budgets are per-bucket BRL 100; "My Budget" (BRL 500) is a manual
  console budget publishing to the same topic — the switch evaluates it too.
