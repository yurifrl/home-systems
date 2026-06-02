---
created: 2026-06-01T20:05:00Z
project: home-systems
description: Talos v1.10.3→v1.13.3 upgrade, Longhorn enablement + capacity fix, and full local-path→Longhorn storage migration
context: Bare-metal Talos cluster (dell01 control-plane, tp1/tp4 RK1 workers), ArgoCD GitOps, nostos provisioning tool
tags: [talos, longhorn, nostos, upgrade, storage-migration, local-path]
session_name: 2026-05-31-1306-dashboard-mock-and-ci
purpose: Get distributed storage working on Talos (upgrade for iscsi-tools), give Longhorn real capacity, and migrate every app off local-path onto Longhorn
session_id: 019e7bf4-f940-7ded-a97e-f21d6b006f25
provider: pi
resume_with: cly agent-session resume --provider pi 2026-05-31-1306-dashboard-mock-and-ci
context_name: 2026-05-31-1306-dashboard-mock-and-ci
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/2026-05-31-1306-dashboard-mock-and-ci.md
---

# Session: Talos v1.13.3 + Longhorn Capacity + Storage Migration

- **Name:** 2026-05-31-1306-dashboard-mock-and-ci (slug stale; real topic = Talos upgrade + Longhorn + storage migration)
- **Purpose:** Working distributed storage on Talos with real capacity, all apps off local-path.
- **Resume:** `cly agent-session resume --provider pi 2026-05-31-1306-dashboard-mock-and-ci`

## Context
dell01 (amd64 control-plane, 512GB NVMe OS + 256GB SATA), tp1 (arm64 RK1, 31GB eMMC + 256GB NVMe), tp4 (arm64 RK1, 31GB eMMC only). GitOps via ArgoCD; provisioning via `nostos` (.submodules/nostos, data in nostos/). Beads (`bd`) for tracking.

## Problem → Solution arc
1. Ceph Rook researched, rejected → Longhorn. Longhorn managers crashed (Talos lacked `iscsiadm`) → needed `iscsi-tools`+`util-linux-tools` extensions → full Talos upgrade.
2. Made nostos UX-first; upgraded all nodes v1.10.3→v1.11.6→v1.12.8→v1.13.3.
3. Longhorn stuck on 28GB OS partition (duplicate `defaultSettings` dropped `defaultDataPath`). Migrated tp1 to 256GB NVMe; wiped dell01's 256GB SATA + mounted + tolerations → joined.
4. Migrated EVERY app off local-path to Longhorn.

## Decisions
- Adjacent-minor upgrade stepping; no etcd snapshot; version-matched talosctl per hop.
- Longhorn data path `/var/mnt/storage`; dell01 SATA wiped (was Windows) per user OK.
- Migration approach: rsync old local-path PV → temp Longhorn PVC → into chart's final volumeClaimTemplate PVC (charts ignore `existingClaim` for StatefulSets), then ArgoCD rebinds.
- home-assistant + zigbee2mqtt: data migrated (zero loss). bind9: thrown away (user said; external-dns repopulates). foundry: recreated fresh (defunct, GPU node gone). echotube: PVC removed → emptyDir.

## Current State (DONE)
- All 3 nodes Talos v1.13.3 (kernel 6.18.33), iscsi-tools present.
- Longhorn disks: tp1 256GB (~242 free), dell01 256GB (~251 free), tp4 30GB (~11 free). ~542GB total, all Ready+Schedulable.
- **Zero local-path PVCs/PVs cluster-wide**; no local-path StorageClass; local-path-provisioner gone.
- home-assistant 2/2 Running (migrated data), bind9 1/1 Running (fresh), zigbee2mqtt CrashLoopBackOff (PRE-EXISTING coordinator/SLZB-06 socket issue at 192.168.68.111:6638 — NOT storage; data safe on Longhorn).
- All beads closed; commits pushed (HEAD ~25e35471).

## Next Steps / Watch-outs
- **zigbee2mqtt coordinator unreachable** (228 restarts) — pre-existing, separate from storage. Investigate SLZB-06 connectivity.
- **Two default StorageClasses** (`longhorn` + `longhorn-ha` both default) — set `persistence.defaultClass: false` in k8s/applications/longhorn.yaml so only longhorn-ha is default.
- tp4 is the capacity constraint (30GB eMMC, no 2nd disk) — keep big replica=2 volumes on tp1+dell01.
- Concurrent session active in this repo (harbor/registry/board-games) — coordinate on git.
- nostos test-pollution bug fixed (bootstrap_test.go HOME redirect); fqt closed.
