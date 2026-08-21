---
created: 2026-08-16
project: home-systems
description: Hermes RWX target provisioned; ESO recovery restored Hermes health.
session_id: 01a00999-2057-76c6-8618-9ddf5862d41c
resume_with: cly agent-session resume --provider pi hermes-rwx-emptydir-recovery
checkpoint_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/checkpoints/hermes-rwx-emptydir-recovery.md
---

## Context
- Goal: make Hermes resilient with durable RWX state and a disposable `emptyDir` workdir rebuilt from Git.
- Existing state PVC was `state-hermes-0` (RWO, 20Gi, Longhorn HA); state use measured at 735Mi. Existing workdir was 15Gi RWO and used 3.9Gi, mostly Obsidian.
- `worktrees/` is user-owned/untracked and must remain untouched.

## Decisions
- Durable target state is `hermes-state-rwx`: 5Gi, ReadWriteMany, `longhorn-ha`; cluster currently has only two eligible Longhorn disks (tp1/tp4), so it is two-replica rather than three-replica.
- Workdir is intended as pod-local `emptyDir`: Git is authoritative and checkout failure must not block the Hermes gateway.
- Public commit `b9067352` changes the chart so repository clone/retry runs in the existing `repos-sync` sidecar instead of blocking agent startup as an init container.
- Do not enable rpi01 storage scheduling; it is offsite and had prior Longhorn failures. pc01 was NotReady during investigation.

## Current State
- Public commits pushed: `4bea5c7d feat(hermes): add RWX state migration target`; `b9067352 fix(hermes): do not block agent on repo sync`.
- Private values commit pushed: `1e51e14 config(hermes): create RWX state target`.
- `hermes-state-rwx` is Bound: 5Gi, RWX, `longhorn-ha`.
- Old state/workdir claims were not deleted and no state data was copied or cut over. The RWX migration remains incomplete.
- ESO `hermes-env` was deleted at user request after provider extraction failed on a duplicate `OPENROUTER_API_KEY`; it was recreated after the duplicate was fixed. ESO controller relocated from rpi01 to macarm01 and reports `SecretSynced`.
- Final verified status: ArgoCD `hermes` Synced/Healthy; `hermes-0` 2/2 Running; dashboard, Pages, and memory backup Running.

## Lessons
- Longhorn replicas provide durability; RWX allows consumers on any node to mount one logical volume. Replica count does not equal the number of mounting nodes.
- `dataFrom.extract` in ESO requires unique 1Password field titles. Duplicate titles prevent Secret recreation.
- ESO lost leader election on rpi01 because API calls to `10.96.0.1:443` timed out; relocation to macarm01 restored reconciliation.
- A Git checkout as an init container can turn an optional private-repo access failure into a full gateway outage. Keep clone/sync retry in a non-blocking sidecar.
- ArgoCD Hermes operations can remain stuck waiting on completed hooks; inspect `operationState` and use `argocd app terminate-op hermes` only for a stale operation.

## Next Steps
1. Verify the live StatefulSet has reconciled `b9067352`: `repos-clone` must not be an init container and clone/sync must retry in the sidecar.
2. Design and execute a separately approved write-freeze/copy/cutover from `state-hermes-0` to `hermes-state-rwx`; do not copy live mutable state without quiescing writers.
3. Complete all workdir consumers’ independent `emptyDir` checkout flows; Pages must clone its own repo before serving.
4. After successful cutover and health verification, retire old RWO claims deliberately through GitOps.
