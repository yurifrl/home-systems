# Hermes Memory Git Backup — Design

Date: 2026-07-18

## Problem

Hermes stores its entire state in `HERMES_HOME` (`/opt/data`, backed by the
`hermes-state` PVC). Today that volume is snapshotted nightly by a Longhorn
`RecurringJob` (`k8s/charts/hermes/templates/backup.yaml`, retain 30). We want
to **also** push the durable, human-meaningful subset of that state to a
private git repo, for two reasons:

- **Disaster-recovery redundancy** — an off-cluster copy independent of Longhorn.
- **Versioning / visibility** — commit history and diffs of the agent's memory,
  skills, cron, and identity over time.

Longhorn stays as the full-state DR mechanism (secrets, session DBs, caches).
Git backup is a curated, secret-free overlay — not a replacement.

## Constraint: HERMES_HOME mixes safe and unsafe data

Observed layout of `/opt/data`:

| Bucket | Paths | Git? |
|---|---|---|
| Durable + safe | `SOUL.md`, `memories/`, `skills/`, `cron/`, `config.yaml` | **yes** |
| Secrets | `.env`, `auth.json`, `channel_directory.json`, `gateway_state.json`, `whatsapp/`, `pairing/`, `home/` | never |
| Ephemeral / DBs / caches | `state.db*`, `kanban.db*`, `response_store.db*`, `sessions/`, `logs/`, `cache/`, `*_cache.json`, `audio_cache/`, `image_cache/`, `sandboxes/`, tmp/lock files | never |

`config.yaml` was inspected and contains only settings (agent/model/security
config), no secrets — safe to version.

Two hard rules from practitioner research (see References):

1. **Allowlist, not denylist.** A `.gitignore` denylist fails open — the moment
   the app writes a new secret file the pattern didn't anticipate, it leaks, and
   git history is forever. We only ever copy 5 known-safe paths, and add a
   fail-closed `.gitignore` (`*` + `!<the 5 paths>`) inside the repo as
   defense-in-depth.
2. **Never git a live WAL-mode SQLite DB.** Copying `state.db`/`kanban.db`
   mid-write yields a corrupt backup. All DBs are excluded from the git path and
   remain owned by the Longhorn snapshot.

## Approach

Reuse the existing `hctl` git plumbing rather than add a new tool. `hctl repos
sync` (`k8s/images/hermes/hctl`) already implements the hardened loop:
`pull --rebase --autostash → add -A → commit if dirty → push if ahead`, with
upstream-tracking setup and a SIGTERM/SIGINT final pass. The only missing piece
is copying a scattered allowlist out of `HERMES_HOME` into a repo working tree
(we cannot `git init` the live volume — secret risk and it is actively written).

### 1. New `hctl backup` subcommand

In `k8s/images/hermes/hctl`, add `cmd_repos_backup` (~30 lines) that each cycle:

- Clone-or-pull the backup repo into `/backup/<repo>` (reuse `_reconcile_branch`;
  handle the empty-repo first-push case where clone yields no branch).
- Ensure a fail-closed `.gitignore` exists in the repo: `*` then
  `!SOUL.md`, `!memories/`, `!memories/**`, `!skills/`, `!skills/**`,
  `!cron/`, `!cron/**`, `!config.yaml`.
- `rsync -a --delete` the 5 allowlisted paths from `HERMES_HOME` into the repo.
- Call the existing `_sync_one()` to commit + push.
- Loop on `--interval`, reusing the same signal-handling / final-pass structure
  as `cmd_repos_sync`.

New CLI: `hctl backup --repo <name> --user <gh-user> --home <HERMES_HOME>
--dest /backup --interval 1h`.

### 2. Backup deployment

New template `k8s/charts/hermes/templates/memory-backup-deployment.yaml`,
mirroring `repository-sync-deployment.yaml`:

- Same image; `hctl auth login` init container for git credentials.
- Mounts the `hermes-state` PVC.
- Runs `hctl backup --repo hermes-memory --interval 1h`.
- `strategy: Recreate`, single replica.
- Gated behind `memoryBackup.enabled`.

### 3. Restore-on-empty init container

Add an init container to the StatefulSet (`statefulset.yaml`), gated by
`memoryBackup.restore.enabled`:

- If `$HERMES_HOME/memories` is **absent**, clone the backup repo and copy the 5
  paths in (seed a fresh volume). If present, skip entirely.
- Never overwrites a populated live volume — the guard is existence of
  `memories/`.

### 4. Values

Add a `memoryBackup:` block to `values.yaml` and `values.schema.json`:

```yaml
memoryBackup:
  enabled: false
  repo: hermes-memory
  user: yurifrl
  interval: 1h
  restore:
    enabled: false
```

Wire real values in `home-systems-values/hermes/values.yaml` (enable both,
`interval: 1h`).

## Interval

Hourly. Repo is ~50K, so cost is trivial; the SIGTERM final pass captures
last-second changes on pod shutdown.

## Prerequisites (one-time, manual)

- Create empty private repo `yurifrl/hermes-memory`.
- Confirm the existing git token has `repo` scope (the current
  `repository-sync` already pushes to private repos, so it should).

## Out of scope (deliberately skipped)

- DB dumps / logical exports, session history, secret backup — Longhorn owns
  these.
- gitleaks pre-commit scanner — the allowlist already fails closed; add later as
  defense-in-depth if desired.
- Pointing Longhorn at off-site S3/B2 (separate concern).

## References

- Official Hermes guide: https://hermes-agent.ai/how-to/backup-hermes-memory
  (endorses the same allowlist: memories/skills/cron/sanitized-config; excludes
  secrets/DBs/caches).
- Allowlist-fails-closed consensus: multiple "committed .env" post-mortems
  (dev.to, filter-repo cleanups).
- SQLite live-copy corruption: sqlite.org/howtocorrupt; r/webdev PSA on copying
  WAL-mode DBs.
- Restore clobbering pitfall: k8s git-sync atomic worktree swap breaks open file
  handles → guard restore on empty.
