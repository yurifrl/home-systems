---
created: 2026-06-06T14:37:30Z
project: home-systems
description: Refactor Hermes chart to drop Obsidian, add hctl-driven init/sidecar containers, slim Argo Application
context: Hermes deployment, Helm chart redesign, in-image CLI, Longhorn/cluster recovery diagnosis
tags: [hermes, helm, hctl, longhorn, kubernetes, argocd, refactor]
session_name: 2026-06-06-1137-flash-command-deployment
purpose: Move Hermes container glue out of YAML into a Python CLI baked into the image, replace Obsidian PVC with a generic /workdir emptyDir + git-repo sync, slim the Argo Application
session_id: 019e9578-e332-7ace-8235-26deeb545c82
provider: pi
resume_with: cly agent-session resume --provider pi 2026-06-06-1137-flash-command-deployment
context_name: 2026-06-06-1137-flash-command-deployment
context_file: /Users/yuri/Workdir/Yuri/home-systems/.agents/contexts/2026-06-06-1137-flash-command-deployment.md
---

# Session: 2026-06-06-1137-flash-command-deployment

- **Purpose:** Refactor the Hermes deployment so that container plumbing (gh auth, 1Password file sync, git-repo clone+sync) lives inside the image as `hctl` subcommands instead of inline shell blobs in `k8s/applications/hermes.yaml`. Remove all Obsidian-specific configuration.
- **Resume:** `cly agent-session resume --provider pi 2026-06-06-1137-flash-command-deployment`

## Context

Hermes runs from this repo's chart `k8s/charts/hermes/` (consumes the upstream `hermes-agent` PyPI wheel via the local image at `k8s/images/hermes/Dockerfile`). The Argo Application `k8s/applications/hermes.yaml` had grown ~300 lines of inline shell init/sidecar containers covering: `gh auth login`, `op-files-sync` (1Password Connect attachments), Obsidian vault PVC mount, and a git-sync sidecar.

The user asked to: (1) strip Obsidian from the chart, (2) make the repo-sync feature generic (`user: yurifrl, repos: [foo, bar]` → cloned into `/workdir`), (3) move all inline shell into a single CLI baked into the image, (4) use `emptyDir` for `/workdir` (no Longhorn — repos are re-clonable), (5) periodic commit+push every hour with a final on-exit backup commit, (6) collapse the Argo Application to ~30 lines.

Repo state has been dirty throughout (parallel session active in `.submodules/nostos/` shipping a new `nostos ship` command). Decided to defer the planned `nostos hermes refresh|exec|shell` host-side companion CLI to avoid clobbering that work.

## Problem

End state: Hermes pod stuck in `Init:0/4` due to a Longhorn block-device race on `state-hermes-0` PVC. Cluster itself is fine (dell01 control plane healthy per talosctl dashboard, tp1/tp4 came back Ready, only rpi01 still NotReady). Longhorn API reports the volume as `attached, healthy, robustness: healthy` with replicas on tp4+dell01, but kubelet's CSI mount fails with `mkfs.ext4: No such device or address` on `/dev/longhorn/pvc-c314b215-...`. mkfs being invoked at all is a red flag (volume has 5+ days of data).

## Decisions

