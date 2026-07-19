---
date: 2026-07-18
status: closed
incident_status: mitigated
sessions:
  - 019f25f1-17e8-7c10-b2da-7c14fbf636e4
components:
  - csi-provisioner
  - longhorn
  - cnpg
  - kube-state-metrics
  - supabase
  - macintel01
  - rpi01
symptoms:
  - supabase down (realtime/storage CrashLoopBackOff 1600+ restarts, studio Pending 15d)
  - CNPG pg cluster degraded (1/3 instances Ready, only primary pg-3)
  - all PVC provisioning dead cluster-wide (PVCs stuck Pending)
  - no alert fired, no self-heal — user discovered it manually
failure_mode: csi-provisioner-pinned-to-cordoned-node
affected_urls:
  - https://app.syscd.live
beads: [home-systems-5sw]
memories: [csi-provisioner-pinned-cordoned-node-2026-07-18]
supersedes: []
related:
  - 2026-07-05-hermes-rwx-sharemanager-cross-site
---

# Postmortem: CNPG/supabase down, cluster-wide volume provisioning dead, zero detection

- **Severity/Impact:** Supabase fully down for days (all API services crashlooping or Pending); the CNPG `pg` cluster ran degraded at 1/3 instances; and — worst — **every** Longhorn PVC in the cluster could not be provisioned for 5+ days. None of it alerted; the user found it by asking.
- **Root cause (one line):** `csi-provisioner-pinned-to-cordoned-node` — Longhorn's `csi-provisioner` Deployment carried a stray `nodeSelector: kubernetes.io/hostname=dell01`, and dell01 is the cordoned control plane, so all three provisioner replicas sat Pending and no volume could ever be created.

## What Happened
Three entangled faults, discovered top-down from "supabase is down":
1. **Supabase app config bugs** (independent of the DB): `realtime` set `search_path` to a `_realtime` schema that never existed (`invalid_schema_name`); `storage` rejected CNPG's self-signed cert (`DB_SSL=require` → `SELF_SIGNED_CERT_IN_CHAIN`) and then lacked `CREATE` on the `supabase` database for its migration; `studio` mounted a `functions` PVC that was never created and referenced a missing `openAiApiKey` secret key.
2. **CNPG `pg` degraded to 1/3.** Replicas `pg-1`/`pg-4` had diverged onto old WAL timelines (`requested timeline N is not a child of this server's history`, `pg_rewind: could not locate required checkpoint record`) and could not rejoin the primary.
3. **The reason the replicas couldn't rebuild — the real root cause:** rebuilding a replica needs a fresh PVC, but **`csi-provisioner` was pinned to cordoned dell01** so no PVC anywhere could provision. This had silently broken all dynamic provisioning cluster-wide for 5+ days.

The primary `pg-3` was healthy throughout, so "postgres" was **degraded, not down** — but supabase (its consumer) was down, which is what the user experienced.

## Detection Gap (how we catch it next time)
- **What the user saw first:** nothing from monitoring — they asked "is supabase using shared postgres / is it too heavy", and only then did the outage surface. A 5-day cluster-wide provisioning outage and a degraded stateful DB produced **zero** pages.
- **How we detect it before the user next time:**
  1. **CNPG cluster-health alert (KSM-independent, primary new signal).** The `cnpg-pg` VMPodScrape already collects CNPG metrics, but **no VMRule consumes them**. Ready instances `< 3` (or `cnpg_collector_up` count below desired) for >10m is a clean symptom alert for "postgres degraded" and does not depend on kube-state-metrics.
  2. **Re-arm the existing safety net: fix kube-state-metrics.** The `KubePodNotReady` VMRule (created by the hermes postmortem) *would* have caught the 5-day-Pending `csi-provisioner` and the 15-day-Pending `studio` — but KSM is unhealthy (pods crashlooping, 176/1174 restarts; one Running at 58 restarts) so `kube_*` metrics are gappy and the alert is effectively inert. This is the still-open hermes bead `home-systems-i8t.1`.
  3. **Container-not-ready alert (KSM-dependent).** `KubePodNotReady` only fires on `phase=Pending|Unknown`; a Running-but-`0/1` CrashLoopBackOff pod (realtime looped 1600+ times) slips through. Alert on `kube_pod_container_status_ready==0` for >20m to cover crashloops too — gated on KSM being healthy first.
- **Fix path once detected:** this file's Mitigation section is the runbook.

## Mitigation (runbook — how to detect & fix this again)
**Symptom:** PVCs stuck `Pending`, `kubectl -n <ns> describe pvc` shows `waiting for a volume to be created ... 'driver.longhorn.io'`.
1. **Check the provisioner first:** `kubectl -n longhorn-system get pods | grep csi-provisioner`. If all replicas are `Pending`, describe one: a `nodeSelector`/affinity mismatch against a cordoned node is the cause. Longhorn's `system-managed-components-node-selector` setting is empty here, so any `nodeSelector` on `csi-provisioner` is a stray manual pin — remove it:
   `kubectl -n longhorn-system patch deploy csi-provisioner --type=json -p '[{"op":"remove","path":"/spec/template/spec/nodeSelector/kubernetes.io~1hostname"}]'`
