# Mecatl — remote agent worker on the cluster

Deploy Stacklok [Mecatl](https://mecatl.dev/) `mecak8s` as a new Argo Application: a remote agent worker you connect to from the laptop on demand, with real OIDC login, session state in Redis, and a persistent workspace PVC. Exposed tailnet-only at `mecatl.syscd.tech`.

## What you're getting (mental model)

- **The pod is the pool; sessions are the workers.** One small always-running agent pod hosts as many sessions as you want. Run `mecatui login` + `mecatui connect mecatl.syscd.tech` → a fresh agent session spawns instantly. Close the laptop → the session keeps its state in Redis; reconnect later and it's still there.
- **Secure login** = OIDC via Dex (single user, password from 1Password). No token is accepted without a valid Dex-issued JWT.
- **Workspace** = a Longhorn PVC mounted at `/workspace` in the pod. The agent's file tools + shell are rooted there; you `git clone` projects into it from inside a session. Backed up weekly by Longhorn.

## Context (facts this plan is built on)

**Mecatl `mecak8s`** (chart `oci://ghcr.io/stacklok/mecatl/charts/mecak8s`, latest release **v0.0.38**, 2026-09-15, signed image):
- Stateless agent pods; session snapshots + durable event log in **Redis**; single-writer per session via Kubernetes Leases; pod death → session resumes from Redis (lease TTL 30s on hard kill).
- gRPC on 8080, HTTP/SSE + health on 8081, ClusterIP Service. `readyz` pings Redis.
- For a real LLM provider the chart **requires** all three:
  1. **TLS-verified Redis** (plaintext refused at startup; mTLS client certs unsupported — server-side TLS only, `tls-auth-clients no`).
  2. **OIDC caller identity**: `issuer` compared byte-exact, `audience` required, JWKS via issuer discovery (fail-closed at startup), isolates sessions/schedules/memory by `(issuer, subject)`.
  3. **TLS posture**: in-pod TLS **or** `security.tlsTerminatedUpstream: true` (operator gateway terminates TLS).
- `oidc.resource` + `oidc.clientID` + `scopes` advertise login metadata so `mecatui login ADDRESS` works out of the box.
- `telemetry.installationID` must be a pinned UUID for GitOps (offline render can't `lookup`).
- `workspace: /abs/path` + a self-mounted volume turns it into a server-assigned filesystem deployment (client can't pick another root). RWO PVC → 1 replica (RWX would be needed for 2).
- In-cluster plain-HTTP OpenAI-compatible gateways (litellm) are **refused** as `--openai-base-url` (HTTPS-only outside loopback) → provider key goes directly to the pod via ExternalSecrets.

**Cluster facts this plan reuses:**
- Istio `tailscale` Gateway serves `*.syscd.tech` on 443 (TLS SIMPLE, cert-manager Let's Encrypt wildcard `syscd-tls` via Cloudflare DNS-01, HTTP→HTTPS redirect); external-dns points it at `syscd-gateway.tailcecc0.ts.net`. Laptop reaches it anywhere via tailnet.
- `*.syscd.dev` resolves via LAN bind9 (192.168.68.200) → gateway LB 192.168.68.201; the `dev` Gateway serves it with the same wildcard cert. **This is the domain cluster pods can resolve** → use for the Dex issuer URL.
- Support chart provides: `externalSecrets`, `virtualServices` (binds dev+cloudflare+tailscale gateways; `domains: tech/xyz/live` flags), `statefulSets`, `services`, `persistentVolumeClaims`, `longhornBackups`, `networkPolicies`, `configMaps`. It has **no** Certificate template → plan adds one (step 2).
- Shared cluster Redis (`redis.redis.svc:6379`) is plaintext/no-auth → **not usable** for mecak8s; we run a dedicated TLS Redis in the `mecatl` namespace (its README says new consumers may add their own; mecak8s can't share it anyway).
- Namespace joins the ambient mesh via `managedNamespaceMetadata: istio.io/dataplane-mode: ambient` (same pattern as cloudflare-tunnel) → the bearer token inside the mesh (gateway→pod, pod→redis) rides mTLS.
- ArgoCD multi-source upstream-chart pattern: `repoURL` + `chart:` + `targetRevision:` + `releaseName:` (as in argocd/istio-gateway/vmks apps). OCI registry source: `repoURL: oci://ghcr.io/stacklok/mecatl/charts` + `chart: mecak8s` (verify ArgoCD 3.x accepts `oci://`; fallback in step 6).
- 1Password → ExternalSecrets is the repo's secret path; the private values repo (`home-systems-values`) only for bulky/private values — inline `valuesObject` is fine here (redis.yaml precedent), except the OpenRouter key which lives in 1Password.

**Decisions (and why):**
| Decision | Why |
|---|---|
| Dex for OIDC (your call) | Tiny CNCF OIDC provider, one static password, no DB; reusable later for Grafana/ArgoCD SSO |
| Tailnet-only exposure (`mecatl.syscd.tech`, `domains: tech` only) | Agent has shell + workspace — never publish it on `.syscd.live` |
| 1 replica | RWO workspace PVC; home lab; sessions survive pod death via Redis + Leases; chart explicitly supports 1 |
| PVC workspace over `redis.filesystem` virtual FS | PVC gives real Shell + git; redis FS mode is file-lite (no shell, no git) |
| Edge-TLS posture (`tlsTerminatedUpstream: true`) | Gateway already terminates real LE certs; avoids in-pod cert plumbing |
| No SLOs from support chart | Its SLO template is blackbox-HTTPS-probe based; a gRPC endpoint would probe red forever |
| No MCP broker / mcp.servers | Broker forces single-replica + Recreate; no remote MCP upstreams yet — add later |
| New Argo app `mecatl`, separate new app `dex` | Repo convention: one Application per service; Dex is platform infra others can reuse |

## Steps

### 1. Pre-flight (read-only checks)
- Cluster reachable (`kubectl get nodes`; switch context per repo rules if needed).
- **Pod-side DNS**: run a throwaway debug pod, `nslookup dex.syscd.dev` → expect 192.168.68.201. If it fails, note it — step 6 fallback (CoreDNS rewrite or ServiceEntry) becomes mandatory.
- **Laptop-side DNS**: `dig dex.syscd.dev` and `dig mecatl.syscd.tech` from the Mac (home LAN resolves .dev via bind9; .tech via tailnet MagicDNS).
- `uuidgen` → save as the pinned `telemetry.installationID`.

### 2. Support chart: add generic `certificates` option
- New `k8s/charts/support/templates/certificate.yaml` + `values.yaml` docs.
- Shape mirrors existing resource lists:
  ```yaml
  certificates: []
  # - name: redis-tls
  #   namespace: mecatl          # optional, defaults to release ns
  #   selfSignedCA: true         # renders Issuer(selfsigned) → CA Certificate(isCA) → CA Issuer
  #   dnsNames: [redis.mecatl.svc.cluster.local, redis.mecatl.svc, redis.mecatl]
  #   duration: 2160h
  #   renewBefore: 360h
  ```
- Purely additive: `[]` default → zero render change for every other app. One Certificate per entry referencing the CA issuer created by the same entry (or a plain `issuerRef` passthrough for reusing `letsencrypt` later).
- Secrets it creates are consumed by both redis (server cert) and mecak8s (`caKey` → `ca.crt`).

### 3. New Application `k8s/applications/dex.yaml` (namespace `dex`)
- Source 1: `https://charts.dexidp.io`, `chart: dex`, pinned `targetRevision` (current stable).
  - `config.issuer: https://dex.syscd.dev`
  - `config.staticClients`: `mecatui` — public (no secret), PKCE S256, redirectURIs for mecatui's loopback flow (**verify exact redirect URI from `mecatui login` output on first attempt**; start with `http://localhost:[127.0.0.1 ports]/callback` patterns dex allows via `http://localhost:*` — dex has no wildcard ports, so expect one iteration to nail the port).
  - `config.enablePasswordDB: true`; static password injected via env expansion from a Secret (chart `secretEnv`/env substitution), value = bcrypt hash.
  - `config.web.http` only (TLS at gateway); ingress none (VS instead).
- Source 2: support chart —
  - `externalSecrets: [mecatl-dex-password]` (1Password item holding the plaintext password; bcrypt hash generated from it — either pre-compute hash into 1Password **or** store the hash as the item; pick: store **bcrypt hash** as item content so the plaintext never lives in the cluster).
  - `virtualServices: dex` → `dex.dex.svc:5556`, `domains: {dev: true, tech: true, xyz: false, live: false}` (dev for pods, tech for off-LAN laptop).
- Dex has no state → no PVC. Label namespace ambient (mesh consistency).

### 4. New Application `k8s/applications/mecatl.yaml` (namespace `mecatl`, ambient-labeled)
- Source 1 — OCI chart:
  ```yaml
  repoURL: oci://ghcr.io/stacklok/mecatl/charts
  chart: mecak8s
  targetRevision: "0.0.38"
  releaseName: mecatl
  helm.valuesObject:
    replicaCount: 1
    workspace: /workspace
    extraVolumes:
      - name: workspace
        persistentVolumeClaim: { claimName: mecatl-workspace }
    extraVolumeMounts:
      - { name: workspace, mountPath: /workspace }
    redis:
      endpoint: redis.mecatl.svc.cluster.local:6379
      credentialsSecret: redis-tls
      caKey: ca.crt            # cert-manager secret key
    security:
      tlsTerminatedUpstream: true
    oidc:
      enabled: true
      issuer: https://dex.syscd.dev          # byte-exact; verified in step 1
      audience: mecatui
      resource: https://mecatl.syscd.tech    # RFC 9728 → powers mecatui login
      clientID: mecatui
      scopes: [openid, profile, offline_access]
    telemetry:
      installationID: <pinned-uuid>
    defaultProvider: openrouter
    model: <default model id, e.g. a mid-cost Claude/GPT>
    extraEnv:
      - name: OPENROUTER_API_KEY
        valueFrom:
          secretKeyRef: { name: mecatl-openrouter, key: OPENROUTER_API_KEY }
  ```
- Source 2 — support chart:
  - `statefulSets`: `redis` — `redis:7-alpine`, args: `--port 0 --tls-port 6379 --tls-cert-file /tls/tls.crt --tls-key-file /tls/tls.key --tls-ca-cert-file /tls/ca.crt --tls-auth-clients no --appendonly yes`; secret volume `redis-tls` at `/tls`; volumeClaimTemplate 5Gi `longhorn-ha`; modest resources (like shared redis).
  - `services`: `redis:6379→6379`.
  - `certificates`: `redis-tls` (selfSignedCA, dnsNames as in step 2).
  - `persistentVolumeClaims`: `mecatl-workspace` — 10Gi, `longhorn-ha`, RWO.
  - `longhornBackups`: `[{pvc: mecatl-workspace}]` (weekly default, retain 30).
  - `externalSecrets`: `mecatl-openrouter` (1Password item with the OpenRouter key; field name `OPENROUTER_API_KEY`).
  - `virtualServices`: `mecatl` — service `mecatl-mecak8s.mecatl:8080`, `domains: {tech: true, xyz: false, live: false}`, custom `http` routes:
    1. `match: uri prefix: /.well-known/` → destination `...:8081` (login metadata discovery)
    2. fallback route → `...:8080` (gRPC; h2c upstream)
    - `/healthz`, `/readyz`, `/drain` stay unrouted — docs explicitly forbid publishing them.
  - `networkPolicies` (cheap hardening): deny redis ingress except from mecak8s pods (label-based); `policyTypes: [Ingress]`.
- Argo `syncOptions` per repo convention (`CreateNamespace=true`, automated prune/selfHeal); `managedNamespaceMetadata.labels: istio.io/dataplane-mode: ambient`.

### 5. Land it
- Commit + push (repo rule: done = synced, not just pushed).
- Watch `kubectl get application mecatl dex -n argocd`; on sync error read `operationState.message` + conditions, iterate (10s–60s, then force sync if needed).
- Health gates: both pods Running/Ready; `readyz` 200 (proves Redis TLS verified); mecak8s logs show OIDC validator initialized (JWKS fetched from Dex = proves pod→Dex TLS path).

### 6. Fallbacks (only if their trigger fires)
- **Pods can't resolve `dex.syscd.dev`** → add a CoreDNS rewrite/forward or a `ServiceEntry` (`dex.syscd.dev` → `dex.dex.svc.cluster.local`) — issuer string must stay byte-identical.
- **ArgoCD rejects the `oci://` source** → vendor the chart under `k8s/charts/mecatl-oss` (pinned copy, upgrade = re-vendor + bump), same values.
- **`mecatui login` redirect mismatch** → copy the exact redirect URI from the error, update the Dex staticClient, resync.
- **Provider wiring misbehaves** → temporary `mockProvider: true` sync to isolate infra from LLM config, then flip back.

### 7. Laptop / first use
- `brew install stacklok/tap/mecatl`
- `mecatui login mecatl.syscd.tech` → browser opens Dex → login (1Password) → back in TUI.
- `mecatui connect mecatl.syscd.tech --tls` → prompt something, watch tool cards (ctrl+t).
- Have the agent `git clone` a real repo into `/workspace` and inspect it.
- Kill the pod (`kubectl delete pod`) mid-session → wait → reconnect → conversation intact (Redis + lease recovery).

## Verification

| Check | How |
|---|---|
| ArgoCD synced + healthy | `kubectl get applications -n argocd` |
| Redis TLS verified | pod `readyz` 200; plain `redis-cli` fails, `--tls --cacert` succeeds from a debug pod |
| Auth enforced | unauthenticated gRPC/HTTP request → 401 (port-forward test); Dex down → 503 after JWKS staleness |
| Secure login end-to-end | `mecatui login` browser flow → session works; owner stamp visible via `GET /v1/sessions` |
| Worker usable | prompt → model responds via OpenRouter (usage visible in OpenRouter dashboard) |
| Workspace | agent lists/clones files in `/workspace`; Longhorn recurring job + label Job exist |
| Session durability | delete pod mid-session → reconnect → history intact |
| Exposure | `mecatl.syscd.tech` reachable only over tailnet; `curl https://mecatl.syscd.tech/healthz` from laptop → unrouted (404/deny), not 200 |

## Out of scope
- Public `.syscd.live` exposure, SLOs (probe model doesn't fit gRPC), MCP broker/servers, multi-user tenancy, Keycloak, litellm as LLM gateway (HTTPS-only rule), image digest pinning (tag pinned; add digest later if desired), embedding the engine / mecatequi CI mode.
