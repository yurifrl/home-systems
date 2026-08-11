---
date: 2026-08-11
status: closed
incident_status: mitigated
sessions:
  - 019ff164-5a24-73d0-ac4b-da72590a5d22
components:
  - tailscale
  - istio-gateway
  - external-dns
symptoms:
  - "*.syscd.tech unreachable (argocd.syscd.tech etc. time out)"
  - ts-istio-gateway-wsmws-0 crashlooping ~every 10min (1070 restarts)
  - "boot: failed to get a reissued authkey: timeout waiting for auth key reissue after 10m0s"
  - "invalid key: API key ...CNTRL not valid / You are logged out"
  - operator flapping TailscaleProxyReady True<->False with statefulset optimistic-lock churn
  - tailnet device syscd-gateway offline (last seen 6d ago) while DNS still targets it
  - fresh proxy re-registered as syscd-gateway-1 (stable hostname held by dead device)
failure_mode: tailscale-operator-proxy-orphaned-device
affected_urls:
  - https://argocd.syscd.tech
  - https://grafana.syscd.tech
beads:
  - home-systems-sys
memories:
  - tailscale-gateway-orphaned-device-2026-08-11
supersedes: []
related:
  - 2026-07-05-pc01-tailscale-flag-drift-crashloop
---

# Postmortem: *.syscd.tech down — Tailscale operator gateway proxy orphaned, stale device held the stable hostname

- **Severity/Impact:** every `*.syscd.tech` host (argocd, grafana, and the rest of the tailnet-only ingress domain) was unreachable. The DNS chain was intact; the Tailscale device backing the domain was down. The user was the monitor ("investigate why .tech is not working"). `*.syscd.live` (Cloudflare tunnel path) was unaffected.
- **Root cause (one line):** `tailscale-operator-proxy-orphaned-device` — the operator-managed ingress proxy for `istio-gateway` (tailnet device `syscd-gateway`, target of every `*.syscd.tech` record) lost its registration and fell into a ~10-min auth-key-reissue crashloop; the dead `syscd-gateway` device stayed registered in the tailnet holding the stable hostname, so even a clean re-provision could only register as `syscd-gateway-1` while DNS kept pointing at the dead `syscd-gateway`.

## What Happened

`*.syscd.tech` is served entirely inside the tailnet: external-dns (the `tailscale` release, `proxied:false`) writes each `*.syscd.tech` record in Cloudflare as a CNAME to `syscd-gateway.tailcecc0.ts.net`; the Istio `tailscale` Gateway (`k8s/charts/support-cluster/templates/gateways.yaml`) terminates `*.syscd.tech` with the `syscd-tls` cert; and `syscd-gateway` is the Tailscale device the operator creates for the `istio-gateway` LoadBalancer Service (annotations `tailscale.com/expose: "true"`, `tailscale.com/hostname: "syscd-gateway"` in `k8s/applications/istio-gateway.yaml`). The stable-hostname annotation exists precisely so the external-dns target does not break across restarts.

The operator's proxy pod `ts-istio-gateway-wsmws-0` (a StatefulSet `ts-istio-gateway-wsmws` in namespace `tailscale`) lost its node authorization. On each boot tailscaled tried its persisted key, got `invalid key: ... not valid` / `You are logged out`, wrote a `reissue_authkey` marker into its state Secret, and waited up to 10 minutes for the operator to mint and hand back a fresh auth key. The operator never completed the handoff: its reconcile loop was stuck in a continuous `optimistic lock error ... Operation cannot be fulfilled on statefulsets "ts-istio-gateway-wsmws": the object has been modified`, flapping `TailscaleProxyReady True<->False` on every pass. tailscaled timed out after 10 minutes (`failed to get a reissued authkey: timeout waiting for auth key reissue after 10m0s`), SIGTERM'd, restarted, and repeated — 1070 restarts. The tailnet device `syscd-gateway` (100.109.0.112) went offline (last seen 2026-08-05) but its registration lingered, so `*.syscd.tech` DNS resolved to a dead device.

