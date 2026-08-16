---
date: 2026-08-16
status: resolved
incident_status: resolved
sessions:
  - 01a006c5-f21e-7533-b79c-6611dbc3cd85
components:
  - longhorn
  - hermes
  - argocd
failure_mode: rwo-affinity-missing-plus-gh-multi-account-active-flip
symptoms:
  - hermes-dashboard pod stuck ContainerCreating, "Multi-Attach error for volume ... Volume is already used by pod(s) hermes-0, hermes-memory-backup, hermes-repository-sync"
  - hermes-repository-sync CrashLoopBackOff, "remote: Invalid username or token. Password authentication is not supported for Git operations."
  - "gh auth status shows active account = mrag23, not yurifrl, despite hermes-env GH_TOKEN belonging to yurifrl"
  - ArgoCD sync operation stuck Running for 9h+, new syncs rejected with "another operation is already in progress"
affected_urls:
  - https://hermes.syscd.live
beads: []
memories: []
supersedes: []
related:
  - 2026-07-05-hermes-rwx-sharemanager-cross-site
  - 2026-08-10-longhorn-daemonsets-excluded-from-dell01
---

# Postmortem: hermes-dashboard missing PVC affinity + gh multi-account active-account race + stuck ArgoCD sync

- **Severity/Impact:** `hermes-dashboard` was stuck `ContainerCreating` for ~33h (Longhorn Multi-Attach on the shared RWO `state-hermes-0` PVC). `hermes-repository-sync` was `CrashLoopBackOff` for ~10h+ on a GitHub authentication failure. An ArgoCD sync operation for the `hermes` Application was stuck `Running` for 9h+, silently blocking subsequent syncs. The remediation rollout temporarily blocked the agent and Pages while the bot token and stale StatefulSet pod were corrected; it closed with all Hermes workloads Running and the ArgoCD Application Synced/Healthy.
- **Root cause (one line):** `rwo-affinity-missing-plus-gh-multi-account-active-flip` — the shared RWO state/workdir design coupled independent Hermes workloads to one node and shared a mutable GitHub CLI credential cache; a dashboard without co-location affinity hit Longhorn Multi-Attach, and concurrent `gh auth login` calls could select the wrong account. A stuck ArgoCD operation then hid the intended rollout.

## What Happened

The user asked to fix `hermes-dashboard` being down. `kubectl describe pod` on the stuck dashboard pod showed `FailedAttachVolume: Multi-Attach error for volume "pvc-7f563eff..." Volume is already used by pod(s) hermes-0, hermes-memory-backup-..., hermes-repository-sync-...` — the PVC `state-hermes-0` is `ReadWriteOnce` on `longhorn-ha`, and the scheduler had placed the dashboard replica on a different node (first `macarm01`, then `dell01` after a manual pod delete) than the node holding `hermes-0` (`tp1`). Comparing `dashboard-deployment.yaml` against the other three templates that mount the same PVC (`repository-sync-deployment.yaml`, `memory-backup-deployment.yaml`, `pages-deployment.yaml`) showed all three already carry `{{- include "hermes-agent.workdirColocationAffinity" . }}` — a required pod-affinity pinning them to the node running the pod matching `hermes-agent.selectorLabels` (i.e. `hermes-0`). The dashboard template was missing it; this was a chart gap, not new drift.

Adding the affinity and pushing the commit did not immediately fix the pod: `kubectl -n argocd get application hermes` showed `Sync Status: OutOfSync` with an `operationState.phase: Running` that had `startedAt: 2026-08-15T22:11:58Z` — 9 hours earlier than the current time, but the app kept reporting a fresh-looking `Tasks`/`Update successful` log line every reconcile loop, and every manual merge-patch to `operation` was silently ignored or immediately overwritten by `selfHeal`. `argocd app sync hermes` (run via `kubectl exec` into the `argocd-server` pod) returned `FailedPrecondition: another operation is already in progress` — confirming a genuinely stuck operation, not a caching artifact. `argocd app terminate-op hermes` cancelled it; the next `selfHeal`-triggered sync then completed normally and the Deployment's `generation` advanced from 1 to 2 with the affinity applied.

With the affinity live, `hermes-dashboard` scheduled onto `tp1` (the same node as `hermes-0`) and came up `1/1 Running`. Separately, the user rotated the GitHub PAT stored in the `hermes-env` 1Password item and asked for the repository-sync container to be re-checked. Force-refreshing the `ExternalSecret` and restarting the crashlooping pod did not fix it — the pull still failed with `Invalid username or token`. Pulling the raw `GH_TOKEN` value from a live pod and querying `https://api.github.com/user` directly confirmed the token authenticates as `yurifrl` and is valid. `gh auth status` inside the same `repos-sync` container, however, showed **`mrag23`** as the active account, with `yurifrl` present but inactive. `hctl auth login` (the `git-login` init container, run independently in `hermes-0`, `hermes-dashboard`, `hermes-repository-sync`, and `hermes-memory-backup`) calls `gh auth login --with-token` on every pod start and persists the result to `$HOME/.config/gh/hosts.yml` on the shared `state-hermes-0` PVC — so every pod's init overwrites the "active account" for every *other* pod sharing that `$HOME`. A prior `mrag23` login (from before the current single-token setup — see `home-systems-6ge`, opened 2026-06-24) was still present in `hosts.yml` from an earlier pod generation and became active again on a subsequent init run, most likely `hermes-dashboard`'s restart during the affinity fix. `Obsidian` belongs to `yurifrl`, so the sync fails regardless of how many times the token itself is rotated.

