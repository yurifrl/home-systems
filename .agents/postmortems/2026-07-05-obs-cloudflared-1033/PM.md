---
date: 2026-07-05
status: closed
incident_status: resolved
sessions:
  - 019f32a5-b892-78d1-916c-943844c3a827
components:
  - cloudflared
  - obs
  - macintel01
symptoms:
  - Cloudflare Error 1033 (Tunnel error) on obs.syscd.live
  - HLS stream unreachable from outside
failure_mode: cloudflared-tunnel-connector-gap
affected_urls:
  - https://obs.syscd.live/obs/index.m3u8
beads: [home-systems-nxj]
memories:
  - obs-cloudflared-1033-macintel01-2026-07-05
supersedes: []
related:
  - 2026-07-05-hermes-rwx-sharemanager-cross-site
  - 2026-07-05-cloudflare-tunnel-quic-edge-unreachable
---

# Postmortem: obs.syscd.live Error 1033 during a cloudflared-obs restart on macintel01

- **Severity/Impact:** `obs.syscd.live` HLS stream unreachable from outside for
  ~2 minutes (Cloudflare Error 1033) around `2026-07-05 14:17–14:19 UTC`.
  Self-healed with no operator action. No data loss; on-demand stream only.
- **Root cause (one line):** cloudflared-tunnel-connector-gap — the dedicated
  `cloudflared-obs` connector crashed and restarted on macintel01 (flaky
  roaming laptop VM); during the restart gap the tunnel had no registered
  connector, so Cloudflare served Error 1033.

## What Happened

`obs.syscd.live` is served by a dedicated Cloudflare tunnel
(`248f4b51-d33d-4896-89c1-acb748c54330`) whose only connector is the
`cloudflared-obs` deployment. Both `cloudflared-obs` and `mediamtx` are pinned
to **macintel01** — the only node on the OBS LAN, so mediamtx can pull the OBS
SRT source (`srt://192.168.0.25:1234`) and cloudflared can reach mediamtx on
`localhost:8888`. This is an inherent single point of failure: the stream
cannot be made HA because both halves must live on that one node.

macintel01 (a roaming Intel-Mac UTM VM, known-flaky) had an outbound-network /
node-reachability blip. `cloudflared-obs`'s startup connectivity precheck to
the Cloudflare edge hard-failed (QUIC+TCP to `region{1,2}.v2.argotunnel.com:7844`
and `api.cloudflare.com:443` all FAIL, plus `Failed to initialize DNS local
resolver ... i/o timeout`), so the container exited with code 1 and entered
CrashLoopBackOff. While no connector was registered, Cloudflare had no origin
for the tunnel and returned **Error 1033** — which is exactly the frame the
user captured at 14:18:59 UTC. Once macintel01's connectivity recovered,
cloudflared re-registered all four edge connections and the stream returned.

## Detection Gap (how we catch it next time)

- **What the user saw first:** the Cloudflare Error 1033 page on
  `obs.syscd.live/obs/index.m3u8` — no monitoring flagged it; the user did.
- **How we detect it before the user next time:** an external gatus check on
  `https://obs.syscd.live/` that expects the Cloudflare Access redirect
  (`[STATUS] == 302`, `ignore-redirect: true`). A registered tunnel returns
  302 (Access login) regardless of whether a stream is live; Error 1033 returns
  530. This is a **tunnel-up** symptom check, independent of stream liveness —
  it does NOT false-fire when nobody is streaming. gatus is the right place:
  the cluster cannot alert on its own tunnel death from the inside, and this
  tunnel connector isn't scraped by VictoriaMetrics.
- **Nuisance guard:** obs blips self-heal in ~1–2 min (macintel01 is flaky by
  design). Use a per-endpoint `failure-threshold` of ~30 checks (~5 min at
  10s interval) so transient self-healing gaps do NOT page, but a sustained
  tunnel outage does.
- **Fix path once detected:** check `cloudflared-obs`/`mediamtx` on macintel01
  (`kubectl -n obs get pods -o wide`, `kubectl -n obs logs deploy/cloudflared-obs`)
  and macintel01 node reachability — see runbook below.

