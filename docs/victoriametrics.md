# VictoriaMetrics: Expectations vs Design Reality

A short note on where my expectations of VictoriaMetrics (VM) collided with how
VM is actually designed, so we don't repeat the misunderstanding.

## What I expected

I wanted **object storage as a backing filesystem**: keep recent data local, push
old data to S3/GCS, and when I query a far-past range the first request is slow
(fetched from object storage) but subsequent ones are fast (cached). Seamless,
cheap, effectively bottomless history. I assumed adding a GCS bucket to VM would
give me this.

## How VM is actually designed

- **All data lives on local disk** (the PVC). Queries only ever read local disk.
- **`-retentionPeriod` deletes** data older than the window from disk. Old data
  is **not** offloaded to S3 — it is gone.
- **S3/GCS is backup-only** (`vmbackup` / `vmbackupmanager`): full snapshots you
  *restore* to local disk. You cannot query a backup in place.
- VM's official answer to "keep more history" is **a bigger local disk + longer
  retention**, not object-storage tiering. This is a deliberate design choice
  (local disk is fast and cheap), not a missing feature.
- Scheduled/retained backups (`vmbackupmanager`) are an **Enterprise** feature
  (free trial license); one-shot `vmbackup` is open source.

## The gap

| Expectation | VM reality |
|---|---|
| S3 as queryable backing store | Local disk only; S3 = backup/restore |
| Old data fetched on demand from S3 | Old data deleted at retention |
| Slow-first / cached-after historical reads | No object-storage query path exists |
| Add a bucket → infinite cheap history | A bucket only enables snapshots |

## What actually does what I wanted

The "slow first, fast after" object-storage query behaviour is **Thanos** and
**Grafana Mimir** (and Cortex). They use object storage as the long-term store
and fetch/cache blocks on query via a store-gateway.

- **Thanos** — sidecar onto existing Prometheus, CNCF, Apache-2.0, most popular.
- **Mimir** — Grafana-led, AGPLv3, push/remote-write, strong multi-tenancy.
- **Cortex** — superseded by Mimir; avoid for new setups.

## Takeaways

1. If the goal is **cheap, bottomless, queryable history** → that's a
   Thanos/Mimir-shaped problem, not VM. Adding a bucket to VM does not deliver it.
2. If the goal is **long history, always fast, simple** → VM with a bigger PVC +
   longer retention.
3. If the goal is **disaster recovery only** → VM + `vmbackup`(OSS one-shot) or
   `vmbackupmanager` (Enterprise) to GCS. This is the only legitimate VM use of
   object storage.