- **`hctl` is Python, single file**, in `k8s/images/hermes/hctl`. Stdlib-only (image already has python3, git, gh, kubectl). Mirrors the existing `op-files-sync` pattern. No Go toolchain added.
- **Subcommands:** `hctl auth login` · `hctl op files sync` · `hctl repos clone` · `hctl repos sync`. Last one runs the periodic loop with SIGTERM/SIGINT trap → final backup pass → exit 0. Push failure inside the loop exits 1 (crash-loops the sidecar so the failure surfaces in pod status).
- **Chart values are structured, default-disabled** (`auth.gitLogin.enabled`, `opFiles.{enabled,host,vault,item,dest,connectCredentialsSecret}`, `gitRepos.{enabled,user,repos,mountPath,syncInterval}`). The chart stays portable; the lab's Argo Application turns the flags on.
- **`/workdir` is `emptyDir`** declared inline in `templates/statefulset.yaml`, mounted on both `hermes-agent` and `repos-sync`.
- **`op-files` token resolution** moved into hctl: accepts `--connect-credentials-secret NS/NAME[:KEY]` and shells out to `kubectl get secret` itself, so the chart YAML stays declarative.
- **Application file** drops to ~60 lines: `virtualService.hosts`, `nodeSelector`, `tolerations`, and three feature flags. Private `gitRepos.user/repos` come from `$values/hermes/values.yaml` in `home-systems-values`.
- **Deferred:** `nostos hermes ...` host CLI (would replace `.bin/hermes-helper`). Blocked because `internal/cli/{root,commands,schema/schema}.go` are dirty from the parallel `nostos ship` session.
- **`k8s/images/hermes/op-files-sync`** moved to `/tmp/agents/removed/--Users-yuri-Workdir-Yuri-home-systems--/k8s/images/hermes/op-files-sync` (replaced by `hctl op files sync`). The Dockerfile no longer copies it.

## Current State

- ✅ `k8s/images/hermes/hctl` written, executable, `--help` validated for every subcommand.
- ✅ `k8s/images/hermes/Dockerfile` updated: copies `hctl` to `/usr/local/bin/hctl`; the legacy `op-files-sync` COPY is gone.
- ✅ `k8s/charts/hermes/values.yaml`: removed Obsidian env/volume/PVC and the inline `extraInitContainers` git-login + op-files blobs; added `auth/opFiles/gitRepos` blocks (all default `enabled: false`).
- ✅ `k8s/charts/hermes/templates/_helpers.tpl`: added `hermes-agent.workdirVolumeName` helper.
- ✅ `k8s/charts/hermes/templates/statefulset.yaml`: renders `git-login`, `op-files`, `repos-clone` init containers and `repos-sync` sidecar (all calling `hctl`); mounts `workdir` emptyDir on `hermes-agent` and `repos-sync`.
- ✅ `k8s/applications/hermes.yaml`: collapsed from 300+ lines to ~60.
- ✅ `helm template` validated in both enabled and disabled modes; chart is clean when feature flags are off.
- ✅ Image rebuilt and pulled by the cluster (489MB pull observed); pod now shows `git-login` + `op-files` containers running `hctl` commands.
- ⚠️ Stale `hermes-obsidian` and `hermes-repos` PVCs still bound in the `hermes` namespace (orphans from prior renders). ArgoCD app status `OutOfSync, Progressing, Failed`.
- ❌ `hermes-0` stuck in `Init:0/4` — `mkfs.ext4: No such device or address` on Longhorn-attached `state-hermes-0`. Independent of this refactor.
- ❌ `nostos hermes refresh|exec|shell` not started; `.bin/hermes-helper` still present (deferred).

## Next Steps

1. **Resolve the Longhorn mount race** (user's call — production mutation):
   - `talosctl -n 192.168.68.100 ls /dev/longhorn/` to confirm whether the device node is actually present on dell01.
   - If absent: bounce the dell01 longhorn-csi-plugin pod (`longhorn-csi-plugin-4qzqm`) to re-register the iSCSI target.
   - If present but mkfs still fails: longhorn-manager bounce on dell01.
2. **Clean stale PVCs:** `kubectl -n hermes delete pvc hermes-obsidian hermes-repos` once we confirm no pod references them.
3. **Recover rpi01:** still NotReady; needs `nostos` attention.
4. **Resume the deferred nostos hermes work** once `internal/cli/{root,commands,schema/schema}.go` are clean from the parallel ship session. Then `mv .bin/hermes-helper` to trash.
5. **Optional hardening:** add `--soft-fail` to `hctl op files sync` so a transient Connect outage doesn't block hermes startup (today it correctly exits non-zero on connection refused, which gates the whole pod).