## Mitigation (runbook — how to detect & fix this again)

**This incident self-healed** — no action was taken; documenting for next time.

1. Confirm the symptom: `curl -s -o /dev/null -w '%{http_code}\n'
   https://obs.syscd.live/obs/index.m3u8` — `302` = tunnel healthy (Access
   redirect), `530`/timeout = Error 1033 (connector down).
2. Check the connector: `kubectl -n obs get pods -o wide` (both pods must be
   Running on macintel01) and `kubectl -n obs logs deploy/cloudflared-obs
   --tail=40` — look for `Registered tunnel connection` (healthy) vs a
   precheck `hard_fail=true` / CrashLoopBackOff (macintel01 can't reach the
   Cloudflare edge).
3. Check the node: is macintel01 `Ready` and reachable? A `node.kubernetes.io/
   unreachable` taint or NotReady state is the usual trigger. If macintel01 is
   wedged, that is the thing to recover (separate, known macintel01 fragility);
   the obs pods follow the node.
4. Confirm the stream source: `kubectl -n obs logs deploy/mediamtx --tail=25`
   should show `[path obs] stream is available and online` — otherwise OBS
   itself isn't pushing SRT (not a tunnel problem).

**The precheck 7844 QUIC/TCP failures in cloudflared-obs logs are benign** —
the config forces `protocol: http2`, so it connects over TCP/443 to the edge
even when 7844 is blocked. They are not the cause; a crash is only real when
the container actually exits (exitCode 1) and stops registering connectors.

## Follow-ups Implemented (epic home-systems-nxj)

See `FP.md` for the full ledger.

- **gatus check (DONE, `home-systems-nxj.1`):** `OBS Stream` endpoint in
  `nixos/modules/gatus/config.yaml` — `[STATUS] == 302`, `ignore-redirect`,
  `failure-threshold: 30` (~5 min) → Discord. Pending a nixos rebuild to
  deploy.
- **Open verification (`home-systems-nxj.2`, P3):** confirm the check shows in
  gatus and a sustained failure routes to Discord.
- **Memory:** `obs-cloudflared-1033-macintel01-2026-07-05`.
- **Rejected:** m3u8 stream-liveness alert (on-demand → nuisance), cloudflared
  restart alert (routine), cloudflared VMRule/scrape (cause-level, inside the
  dying node), obs HA (architecturally impossible — both halves must share
  macintel01).

## Dead Ends

- The cloudflared connectivity-precheck banner (`QUIC connection failed`, `TCP
  ... blocked`, `api.cloudflare.com ... FAIL`, `SUMMARY: critical failures`)
  looks alarming but is a red herring in steady state — with `protocol: http2`
  the tunnel registers fine over TCP/443. It only mattered here as a symptom
  of the wider macintel01 outbound blip, not as an independent fault.

## Timeline

### 2026-07-05 (UTC)
- `14:17:13` `cloudflared-obs` container terminates with exitCode 1 (startup
  connectivity precheck to Cloudflare edge hard-failed) → CrashLoopBackOff.
- `14:18:23` New attempt's precheck reports all edge targets FAIL
  (QUIC+TCP 7844, api.cloudflare.com:443) and DNS resolver i/o timeout —
  macintel01 outbound connectivity blip.
- `14:18:59` User captures Cloudflare **Error 1033** on
  `obs.syscd.live/obs/index.m3u8` (Ray ID a16700870c140180) — no connector
  registered for the tunnel.
- `14:19:13–14:19:16` cloudflared registers 4 edge connections
  (gru13/gru21/gru19/gru20, protocol http2) — tunnel recovered.
- `~14:21` Verified recovery: external `obs.syscd.live/obs/index.m3u8` → HTTP
  302 (Cloudflare Access redirect); `mediamtx` logs show `[path obs] stream is
  available and online, 2 tracks (H264, AAC)` and active HLS muxing. Both obs
  pods Running 1/1 on macintel01.
