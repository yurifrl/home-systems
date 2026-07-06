---
date: <YYYY-MM-DD>
status: draft | closed | reopened           # draft = follow-ups not yet approved; closed = beads/alerts created
incident_status: mitigated | resolved       # mitigated = temporary fix, resolved = durable fix
sessions:
  - <pi-session-uuid>  # every session that created/updated this file; dedupe key
components:            # tags from vocab.md — grep target for recurrence detection
  - <argocd | cilium | pc01 | ...>
symptoms:              # what the user saw — the terms a future search will use
  - <502 on argocd.syscd.live | pod DNS timeout | ...>
failure_mode: <tag from vocab.md, e.g. vxlan-tx-checksum-offload>
affected_urls:
  - <https://argocd.syscd.live>
beads: []              # epic id — filled at approval (individual bead ids live in FP.md)
memories: []           # bd-remember keys — filled at approval
supersedes: []         # prior incidents this one absorbs/extends — bare dir slugs, e.g. 2026-07-05-obs-cloudflared-1033
related: []            # incidents sharing components/territory — bare dir slugs (no /PM.md), link BOTH ways
---

# Postmortem: <name>

<!-- DOCUMENTATION ONLY — never declares things to be changed. Approved
     follow-up work lives in the sibling FP.md. Section order is deliberate:
     reader-first (what happened, how we catch it next time, how it won't
     recur), then reference detail, timeline LAST. -->

- **Severity/Impact:** <what broke, for whom, how long>
- **Root cause (one line):** <tag + short phrase>

## What Happened
<one paragraph — root cause, expanded>

## Detection Gap (how we catch it next time)
<!-- The user saw it before monitoring did. This drives which alerts to create. -->
- **What the user saw first:** <symptom>
- **How we detect it before the user next time:** <signal → primary alert candidate>
- **Fix path once detected:** <action → follow-up bead>

## Mitigation (runbook — how to detect & fix this again)
<!-- The knowledge-base core. Exact commands, symptoms, diagnosis path. -->

## Dead Ends
<!-- Red herrings, wrong theories, probes that misled. Often the most valuable section. -->
- <e.g. "Stale/unroutable IP" cilium drop looked like the cause — was a dead pod IP, red herring>

## Timeline
<!-- Authoritative recurrence record: one dated block per occurrence. -->
<!-- kubernetes-events style. Append new dated blocks on reopen; never rewrite history. -->
### <YYYY-MM-DD>
- `HH:MM` <event>
- `HH:MM` <event>