2. **CNPG replica diverged / crashloop** (`requested timeline ... not a child`, `pg_rewind: could not locate required checkpoint record`): the replica's local data is unrecoverable. With the **primary healthy**, delete the replica's PVC + pod and let CNPG re-clone via `pg_basebackup`:
   `kubectl -n databases delete pod/pg-N pvc/pg-N --wait=false` (the operator recreates the instance). Verify `kubectl -n databases get cluster pg` reaches `3/3 healthy`.
3. **Keep Longhorn replicas off cross-site nodes** (rpi01/macintel01 can't be reliable Longhorn members): `kubectl -n longhorn-system patch nodes.longhorn.io <node> --type=merge -p '{"spec":{"allowScheduling":false}}'`. If a pod needing a Longhorn RWO volume lands on macintel01, `kubectl cordon macintel01`.
4. **Supabase app fixes** (now in `k8s/applications/supabase.yaml` + `postgres.yaml`):
   - realtime: `CREATE SCHEMA IF NOT EXISTS _realtime AUTHORIZATION supabase_admin;`
   - storage TLS: `environment.storage.DB_SSL: no-verify`
   - storage perms: `GRANT CREATE ON DATABASE supabase TO supabase_storage_admin;`
   - studio: `persistence.functions.enabled: false` + `openAiApiKey: ""` in the dashboard ExternalSecret template
5. **Propagation gotcha:** these are app-of-apps managed; a git push must sync `applications` (parent) THEN `supabase`. Force with `kubectl -n argocd patch application <app> --type merge -p '{"operation":{"initiatedBy":{"username":"admin"},"sync":{"syncStrategy":{"hook":{}}}}}'`.

## Dead Ends
- **"It's etcd/control-plane instability"** (per the `supabase-cnpg-deploy-status` memory) — plausible prior theory, but false this time: the real cause was a stray `nodeSelector` on `csi-provisioner`. The simplest explanation (a pinned pod on a cordoned node) beat the scary one.
- **"pg replicas being down is why supabase is down"** — false. Supabase connects to `pg-rw` = the healthy primary. The replica rebuild was necessary HA hygiene but was NOT blocking supabase; the app crashes were independent config bugs (schema, TLS, secret key, PVC).
- **First replica rebuild landed on rpi01 and the volume wouldn't format/attach** — looked like a new fault; was the known cross-site Longhorn limitation. Disabling Longhorn scheduling on rpi01/macintel01 and re-cloning onto LAN nodes fixed it.
- **Over-trimmed kong to 512Mi** during right-sizing → immediate OOMKilled. Real-world headroom: openresty needs ~1Gi at startup even though steady state is ~370Mi.

## Timeline
### 2026-07-18
- `~19:40` User asks about supabase footprint / shared-postgres use; investigation begins.
- `~19:45` Found supabase realtime/storage in CrashLoopBackOff (1600+/800+ restarts), studio Pending 15d; CNPG `pg` at 1/3 Ready.
- `~19:46` `pg-4` diagnosed: `FATAL requested timeline 6 is not a child`; `pg-1`: `pg_rewind could not locate required checkpoint record`.
- `~19:48` Deleted `pg-4`/`pg-1` PVC+pod to force re-clone; PVCs stuck `Pending` — Longhorn not provisioning.
- `~19:52` Root cause found: all 3 `csi-provisioner` replicas `Pending` for 5d; `nodeSelector=dell01` (cordoned control plane). Longhorn node-selector setting empty → stray manual pin.
- `~19:53` Removed the nodeSelector → provisioners Running; PVC binds again.
- `~20:00` First replica volume landed on rpi01, format/attach failed (cross-site). Disabled Longhorn scheduling on rpi01+macintel01; cordoned macintel01; re-cloned.
- `~20:04` CNPG `pg` reaches 3/3 "Cluster in healthy state" (replicas on tp1/tp2-good nodes).
- `~20:06` realtime fixed via `CREATE SCHEMA _realtime`; both pods Running.
- `~20:20` Committed supabase.yaml (storage DB_SSL no-verify, studio functions off + openAiApiKey, kong/realtime/rest → 1 replica); drove app-of-apps → supabase sync.
- `~20:24` kong OOMKilled at 512Mi; bumped limit to 1Gi (committed).
- `~20:36` storage past TLS, now `permission denied for database supabase`; granted `CREATE ON DATABASE supabase` to `supabase_storage_admin`.
- `~20:40` All 7 supabase pods 1/1 Running; storage "Server listening ... Started Successfully". Persisted `_realtime` schema + storage grant into postgres.yaml initdb.
