---
date: 2026-07-05
status: closed
incident_status: mitigated
sessions:
  - 019f3483-f957-757c-b783-6499199b27dc
components:
  - cloudflared
  - tp4
symptoms:
  - Cloudflare Error 1033 (Tunnel error) on argocd.syscd.live
  - Cloudflare Error 524 (a timeout occurred) on argocd.syscd.live
  - cloudflare-tunnel connector crashlooping (CrashLoopBackOff)
  - "failed to dial to edge with quic: timeout: no recent network activity"
  - external syscd.live services flapping (302 from curl, 1033 in browser)
failure_mode: cloudflared-quic-edge-unreachable
affected_urls:
  - https://argocd.syscd.live
  - https://zigbee2mqtt.syscd.live
beads: [home-systems-c67]
memories: [cloudflare-tunnel-http2-quic-fix-2026-07-05]
supersedes: []
related:
  - 2026-07-05-obs-cloudflared-1033
  - 2026-07-11-vmsingle-storage-readonly
---

# Postmortem: main cloudflare-tunnel down — QUIC to Cloudflare edge unreachable

- **Severity/Impact:** All `*.syscd.live` / tunnel-fronted services (argocd, zigbee2mqtt, etc.) flapping between reachable and Cloudflare **Error 1033 / 524** for ~1h the night of 2026-07-05. The connector served intermittently, so `curl` saw 302 while the browser hit zero-connector windows and got 1033.
- **Root cause (one line):** `cloudflared-quic-edge-unreachable` — the `cloudflare-tunnel` (nixos-1) connector could not establish **any** QUIC (UDP) connection to the Cloudflare edge (`failed to dial to edge with quic: timeout: no recent network activity`); TCP egress was fine, so the fix was to force `protocol: http2` (TCP/443).

## What Happened
The main tunnel runs a single cloudflared connector (helm chart `cloudflare-tunnel` 0.3.2) that maintains 4 QUIC connections to Cloudflare's edge. Two connections dropped at ~22:47–22:50 UTC and then all four failed to re-establish over QUIC with `no recent network activity` — UDP/QUIC egress to the edge (198.41.192.x) had broken, while TCP/443 to the same edge still worked. cloudflared's aggressive `/ready` liveness probe then killed the connector during its failed startup, producing CrashLoopBackOff. With 0 registered connectors Cloudflare returned 1033; during brief partial-registration windows it returned 524 or served 302 — hence the flapping. The connector had been running on tp4, which was BMC-reset earlier in the same session (~22:35 UTC); the post-reboot UDP path on tp4 is the most likely trigger.

## Detection Gap (how we catch it next time)
- **What the user saw first:** Error 1033 (and 524) on argocd.syscd.live in the browser, while my `curl` intermittently showed 302 (flapping fooled a single probe).
- **How we detect it before the user next time:** the tunnel is a single point of failure fronting *all* `*.syscd.live` services, but only argocd has an external gatus check and nothing scrapes cloudflared's own health. Scrape cloudflared metrics (`:2000/metrics`) and alert when `cloudflared_tunnel_ha_connections == 0` (or `< expected`) — one SPOF symptom alert that covers every tunnel-fronted service at once, and catches partial degradation (2/4 down) before it becomes a full outage. This is the primary alert candidate.
- **Fix path once detected:** force `protocol: http2` durably (see Mitigation + bead) — QUIC-to-edge is the flaky path here.

## Mitigation (runbook — how to detect & fix this again)
Symptom: `*.syscd.live` returns Cloudflare 1033/524 while pods/origin are healthy; `kubectl -n cloudflare-tunnel logs deploy/cloudflare-tunnel` shows `failed to dial to edge with quic: timeout: no recent network activity` and the pod is CrashLoopBackOff.

Confirm it's QUIC (not origin/egress): from a pod on the connector's node, TCP/443 to a CF edge IP (e.g. `nc -zvw3 198.41.192.7 443`) works and DNS/HTTPS work, but QUIC connections never register. The OBS tunnel (which forces http2) staying healthy is a live control that proves TCP works and QUIC is the broken path.

Fix (break-glass, worked instantly):
```
kubectl -n cloudflare-tunnel set env deployment/cloudflare-tunnel TUNNEL_TRANSPORT_PROTOCOL=http2
# new pod registers 4 connections over TCP/443 within ~25s; argocd.syscd.live -> 302
kubectl -n argocd patch application cloudflare-tunnel --type merge \
  -p '{"spec":{"syncPolicy":{"automated":{"selfHeal":false,"prune":true}}}}'
```
The second step is required because the upstream chart 0.3.2 exposes no `protocol`/`env`/`extraArgs` (its `configmap.yaml` hardcodes the config keys), so the env var cannot be set via helm values — argo `selfHeal` would otherwise revert the live patch back to QUIC and regress. **selfHeal is currently OFF on this app** until the durable fix lands; do not re-run `task argo:apply` on cloudflare-tunnel or it reverts to QUIC.

Durable fix: self-manage the cloudflared Deployment like `k8s/charts/obs/cloudflared.yaml` (which sets `protocol: http2` in `config.yaml`), then re-enable selfHeal.

## Dead Ends
- **Suspected the pc01 tailscale reset (both the user and I initially).** Ruled out: pc01 is a cordoned, different node/path; the tunnel connector runs on tp4. The connection drops (22:47) and the tp4 BMC reset (22:35) line up far better than the pc01 work.
- **`nc -zvuw3 <edge> 443/7844` from tp4 reported UDP "succeeded"** — misleading; UDP `nc` reports success without an ICMP reject, so it didn't reveal the broken QUIC path. The definitive signal was cloudflared's own repeated `quic: no recent network activity`.
- **A single `curl` returning 302 looked like "it's fine"** — the connector was flapping; one probe hit a good window. Poll repeatedly, or check registered connection count, when diagnosing tunnel flap.
- **524 vs 1033 confusion:** 524 = connector registered but origin/window timed out; 1033 = zero connectors. Both were the same flapping-tunnel root cause, not two problems.

## Timeline
### 2026-07-05 (UTC)
- `~22:35` tp4 BMC-reset earlier in the session (node reboot work); cloudflared connector had been on tp4.
- `22:47` cloudflared connIndex=0 unregistered; `22:50` connIndex=1 unregistered (2 of 4 QUIC connections lost).
- `~22:39` (local screenshot) argocd.syscd.live shows Error 1033 in browser.
- `23:35:54` argocd.syscd.live shows Error 524 (screenshot).
- cloudflared crashlooping; all 4 QUIC connections fail `no recent network activity`; ~10 restarts.
- argocd-server confirmed Healthy (Ready, 0 restarts) on tp4 — origin fine, connector was the fault.
- tp4 pod egress tested: DNS, ping 1.1.1.1, TCP/443 to edge, HTTPS trace all OK — only QUIC/UDP failing.
- OBS tunnel (http2) confirmed 302 throughout = proof http2/TCP works.
- Applied `TUNNEL_TRANSPORT_PROTOCOL=http2`; new pod scheduled on tp1, registered 4 connections, argocd → 302.
- Disabled argo `selfHeal` on cloudflare-tunnel so the http2 patch holds; argocd/zigbee2mqtt/obs all 302.
