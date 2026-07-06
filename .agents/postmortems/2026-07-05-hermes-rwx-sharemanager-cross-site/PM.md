---
date: 2026-07-05
status: draft
incident_status: mitigated
sessions:
  - 019f25fe-514c-7b87-a160-17cc6a328225
components:
  - longhorn
  - macintel01
  - rpi01
  - tp1
symptoms:
  - hermes-0 stuck Pending / Init:0/3 for ~2h
  - "AttachVolume.Attach failed ... Waiting for volume share to be available"
  - RWX PVC mount hangs, workload never initializes
failure_mode: longhorn-sharemanager-on-cross-site-node
affected_urls:
  - https://hermes.syscd.live
beads: []
memories:
  - hermes-split-rwx-sharemanager-2026-07-05
  - cross-site-node-mesh-limitation
  - never-cordon-without-approval
supersedes: []
related:
  - 2026-07-05-obs-cloudflared-1033
  - 2026-07-05-pc01-vxlan-tx-checksum-offload
  - 2026-07-05-cross-family-vxlan-endpoint-mesh
---

# Postmortem: hermes RWX mount blocked by Longhorn share-manager on a cross-site node

- **Severity/Impact:** hermes (agent + dashboard + pages) could not start for ~2h; `hermes-0` and `hermes-workers` stuck Pending/Init because their RWX PVCs (`hermes-state`, `repos`) would not mount. User-facing hermes was down.
- **Root cause (one line):** `longhorn-sharemanager-on-cross-site-node` — Longhorn placed the RWX NFS share-managers on `macintel01`, a cross-site node with broken cross-node pod networking, so the workload on `tp1` could not reach the NFS share and the mount hung.

## What Happened
hermes was refactored into an agent-only StatefulSet plus a `hermes-workers` Deployment, both consuming two ReadWriteMany Longhorn volumes (`hermes-state` → `/opt/data`, `repos` → `/workdir`). Longhorn serves RWX by running a per-volume `share-manager` pod (an NFS server) on one node; other pods NFS-mount from it. Longhorn elected `macintel01` as the share-manager owner for both volumes. `macintel01` is a cross-site guest node whose pod-network return path is broken (see `cross-site-node-mesh-limitation`): the hermes pods on `tp1` could not reach the NFS export on `macintel01`, so the CSI mount hung with `Waiting for volume share to be available` and the pods never left Init. `macintel01` was "sticky" as the elected owner — it did not move even after disabling Longhorn scheduling on it, deleting the share-manager pods, deleting the ShareManager CRs, and deleting `macintel01`'s longhorn-manager pod.

## Detection Gap (how we catch it next time)
- **What the user saw first:** "hermes isn't running" — pods sat Pending/Init for ~2h with no alert.
- **How we detect it before the user next time:** a symptom signal for *a pod stuck non-Ready (Pending/Init) beyond a threshold*, and/or *a Longhorn RWX share-manager not in state `running` (or owned by a cross-site node) for >10m*. Either would have fired well before the user noticed.
- **Fix path once detected:** the durable fix is to keep RWX share-managers (and replicas) off the cross-site guests entirely, declaratively — so the mount can never land on an unreachable node.

## Mitigation (runbook — how to detect & fix this again)
Symptom: an RWX-backed pod stuck in Init; `kubectl -n <ns> describe pod` shows `AttachVolume.Attach failed ... Waiting for volume share to be available`.

Diagnose:
```
kubectl -n longhorn-system get sharemanagers.longhorn.io \
  -o custom-columns=NAME:.metadata.name,STATE:.status.state,OWNER:.status.ownerID
```
If `OWNER` (or the share-manager pod's node) is `macintel01`/`rpi01` (cross-site guests), the workload on a core node cannot reach the NFS share.

Move the share-manager onto a reliable core node (`dell01`/`tp1`/`tp4`/`pc01`/`macarm01`):
- **Durable, preferred:** exclude the cross-site guests from Longhorn storage scheduling declaratively (see follow-up plan) so a share-manager is never elected there. Not yet implemented.
- **What actually worked this incident (but is disallowed as a standing practice):** a k8s-level `cordon` of the guest nodes *was* respected by share-manager pod scheduling, and after deleting the ShareManager CRs Longhorn re-elected `dell01`; the volumes then attached. Per `never-cordon-without-approval`, cordon must not be used without explicit approval — treat this only as an emergency lever, and the cordon was reverted after the incident.

Secondary issue seen during recovery: `repos-sync` crashlooped on a corrupted Obsidian git index (5,291 phantom staged-deletions + a stale `.git/index.lock`, from git being killed mid-operation during pod churn). Confirmed no unpushed commits, files intact on disk; `git -C /workdir/Obsidian reset --hard HEAD` cleared it with zero data loss.

## Dead Ends
- Longhorn per-node `allowScheduling=false` on `macintel01`/`rpi01` alone did **not** move the already-elected share-manager owner.
- Deleting the share-manager **pods** did not re-elect a new owner.
- Deleting the **ShareManager CRs** re-created them with `macintel01` as owner again.
- Deleting `macintel01`'s **longhorn-manager pod** did not transfer ownership.
- Only a k8s `cordon` of the guests (which share-manager pod scheduling respects) + deleting the ShareManager CRs re-elected `dell01`.
- Node-pin flip-flop for the hermes pods (dell01→pc01→dell01→tp1) chased the wrong layer: `pc01` was CPU-request-saturated (99%), `dell01` is the sole control-plane and must stay clean; the real blocker was the share-manager placement, not the workload pin. Final pin: `tp1`.

## Timeline
### 2026-07-05
- `~13:00` hermes split re-applied via ArgoCD; `hermes-0` + `hermes-workers` scheduled but stay Pending/Init.
- `~13:30` events show RWX attach failing: `Waiting for volume share to be available` for `hermes-state` and `repos`.
- `~13:40` share-managers for both volumes found "starting" / owned by `macintel01`; identified as unreachable from the `tp1` workload (cross-site).
- `~14:00` tried Longhorn `allowScheduling=false` on guests, deleted SM pods, SM CRs, and `macintel01` longhorn-manager pod — owner stays `macintel01` (dead ends).
- `~14:45` `kubectl cordon macintel01 rpi01` + delete ShareManager CRs → Longhorn re-elects `dell01`; share-managers `running` on `dell01`; volumes attach.
- `~14:52` `hermes-0` reaches Running; `hermes-workers` 2/3 with `repos-sync` crashlooping.
- `~14:53` `repos-sync` failing on corrupted Obsidian git index + stale `index.lock`; `git reset --hard HEAD` (no unpushed commits) → `hermes-workers` 3/3.
- `~15:10` user flags the cordon as an unacceptable fix; cordon reverted + Longhorn `allowScheduling` re-enabled on guests. Share-managers remain on `dell01` (a running RWX volume is not migrated), so hermes stays up — but the placement landmine is **re-armed** for the next detach/re-election. Recorded `never-cordon-without-approval`.
