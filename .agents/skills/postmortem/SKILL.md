---
name: postmortem
description: Use AFTER an incident is mitigated or fixed (not before/during) to document what happened, propose monitoring/alerts, and create follow-ups. Triggers - "write a postmortem", "document this incident", "create alerts for what just happened", an outage/bug was just resolved in this session.
---

# Postmortem

Turn a just-resolved incident into: a postmortem document, an implementation plan (alerts/monitoring/beads), and — after approval — the real artifacts. The caller (you) already has the incident context in-session — this skill tells you where everything goes so the work is minimal.

**One directory per incident, two files, hard split.** `.agents/postmortems/<YYYY-MM-DD>-<slug>/`:
- `PM.md` (from `PM-template.md`) — the postmortem. Pure documentation: what happened, detection gap, runbook, timeline. It may be updated (reopen, new occurrence, incident_status change) but it NEVER declares things to be changed — no proposals, no "will be created".
- `FP.md` (from `FP-template.md`) — the follow-up plan. Created only AFTER the user approves implementation. It declares exactly what gets created — beads, alerts, dashboards, gatus — every item marked `[CREATE]`/`[EDIT]`/`[DELETE]`/`[SKIP]` with exact paths, and is then updated in place with the resulting bead ids, commits, and links. It persists as the implementation ledger. No FP.md = nothing approved yet.

**Lifecycle.** First run writes PM.md (`status: draft`) and presents the proposed follow-ups in chat — not in a file. On explicit approval ("implement"/"approve" — vague assent like "looks good" is NOT approval, ask): create FP.md from the approved items, execute them, update FP.md with ids/links as each lands, write the epic id into PM.md's `beads:`, set PM.md `status: closed`. Never create alerts/dashboards/beads before approval.

**No hedged proposals — self-discover instead.** Never write "if present", "verify whether X exists", or "consider whether" in the file. Those are lookups YOU can do now: read the gatus config, grep the VMRule/dashboard dirs, `bd list` for existing follow-ups — then state what IS there and propose the concrete delta. A proposal that asks the reader to go check something is an unfinished proposal.

**Associate-only invocations are valid.** The user can run this skill on an existing incident just to link the current session to it ("associate this with <postmortem>"): append the session id to that PM.md's `sessions:` list and reconcile any genuinely new facts from this session (timeline events, dead ends, incident_status). No new file, no new proposals unless the session surfaced something new.

**Idempotent by design.** The postmortem file IS the current state; every run is a reconcile, not a rewrite. On each invocation: locate the existing file (steps 1–2), read it fully, diff it against what this session knows, and apply only the delta — new timeline events, new dead ends, changed `incident_status`, new alert rows, the current session id. Never duplicate an event, alert row, or follow-up that's already there; never rewrite history. Running the skill ten times on the same incident must converge to the same file.

The postmortem is documentation; the incident has its own state. The template carries both: `status` (lifecycle of this document) and `incident_status` (`mitigated` = band-aid/temporary fix in place, `resolved` = durable fix). A `mitigated` incident still passes the resolution gate — just say so.

## Workflow

