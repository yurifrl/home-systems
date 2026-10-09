---
created: 2026-10-09
project: home-systems
description: GitHub mirror backup CronWorkflow deployed and verified until Longhorn storage instability and network loss blocked final clean run.
session_id: 01a11d27-ae11-73ed-9ba7-ec08aa86d1ec
resume_with: omp --resume 01a11d27-ae11-73ed-9ba7-ec08aa86d1ec
snapshot_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/snapshots/github-backup-deploy-handoff.md
---

## Context
GitHub mirror backup (Argo Workflow + CronWorkflow) deployed to home Talos cluster, ns argo-workflows. Repo public=yurifrl/home-systems (script), private=.submodules/home-systems-values (manifest).

## Decisions
- Single-file stdlib python script automations/github_backup.py; mirror clone + tar snapshots + manifest.json; ThreadPool parallel; GHA_PAT via one-shot credential helper env (not URLs)
- Manifest = 4-doc yaml in home-systems-values/applications/github-backup.yaml: ExternalSecret(item github-backup/GHA_PAT) + PVC 20Gi longhorn-ha + WorkflowTemplate + CronWorkflow 0 */6 * * * Forbid
- No git artifact in template: home-systems .gitmodules uses SSH URLs, argo artifact loader can't auth in-pod -> script fetched via urllib raw.githubusercontent
- GHA_BACKUP_JOBS=2 commit PENDING (was 4): Longhorn on ARM SBC nodes flipped XFS read-only under 4-job parallel write load twice

## Current State
- ALL pushed: public b503cca5, private 80641a8. 1Password item github-backup created (vault kubernetes, GHA_PAT seeded from env)
- In-cluster verified: SecretSynced, PVC Bound, template+cron live, PAT valid (listing NSXBet+yurifrl with privates, clones+snapshots work)
- Runs: r1 27/304 ok (Longhorn replica fault tp4->volume faulted), r2 0 ok (volume reattached READ-ONLY), r3 97/304 ok (RO again). PVC/PV 7795e75c stuck faulted+detached -> needs delete (ArgoCD recreates) before next run
- STUCK UNCOMMITTED: home-systems-values GHA_BACKUP_JOBS 4->2 staged; commit signing=1Password op-ssh-sign needs biometric (user away); 'failed to fill whole buffer' x2
- Mac left home LAN (now 192.168.0.x); cluster 192.168.68.x + ts context peer 100.65.212.5 both unreachable -> cluster ops blocked
- Collateral: zigbee2mqtt volume degraded, f21e6ce6 faulted — same storage blips; rpi01+tp4 are the only Longhorn disks with load

## Lessons
- Argo git artifact fails on repos with SSH submodules (exit 64 SSH_AUTH_SOCK) — fetch single files via urllib instead
- op item create: fields are assignment args 'FIELD[text]=value', no --field flag
- 1Password commit signing blocks automated flows when user away — commit early in session, never at end
- Longhorn RO flips under write load self-heal NOTHING: pod must unmount; delete PVC to force clean volume

## Next Steps
- User: approve 1Password signing -> retry commit of JOBS=2 in home-systems-values, push, refresh private-apps
- Delete PVC github-backup-state (kubectl), wait ArgoCD recreate + Bound + 2 replicas
- Create test Workflow (workflowTemplateRef github-backup), watch to Succeeded; expect 304/304 ok then exit 0
- Investigate rpi01 disk (SD/USB?) + tp4: consider removing rpi01 from Longhorn scheduling; check zigbee2mqtt recovery
- Cron fires 12:00 UTC regardless (JOBS=4 until commit lands) — will RO-fail against stuck volume, exit 1, harmless