## Detection Gap (how we catch it next time)

- **What the user saw first:** they asked to fix hermes being "down" — no alert had fired for either the dashboard Multi-Attach (~33h) or the repository-sync CrashLoopBackOff (~10h+ and counting).
- **Why nothing paged:** `KubePodNotReady` (20m threshold, would have caught the dashboard's `ContainerCreating`) and `KubePodCrashLooping` (5m threshold, would have caught repository-sync's restarts) both depend on `kube_state_metrics`/`kube_pod_*` series in `vmsingle-vmks`. Investigating the monitoring path for this incident surfaced that `vmsingle-vmks` is **currently in read-only mode again** (99% full, 94.6M free on its 10Gi PVC) — a recurrence of `2026-07-11-vmsingle-storage-readonly`, documented separately in `2026-08-16-vmsingle-storage-readonly-recurrence`. Both `KubePodNotReady` and `KubePodCrashLooping` evaluate `inactive`/`ok` in vmalert (never triggered) because there are **zero** `kube_*` series to evaluate against — not because the pods weren't unhealthy long enough. This incident's alerts were dark for the same underlying reason as the sibling recurrence.
- **Fix path once detected:** for the affinity gap, the durable fix already landed (see Mitigation). For the gh multi-account race, the durable fix is to stop persisting `gh auth` state to the shared PVC per-pod-init, or to pin a single account non-interactively every time regardless of what's on disk (see Mitigation) — the token rotation alone cannot fix an active-account selection problem.
- **For the stuck ArgoCD sync:** no alert exists for an Application whose `operationState.phase` has been `Running` beyond a threshold; this is the concrete new gap from this incident.

## Mitigation (runbook — how to detect & fix this again)

**Dashboard/sidecar stuck on RWO PVC Multi-Attach:**
```
kubectl -n hermes describe pod <pod> | grep -A3 FailedAttachVolume
```
`Multi-Attach error for volume ... already used by pod(s) <other-pods>` on a `state-hermes-0`-mounting pod means it lacks the co-location affinity. Confirm the chart template for that workload includes:
```yaml
affinity:
  {{- include "hermes-agent.workdirColocationAffinity" . | nindent 8 }}
```
(see `_helpers.tpl:18-25`) — every template mounting the shared `state`/`workdir` PVCs must have this. Fixed for `dashboard-deployment.yaml` in commit `226e116d`.