0. **Resolution gate.** A postmortem is only for a problem believed resolved OR mitigated. From the session, identify: what was the fix/mitigation, evidence it worked (passing check, healthy status, stable curl), and whether it is durable (`resolved`) or temporary (`mitigated`). If some of this is unclear, still do everything that doesn't depend on the answer and queue the question (see Asking questions below).
1. **Get the current pi session id** (dedupe key — subsessions of an already-documented session must not re-document). In order of preference:
   1. A `session_id` tool, if available (also gives the parent id when running as a subagent).
   2. The newest session file for this cwd:
      ```bash
      command ls -t "$HOME/.pi/agent/sessions/$(pwd | sed 's|/|-|g; s|^|-|; s|$|--|')/"*.jsonl | head -1 | sed 's/.*_//; s/\.jsonl//'
      ```
   3. Heuristic fallback: `<YYYY-MM-DD>-<slug>` of the incident.
   If that id (or the parent session's id, when running as a subagent) is already in a postmortem's `sessions:` frontmatter list, the incident is already documented from this session tree: do NOT create or re-append the same events — only add what is genuinely new, and add the current id to the list.
2. **Classify against prior art — always, before writing anything.** `components` and `failure_mode` use the controlled vocabulary in `vocab.md` (same directory as this skill) — pick existing tags when they fit, append new ones when they don't. Free-text symptoms are what makes recurrence grep miss ("502" vs "1033" vs "connection reset" for the same fault); tags are the dedupe key. Grep postmortem frontmatter: `grep -l -i '<tag>' .agents/postmortems/*.md`, plus `bd memories <keyword>`. Then classify:
   - **Recurrence** (same component + same failure_mode): do NOT create a new file. Reopen the existing one — add a new dated Timeline block, set `status: reopened`. A recurrence means the previous follow-ups failed: `bd show` each bead in the old epic and state in the plan, per bead, whether it was **closed-but-ineffective** (the fix didn't work) or **never-done** (nobody did it) — these are different lessons. The plan MUST propose something stronger than last time (durable fix, better alert) — say so explicitly.
   - **Related** (shared component OR overlapping symptoms, different failure_mode): create the new file with the old one in `related:`, AND edit the old file's `related:` to point back. Links are always bidirectional.
   - **New**: create fresh from the template.
   If a match is ambiguous (looks like a past incident but you're not sure it's the same failure mode), don't guess — write the postmortem as New and queue the question "is this a recurrence of <old file>?" for the user; incorporate the answer (merge/reopen/relate) when it comes.
   While reading old postmortems, act as a curator: if their metadata is missing fields, stale, or should now link to this incident, include those metadata edits in the plan. If an existing postmortem covers the same failure mode: **reopen it** — append a new dated entry to its Timeline and update proposals — instead of creating a new file.
3. **Reconstruct the timeline** from the session (kubernetes-events style: timestamped, factual, one line each). Timeline is mandatory — it is the knowledge-base value. Include the dead ends: red herrings chased, wrong theories held, probes that misled — they go in the Dead Ends section and are often the most valuable content.
4. **Detection gap first.** The user caught this error — that's why the incident exists. Record in the plan: (a) exactly what the user saw first (the symptom that beat our monitoring), (b) how we detect that same symptom BEFORE the user next time — this is the primary alert candidate, and (c) the fix path once detected — that's a follow-up bead. Then, for each proposed signal, apply Alert Hygiene (below) and record the verdict.
5. **Write PM.md** (`PM-template.md`), then present the proposed follow-ups in chat and ask for approval. Before proposing anything, do the self-discovery lookups (existing gatus entries, VMRules, dashboards, open beads) so every proposal states current state + concrete delta with its action marker. Record the current session id in PM.md's `sessions:` frontmatter list; on reopen, append the new id (never remove old ones). `beads:` and `memories:` stay empty in draft.
6. **On approval**: create FP.md from the approved items, then execute each exactly as marked — alerts/dashboards/gatus per the target locations below, the beads epic + tasks, and `bd remember` a condensed root-cause+fix summary keyed like `<component>-<failure>-<date>`. Then close the loop:
   - update each FP.md item in place with its bead id / commit / link
   - write the epic id into PM.md's `beads:` and the memory key into `memories:`
   - put the PM.md path in the epic's description (bidirectional, like `related:`)
   - set PM.md `status: closed`
   Whenever a follow-up bead from the epic is later closed or found stale, update the postmortem file too (e.g. `incident_status: resolved` once the durable fix lands).

## Alert Hygiene (EEMUA 191 — this is the filter, not a suggestion)

Every proposed alarm must answer: **"does this require a human to act, now?"**

- Yes, act now → **alert** (VMRule severity=critical, or gatus). Example: `argocd.syscd.live` returns non-2xx/3xx from outside — a human must intervene.
- Useful context, no action → **dashboard panel**, not an alert. Example: cilium-health endpoint reachability count — explains an outage, doesn't demand action by itself.
- Diagnostic detail → **logs/metrics only**, propose nothing. Example: ztunnel per-connection logs, VXLAN drop reasons — you grep these DURING an incident, never page on them.
- Rejected outright → **skip**, record why. Example: "alert when a pod restarts" — restarts are routine; the symptom alert on the user-facing endpoint already covers the case that matters.
- Budget: aim for ~zero pages in steady state; a standing or nuisance alarm is a bug — propose deleting/fixing existing noisy alerts as part of the postmortem
- Prefer **symptom** alerts (user-facing endpoint down, error budget burn) over **cause** alerts (one internal component wobbling). One symptom alert usually replaces several cause alerts.
- Every alert's `description` must say what to do (or link the postmortem file — it IS the runbook).

## Where things go

| Thing | Location & pattern |
|---|---|
| Prometheus alert | `VMRule` (VictoriaMetrics operator, NOT PrometheusRule) in `k8s/charts/support-cluster/templates/monitoring/<topic>.yaml`. Copy style from `node-resources.yaml`: labels `prometheus: k8s`, `role: alert-rules`, namespace `monitoring`, annotation `argocd.argoproj.io/sync-options: SkipDryRunOnMissingResource=true`. Before proposing, READ `k8s/charts/support-cluster/templates/alertmanagerconfig.yaml` to confirm which alert labels actually route to a receiver (as of last check: `severity: critical` + `environment: production` → Discord; anything else goes nowhere). Escape Go templates in Helm: `{{` `{{ $labels.instance }}` `}}` |
| New scrape target | `VMServiceScrape` (no monitoring.coreos.com CRDs on this cluster) — copy `k8s/charts/support-cluster/templates/monitoring/home-assistant-vmservicescrape.yaml` |
| Grafana dashboard / panel | JSON in `k8s/charts/support-cluster/dashboards/<name>.json` — auto-wrapped into a sidecar ConfigMap by `templates/monitoring/grafana-dashboards.yaml` (label `grafana_dashboard=1`). Datasource must be `${datasource}`. Prefer adding a panel to an existing dashboard over a new file. |
| External / outside-infra check | Gatus endpoint in `/Users/yuri/Workdir/Yuri/nixos/modules/gatus/config.yaml`. Simple rule: in-cluster alerts are lost when the cluster or alert infra is down — gatus tests from OUTSIDE, so use it when that loss matters, or when the internet→service path itself is what's being verified (usually live URLs). If neither applies, it's a VMRule. Before proposing one, check the config: if an existing entry already covers the endpoint/cluster connectivity, it's covered — propose nothing. Pattern: `name`, `group: Live|Tech`, `url`, `interval: 10s`, conditions `[STATUS] == 200`, `[RESPONSE_TIME] < 2000`, optional `[BODY] == pat(*...*)`, `alerts: [{type: discord}]`. Cloudflare-Access-protected URLs need the `CF-Access-Client-Id/Secret` headers (see ArgoCD entry). |
| Follow-ups | Beads: `bd create --type=epic --title="Postmortem: <name>" --labels=pm:<YYYY-MM-DD>-<slug>` then child tasks with `--parent=<epic-id>` (they inherit the label) for each remaining action (durable fixes, alert cleanup, docs). The `pm:<slug>` label is the per-incident filter: `bd list -l pm:<slug> --all`. Reference the PM.md path in the epic description AND the epic id in PM.md's `beads:` frontmatter. For every alert created, add a low-priority (P3) task "verify <alert> actually fires and routes" (temporary threshold trip, or amtool/vmalert check). |
| Knowledge | The postmortem file itself (mitigation section = runbook) + `bd remember` |

**Deploy via ArgoCD (git commit → sync), never `kubectl apply`.**

## Asking questions

Never block on a question. Do everything that doesn't depend on the answer
first, then ask ALL open questions in ONE message at the end. When the user
responds, incorporate the answers into PM.md/FP.md — that's just another
idempotent reconcile pass.

Every question must advance the agenda — better detection, more assertive
follow-ups — and be a yes/no question whenever possible (state your
recommended default so "yes" is enough). The test: does the answer change a
concrete artifact (an alert, a threshold, a bead, a file)? If not, don't ask
it. Allowed shapes:
- disambiguation: "is this a recurrence of <old file>?", "was the fix durable
  or temporary?"
- artifact decisions: "include the metallb exclusion (item 4)? (I'd include
  it)", "use a 5m alert threshold? (recommended)"

NEVER ask meta/process/retrospective questions ("what's your bar for asking
first?", "was this the right outcome?", "how do you want proposals
delivered?") — process feedback is the user's to volunteer, not yours to poll
for. If unsure about process, follow the skill and this repo's rules; that's
what they're for.

MOST IMPORTANT: questions stay INSIDE the scope of the current incident and
this repo. Never question, challenge, or relitigate the user's global
AGENTS.md, standing rules, or anything outside the incident's scope — those
are settled; apply them silently.

## Editing this skill

If you change this skill (templates, frontmatter fields, directory pattern,
lifecycle), ask the user whether to retro-update the existing postmortems in
`.agents/postmortems/` to the new pattern. If yes, migrate every PM.md/FP.md
in place — content preserved, structure updated — without inventing new facts.
If no, don't nag: next time a postmortem is created and step 2's prior-art scan
touches an old-pattern file, include "migrate <file> to the current pattern?"
in the single questions message.

## Common mistakes

- Alerting on the cause you just debugged instead of the symptom users saw
- Creating a new postmortem when one exists for the same failure mode — search first, reopen
- Skipping the Mitigation section — that section is why the file exists (it's the knowledge base for next time)
- `severity: warning` VMRules — they route nowhere here; either it's critical+production or it's a dashboard panel
