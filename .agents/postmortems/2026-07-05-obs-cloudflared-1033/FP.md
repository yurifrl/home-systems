---
date: 2026-07-05
pm: PM.md
epic: home-systems-nxj
label: pm:2026-07-05-obs-cloudflared-1033
---

# Follow-up Plan: obs.syscd.live Error 1033

Approved 2026-07-05. Ledger of what was created and where it landed.

## Items

- **[CREATE] gatus external check** — `/Users/yuri/Workdir/Yuri/nixos/modules/gatus/config.yaml`
  - Added `OBS Stream` endpoint (`group: Live`, `url: https://obs.syscd.live/obs/index.m3u8`,
    `[STATUS] == 302`, `client.ignore-redirect: true`, per-endpoint
    `alerts[0].failure-threshold: 30` ≈ 5 min → Discord).
  - Symptom alert on exactly what the user saw (Error 1033 = 530). Does not
    false-fire on an off stream (302 = tunnel up, Access redirect).
  - **DONE** (file edited). Bead `home-systems-nxj.1` — closed. Deploy via nixos rebuild.

- **[CREATE] verify alert** — bead `home-systems-nxj.2` (P3): confirm the OBS
  Stream endpoint shows in the gatus dashboard and a sustained (>5 min) failure
  routes to Discord. **OPEN** — do after nixos deploy.

- **[CREATE] beads** — epic `home-systems-nxj` (label `pm:2026-07-05-obs-cloudflared-1033`),
  children `.1` (closed), `.2` (open). **DONE.**

- **[CREATE] memory** — `bd remember` key
  `obs-cloudflared-1033-macintel01-2026-07-05` (root cause + runbook). **DONE.**

- **[SKIP] m3u8 body / stream-liveness alert** — obs is on-demand (off most of
  the time); a content check would be a standing nuisance alarm.

- **[SKIP] alert on cloudflared pod restarts** — routine; the external gatus
  symptom check covers the case that matters.

- **[SKIP] cloudflared VMRule / VMServiceScrape** — cause-level, pinned to a
  flaky node, and the cluster cannot alert on its own tunnel death from inside;
  gatus (external vantage) is correct.

- **[SKIP] make obs HA** — architecturally impossible: mediamtx (OBS SRT source
  LAN) and cloudflared (localhost:8888) must share macintel01.

## Deploy

gatus config lives in the nixos repo — apply with a nixos rebuild of the gatus
host (not ArgoCD). Not yet deployed.