**ArgoCD Application stuck with a non-terminating sync operation:**
```
kubectl -n argocd get application <app> -o jsonpath='{.status.operationState.phase} {.status.operationState.startedAt}{"\n"}'
```
A `phase: Running` with a `startedAt` many hours in the past, combined with `argocd app sync <app>` returning `FailedPrecondition: another operation is already in progress`, confirms a stuck operation. Merge-patching `operation: null` or a fresh `operation.sync` block on the Application resource does **not** reliably clear it (selfHeal reissues the same stuck operation). The only command that worked:
```
kubectl -n argocd exec deploy/argocd-server -- argocd app terminate-op <app> --grpc-web --plaintext --server localhost:8080
```
followed by (if `selfHeal` doesn't retrigger automatically):
```
kubectl -n argocd exec deploy/argocd-server -- argocd app sync <app> --grpc-web --plaintext --server localhost:8080
```
Verify the fix landed: `kubectl get deploy -n <ns> <name> -o jsonpath='{.metadata.generation}'` should have incremented, and the new pod/spec should reflect the pushed change.

**gh multi-account active-account flip (git auth fails despite a valid, correctly-scoped token):**
```
kubectl exec -n hermes <any-hermes-pod> -c <main-container> -- gh auth status
```
If the **active** account differs from the account that owns the target private repo, the `hosts.yml` on the shared `state-hermes-0` PVC has a stale entry from a prior `hctl auth login` run (possibly by a different sidecar pod sharing `$HOME`) that is winning the active-account race. Rotating the token in 1Password does **not** fix this by itself — `gh auth login --with-token` adds/updates an account entry but does not force it active if another account is already marked active in `hosts.yml`. Not yet fixed durably; see Follow-ups.

## Dead Ends

- Assumed the token rotation the user performed had not propagated — checked and force-refreshed the `ExternalSecret`, confirmed the `hermes-env` Secret's `resourceVersion` changed, restarted the pod. The pod still failed identically; the token was never the problem on the *current* attempt (it correctly resolves to `yurifrl` via a direct `api.github.com/user` call) — the earlier `curl` test that seemed to show `mrag23` owning the token was itself corrupted by shell quoting/stderr leaking into the captured token string in a mixed bash/fish terminal session; re-run cleanly it confirmed `yurifrl`.
- Repeated `kubectl -n argocd patch application hermes --type merge -p '{"operation":{"sync":{...}}}'` calls appeared to have no effect; several were misread as "the manifest change isn't being picked up by ArgoCD" when the real blocker was the stuck prior operation silently absorbing every new patch. `kubectl -n argocd annotate application hermes argocd.argoproj.io/refresh=hard` did correctly force a fresh git diff (confirmed the Deployment's `OutOfSync` status appeared), but a hard refresh alone does not cancel an in-flight stuck sync operation.
- Deleting the `operation` field via `kubectl patch --type json -p '[{"op":"remove","path":"/operation"}]'` succeeded (returned `patched`) but `status.operationState.phase` still read `Running` with the same 9-hour-old `startedAt` on the next read — the deletion did not propagate into a fresh operation; only the CLI's `terminate-op` (which calls a dedicated gRPC endpoint, not a generic patch) actually worked.

## Timeline

### 2026-08-16 (UTC)
- `~04:xx` User reports hermes-dashboard down. `describe pod` shows `Multi-Attach error` on `state-hermes-0`; pod stuck `ContainerCreating` since `2026-08-14T21:46:16Z` (~33h) on `macarm01`.
- `~04:0x` Deleted the stuck pod (guardrail-approved, reversible — Deployment-owned); rescheduled onto `dell01` — still a different node than `hermes-0` (`tp1`), same Multi-Attach error recurs.
- `~04:1x` Compared `dashboard-deployment.yaml` against `repository-sync-deployment.yaml`/`memory-backup-deployment.yaml`/`pages-deployment.yaml`: all three already have `workdirColocationAffinity`; dashboard is missing it. Added it, `helm template` confirmed correct render, committed + pushed (`226e116d`).
- `~04:1x–06:5x` ArgoCD sync repeatedly reported success (`Tasks`/`serverside-applied` log lines) but the live Deployment's `generation` stayed `1` and had no `affinity` block. Discovered `status.operationState.startedAt: 2026-08-15T22:11:58Z` — a stuck operation ~9h old, silently blocking real syncs. `argocd app sync hermes` via `kubectl exec` into `argocd-server` confirmed `FailedPrecondition: another operation is already in progress`.
- `07:33` `argocd app terminate-op hermes` cancelled the stuck operation.
- `07:36` A fresh selfHeal-triggered sync started; completed by `~07:41` — Deployment `generation: 2`, affinity present, `hermes-dashboard` pod rescheduled onto `tp1` (same node as `hermes-0`), `1/1 Running`.
- User asked whether the affinity was the only way to fix scheduling flexibility, and separately reported having rotated the `hermes-env` GitHub token and asked to re-check `hermes-repository-sync`.
- `~07:5x` Force-annotated the `ExternalSecret` to resync early (`hermes-env` resourceVersion changed, confirming the fresh 1Password value landed), deleted the crashlooping `repos-sync` pod. New pod still failed identically: `remote: Invalid username or token`.
- `~08:0x` Pulled `$GH_TOKEN` directly from a live `hermes-0` container and queried `https://api.github.com/user` — confirmed the current token is valid and belongs to `yurifrl`. `gh auth status` inside the `repos-sync` container showed active account `mrag23`, inactive `yurifrl` — the real fault is account selection, not the token value.
- `~08:1x` Investigating alert dark spots for this incident led to discovering `vmsingle-vmks` is in read-only mode again (documented separately as a recurrence, `2026-08-16-vmsingle-storage-readonly-recurrence`).
- Incident left in `resolved` state. The dashboard and agent run from the current chart revision; Pages has an isolated `emptyDir` checkout; the ArgoCD Application is `Synced`/`Healthy`; and the new `mrag23` bot token is used for both `GH_TOKEN` and `GITHUB_TOKEN`. The unresolved infrastructure concern is External Secrets: its cert-controller continues to crashloop on `rpi01`, even though the Hermes secret refreshed successfully during this recovery.

## Close-out: RWX state and disposable workdirs

The durable follow-up was started with `hermes-state-rwx`: a 5Gi Longhorn `ReadWriteMany` target PVC using the existing two-replica `longhorn-ha` class. It is intentionally staged rather than switched live: current Hermes state was measured at 736Mi, and a quiesced copy must precede changing `sharedStorage.state.claimName`.

`/workdir` no longer depends on `workdir-hermes-0`. The agent uses the chart's `emptyDir` workdir and non-blocking `repos-sync` sidecar; Pages receives its own `emptyDir`, isolated `/auth` credential directory, `git-login`, and `hermes-pages` clone init container. This removes the RWO Multi-Attach and shared `gh` active-account race from the workdir path. The old repository-sync Deployment is removed.

During the rollout, ArgoCD applied the current StatefulSet template but left `hermes-0` on an old controller revision containing the removed `repos-clone` init container. A normal user-approved deletion remained stuck past its 120-second grace period; the user then explicitly approved force deletion. The StatefulSet immediately recreated `hermes-0` at the current revision, without that init container. The bot's classic PAT initially lacked `read:org` and access to `hermes-pages`; after the user corrected the account access and secret fields, all workloads reached Running and ArgoCD reported `Synced` and `Healthy`.