The operator had been restarted ~36h before the fix; its Connector proxy `ts-tailscale-1` re-provisioned cleanly then (0 restarts), but the pre-existing `ts-istio-gateway-wsmws` StatefulSet only got patched and kept losing the optimistic-lock race, so it never recovered on its own.

**Fix (mitigation).** Deleting the wedged StatefulSet + state Secret let the operator re-provision a healthy pod — but it registered as `syscd-gateway-1` because the dead `syscd-gateway` device still held the name. The durable path was to clear the tailnet of the orphaned device: delete both the stale `syscd-gateway` and the transient `syscd-gateway-1` devices (via the operator's OAuth client token against the Tailscale API; the user also removed the entry in the admin console), then clear the pod state Secret once more so the proxy did a fresh first-registration and reclaimed the free `syscd-gateway` name. The proxy then landed on `tp4` (a stable node), authorized cleanly (`machineAuthorized=true`, 0 restarts), and `*.syscd.tech` recovered.

Why mitigated, not resolved: service is durably back and the device is cleanly registered, but nothing was changed to prevent recurrence and the original trigger of the auth loss (2026-08-05) is not fully understood. The operator can wedge the same way again.

## Detection Gap (how we catch it next time)

- **What the user saw first:** "investigate why .tech is not working" — the user was the monitor; no page prompted the investigation.
- **External coverage exists but flap-masked.** `modules/gatus/config.yaml` on `digitalocean-gatus-01` (itself on the tailnet) already checks the `*.syscd.tech` path from outside with Discord alerts: `Httpbin Via Tailscale` (`https://httpbin.syscd.tech/status/200`) plus app checks `Alertmanager`, `Prometheus`, `Home Assistant`, `zigbee2mqtt`. But the gateway proxy flipped Ready roughly every ~10 min (each crash cycle briefly served traffic before dying), and the canary uses the default `failure-threshold: 6` (~60s) with `send-on-resolved: false`. A device that recovers for a few seconds every 10 minutes keeps resetting the failure counter and re-crossing the success threshold, so gatus oscillates between short failure bursts and resolves — at most intermittent pings that read as transient blips, never one sustained "gateway is down" page. gatus has no flap detection and keeps only ~50 in-memory samples (`storage` commented out), so the window can't be replayed. **gatus alone was not enough.**
- **No in-cluster alert exists, and the usual one would be dark.** There is no VMRule for the Tailscale ingress proxy. A KSM-derived alert (`kube_pod_status_ready==0` / restart-rate for `ts-istio-gateway-wsmws-0`) is **not** viable here: `vmks-kube-state-metrics` is crashlooping again (0/1, CrashLoopBackOff, confirmed this session) — the same chronic dark-`kube_*` failure class as `2026-07-05-hermes-rwx`, `2026-07-13-apiserver-crd-cache-oom`, `2026-08-10-longhorn` (tracked by `home-systems-5sw.1` / `home-systems-k5b`). Any KSM-based signal is inert.
- **Primary alert candidate — scrape the proxy's own tailscaled health (KSM-independent).** The operator can expose per-proxy tailscaled metrics via a `ProxyClass` (`spec.metrics.enable: true`); the `ProxyClass` CRD is already installed but no object exists and metrics are off (no `TS_ENABLE_METRICS` on the proxy). With it on, tailscaled exports a health gauge (e.g. `tailscaled_health_messages`, labeled by warning type such as `login-state` / `not-in-map-poll`) scraped directly off the proxy pod by vmagent (healthy, 2/2) — no KSM dependency. A VMRule that fires when the gateway proxy reports a health warning for >10m (`severity: critical` + `environment: production` to route to Discord) is the direct "tailscale gateway is broken / logged out" signal. This still rides the in-cluster VM stack, so gatus remains the external backstop; the two are layered, not either/or.
- **Why not auto-remediate the restart.** The operator was *already* auto-restarting the proxy (1070 times) — a plain restart never fixed it. The actual fix required deleting the orphaned tailnet device (a destructive Tailscale-API call) plus clearing state; a controller that auto-deletes tailnet devices is high-blast-radius and would mask the root cause. The durable fix is to stop the operator wedging and make it reap orphaned devices (see plan), so normal re-registration self-heals — not a bespoke device-deleting robot. A bounded, safe option is a liveness probe tied to tailscaled health so K8s restarts a *logged-out* proxy on its own, but note that alone would NOT have fixed this incident (the stale-device name conflict needed device deletion).
- **Fix path once detected:** runbook below.

## Mitigation (runbook — how to detect & fix this again)

**Symptom:** every `*.syscd.tech` host is unreachable while `*.syscd.live` is fine. `dig +short <host>.syscd.tech` still returns `syscd-gateway.tailcecc0.ts.net` → a 100.x IP (DNS is not the problem).

**Diagnose:**
```bash
# 1. Is the gateway proxy healthy? High restart count = the wedge.
kubectl get pods -n tailscale | grep istio-gateway
kubectl logs -n tailscale ts-istio-gateway-wsmws-0 --tail=40
#   look for: "invalid key ... not valid", "You are logged out",
#   "failed to get a reissued authkey: timeout ... after 10m0s"

# 2. Operator stuck reissuing? optimistic-lock churn = it can't complete the handoff.
kubectl logs -n tailscale deploy/operator --tail=100 | grep -iE "istio-gateway|optimistic lock|TailscaleProxyReady"

# 3. What devices exist in the tailnet, and is the live one named syscd-gateway?
CID=$(kubectl get secret -n tailscale tailscale-operator-oauth -o jsonpath='{.data.client_id}' | base64 -d)
CS=$(kubectl get secret -n tailscale tailscale-operator-oauth -o jsonpath='{.data.client_secret}' | base64 -d)
TOK=$(curl -sS -u "$CID:$CS" -d 'grant_type=client_credentials' https://api.tailscale.com/api/v2/oauth/token | python3 -c 'import sys,json;print(json.load(sys.stdin)["access_token"])')
curl -sS -H "Authorization: Bearer $TOK" https://api.tailscale.com/api/v2/tailnet/-/devices \
  | python3 -c 'import sys,json;[print(x["id"],x["name"],"lastSeen="+x.get("lastSeen","")) for x in json.load(sys.stdin)["devices"] if "syscd-gateway" in x["name"]]'
#   smoking gun: an offline "syscd-gateway" (holding the name) + a live "syscd-gateway-N" (the -N suffix proves the conflict).
```

**Fix (clear the wedge + free the stable hostname):**
1. Delete the stale/duplicate tailnet devices so the name `syscd-gateway` is free (admin console, or the API with the token minted above):
   ```bash
   curl -sS -X DELETE -H "Authorization: Bearer $TOK" https://api.tailscale.com/api/v2/device/<stale-syscd-gateway-id>
   curl -sS -X DELETE -H "Authorization: Bearer $TOK" https://api.tailscale.com/api/v2/device/<syscd-gateway-N-id>   # if a -N dup exists
   ```
2. Clear the proxy state and restart so it does a clean first-registration and reclaims `syscd-gateway`:
   ```bash
   kubectl delete secret ts-istio-gateway-wsmws-0 -n tailscale
   kubectl delete pod    ts-istio-gateway-wsmws-0 -n tailscale
   # (if the StatefulSet itself is wedged with optimistic-lock churn, delete it too; the operator recreates it from the Service annotation)
   ```
3. Verify: the pod is `Running 1/1` with 0 restarts and logs `Switching ipn state Starting -> Running (... nm=true)` and `machineAuthorized=true` (no more `node not found` / `invalid key`); the tailnet device is named exactly `syscd-gateway` again; `curl -sS -o /dev/null -w '%{http_code}' https://argocd.syscd.tech` returns 200.

**Gotcha:** if you delete a tailnet device while its pod is still running, the pod keeps a node key for a now-deleted node and logs `PollNetMap: initial fetch failed 404: node not found` — it will not re-register on its own. Always clear the pod's state Secret after deleting its device so it re-registers fresh. These `ts-*` StatefulSets/Secrets/devices are operator-managed and not in git — deleting them is safe; the operator recreates them.

## Dead Ends

- **First fix looked complete but wasn't.** Deleting the wedged StatefulSet + Secret produced a healthy pod (0 restarts, authorized) and it was tempting to call it done — but it had registered as `syscd-gateway-1`, and DNS still pointed at the dead `syscd-gateway`, so `.tech` was still down. The healthy-pod signal hid the name conflict.
- **`PollNetMap: initial fetch failed 404: node not found`** after deleting the device looked like a new failure — it was just the running pod holding a node key for the device that had been deleted out from under it. Clearing the state Secret and letting it re-register fixed it.
- **`home-systems-8f0` (open P0, 2026-07-03)** describes the same symptom ("Tailscale ingress down - ts-istio-gateway proxy stuck") but blames a flapping resource-starved `macarm01` VM. That was a real, different root cause; this time the healthy pod landed on `tp4` and macarm01 (currently `NodeStatusUnknown`) was not involved. Same symptom, different failure mode — do not assume it is the macarm01 problem again.
- **External DNS was an early suspect** but resolved correctly the whole time (`*.syscd.tech` → `syscd-gateway.tailcecc0.ts.net` → 100.x); the fault was entirely the device behind the name.

## Timeline

### 2026-08-11 (UTC)
- (prior, `2026-08-05T14:10`) tailnet device `syscd-gateway` (100.109.0.112) last seen; its registration goes stale but lingers. Proxy pod `ts-istio-gateway-wsmws-0` created ~this date.
- (ongoing ~2.5 days) `ts-istio-gateway-wsmws-0` crashlooping ~every 10 min (1070 restarts): `invalid key ... not valid` → `reissue_authkey` → `timeout waiting for auth key reissue after 10m0s` → SIGTERM → restart. Operator flapping `TailscaleProxyReady True<->False` with `optimistic lock error` on the StatefulSet. `*.syscd.tech` down.
- `~15:05` User: "investigate why .tech is not working, this is tailscale gateway."
- `~15:06` Traced the chain: DNS intact (`argocd.syscd.tech` → `syscd-gateway.tailcecc0.ts.net` → 100.109.0.112). Found the crashloop + auth-reissue timeout in the proxy logs and the operator optimistic-lock churn. Noted `ts-tailscale-1` healthy (36h) while the 6d-old gateway proxy is wedged.
- `~15:20` (approved) Deleted StatefulSet `ts-istio-gateway-wsmws` + Secret `ts-istio-gateway-wsmws-0`. Operator recreated a pod that authorized cleanly (`machineAuthorized=true`, 0 restarts) — but as `syscd-gateway-1` (100.120.230.104); the dead `syscd-gateway` still held the name, DNS still dead.
- `~15:23` Minted a token from the operator OAuth client; deleted tailnet devices `syscd-gateway` (5869224077286570) and `syscd-gateway-1` (1427772760879522), both HTTP 200; deleted the pod state Secret + pod to force clean re-registration.
- `~15:25` New pod hit `PollNetMap: initial fetch failed 404: node not found` (held a key for the just-deleted device). User also deleted the gateway entry in the Tailscale console. Cleared the state Secret + pod once more.
- `~15:27` Pod re-registered fresh, reclaimed `syscd-gateway.tailcecc0.ts.net` (now 100.106.9.124), on node `tp4`, `Running 1/1`, 0 restarts, `Switching ipn state -> Running (nm=true)`.
- `~15:28` Verified: `argocd.syscd.tech` → HTTP 200 (valid TLS), `grafana.syscd.tech` → HTTP 302. Proxy stable at 0 restarts. `*.syscd.tech` restored.
