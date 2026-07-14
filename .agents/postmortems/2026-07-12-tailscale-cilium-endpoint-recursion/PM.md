---
date: 2026-07-12
status: draft
incident_status: mitigated
sessions:
  - 019f57a3-840f-7b9c-a70a-5f92597080c7
components:
  - argocd
  - cilium
  - tailscale
  - macarm01
  - macintel01
symptoms:
  - ArgoCD UI unable to load application data
  - repo-server gRPC server-preface timeout
  - cross-node TLS handshake timeout
  - Cilium reports one unreachable endpoint peer
failure_mode: tailscale-cilium-endpoint-recursion
affected_urls:
  - https://argocd.syscd.live
beads: []
memories: []
supersedes: []
related:
  - 2026-07-05-cross-family-vxlan-endpoint-mesh
---

# Postmortem: Tailscale selected Cilium overlay endpoints and broke ArgoCD repo access

- **Severity/Impact:** ArgoCD's web UI loaded, but application details, revision metadata, refreshes, and syncs that required `argocd-repo-server` failed from `argocd-server`. The failure was intermittent and pair-specific between `macarm01` and `macintel01`; same-node Argo components continued working.
- **Root cause (one line):** `tailscale-cilium-endpoint-recursion` — Tailscale selected each peer's `cilium_host` address as its direct WireGuard transport endpoint while Cilium VXLAN itself used the peer's Tailscale address as its underlay, creating a circular dependency.

## What Happened
`argocd-server` ran on `macarm01` (`10.244.5.135`) while the sole `argocd-repo-server` endpoint ran on `macintel01` (`10.244.7.215`). The Service and Cilium load-balancer maps were correct, the repo pod was Ready with zero restarts, and the repo-server NetworkPolicy explicitly allowed `argocd-server`. A controlled TLS handshake succeeded from the same-node application controller but timed out from `argocd-server`. Source-side Cilium monitoring showed the SYN leaving `macarm01`; a simultaneous destination-side monitor showed no matching packet arriving at `macintel01`.

Tailscale logs supplied the missing boundary evidence. `macarm01` repeatedly selected `10.244.7.219:55248` for `macintel01`, while `macintel01` selected `10.244.5.249:37487` for `macarm01`. Those addresses were confirmed as the peers' `cilium_host` interfaces. The resulting path was recursive: Cilium VXLAN targeted a Tailscale node address, while Tailscale tried to reach that peer through the Cilium overlay carried by the same VXLAN tunnel. Small discovery traffic occasionally succeeded, causing Tailscale to reselect the invalid candidate and making the outage flap.

The mitigation set `TS_DEBUG_OMIT_LOCAL_ADDRS=true` on the two affected Tailscale extension services. Tailscale 1.98.2 implements this knob by omitting `EndpointLocal` candidates while retaining STUN and DERP endpoints. This restored the pair, but the knob is explicitly a debug/test setting and five other node templates still advertise local addresses, so the incident is mitigated rather than durably resolved.

## Detection Gap (how we catch it next time)
- **What the user saw first:** ArgoCD displayed `Unable to load data` with `error reading server preface` / `use of closed network connection`. The external Gatus check stayed green because it validates only the Argo login HTML, not an authenticated application-details call that traverses repo-server gRPC.
- **How we detect it before the user next time:** Cilium already exposed the failing pair as one unreachable endpoint peer. `CiliumNodeConnectivityDown` did not page because its expression tolerates one unreachable peer (`>1`) for an obsolete macarm01 baseline. A critical `>0 for 15m` endpoint-connectivity alert is the primary signal; it requires intervention and covers this symptom without coupling detection to Argo placement.
- **Fix path once detected:** identify the failing node pair with Cilium health, compare a direct TLS handshake from same-node and cross-node clients, then inspect Tailscale logs for `now using 10.244.*`. Apply the local-address omission only as an emergency mitigation; durable resolution requires eliminating local-overlay endpoint advertisement cluster-wide with a supported mechanism or removing the Cilium-over-Tailscale dependency cycle.

## Mitigation (runbook — how to detect & fix this again)
1. Identify the Argo source, Service, and endpoint placement:
   ```bash
   kubectl -n argocd get pods -o wide
   kubectl -n argocd get svc argocd-repo-server
   kubectl -n argocd get endpointslice -l kubernetes.io/service-name=argocd-repo-server -o wide
   ```
2. Test TLS from the actual clients. A cross-node timeout with a same-node success localizes the failure below Argo and above the destination process:
   ```bash
   kubectl -n argocd exec <argocd-server-pod> -- sh -c \
     'timeout 12 openssl s_client -connect <repo-pod-ip>:8081 -servername argocd-repo-server -brief </dev/null'
   kubectl -n argocd exec argocd-application-controller-0 -- sh -c \
     'timeout 12 openssl s_client -connect <repo-pod-ip>:8081 -servername argocd-repo-server -brief </dev/null'
   ```
3. Confirm the packet boundary with endpoint-filtered Cilium monitors. The failing signature is a SYN leaving the source endpoint and no corresponding source IP on the destination endpoint:
   ```bash
   kubectl -n kube-system exec <source-cilium-pod> -c cilium-agent -- \
     cilium monitor -vv --from <source-endpoint-id> --type trace --type drop --type policy-verdict
   kubectl -n kube-system exec <destination-cilium-pod> -c cilium-agent -- \
     cilium monitor -vv --to <destination-endpoint-id> --type trace --type drop --type policy-verdict
   ```
4. Confirm endpoint recursion. Match every selected `10.244.x` address to `cilium_host`:
   ```bash
   talosctl -n <node-ts-ip> -e <node-ts-ip> logs ext-tailscale --tail 500 | grep 'now using 10.244'
   kubectl -n kube-system exec <cilium-pod> -c cilium-agent -- ip -4 addr show cilium_host
   ```
5. Emergency mitigation for Tailscale 1.98.2: add this to the affected node's `ExtensionServiceConfig` environment, render, and apply one node at a time:
   ```yaml
   - TS_DEBUG_OMIT_LOCAL_ADDRS=true
   ```
   ```bash
   go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml render <node>
   go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml apply <node> --yes
   ```
6. Verify recovery with fresh evidence: both nodes Ready, Tailscale advertises STUN/port-mapped endpoints rather than `10.244.x`, cross-node TLS completes through both the Service and pod IP, and no new Argo repo-server transport errors appear after the apply timestamp.

## Dead Ends
- The initial diagnosis called this generic cross-node networking and associated it with the prior VXLAN incident before proving the packet boundary. That was too broad: the old incident involved mixed LAN/Tailscale Cilium tunnel endpoints; this incident used homogeneous Tailscale node endpoints but recursive Tailscale peer candidates.
- The OOM check found no repo-server or Argo server restart. Restart history was unrelated to this transport failure.
- The repo-server NetworkPolicy looked suspicious because it is ingress-restrictive, but it explicitly allowed the Argo server labels. A test from Home Assistant was invalid because that pod was intentionally outside the allowlist.
- Historical Hubble queries returned no matching flows because the local ring buffer was saturated; controlled simultaneous Cilium monitors produced the useful source/destination boundary evidence.
- The Cilium image lacked `tcpdump`, so the first packet-capture attempt failed before collecting evidence.
- `cilium-health status` initially displayed stale pre-mitigation samples. Direct TLS and `/hello` checks were used for fresh verification instead.
- Restarting Cilium alone was considered from the previous runbook, but it could not prevent Tailscale from selecting the same recursive endpoint again.

## Timeline
### 2026-07-12 (UTC)
- `18:41` Argo server logged the first captured `error reading server preface` from `10.244.5.135` to repo Service `10.101.163.186:8081`.
- `18:43` User reported the ArgoCD UI `Unable to load data` error.
- `18:44` Argo logs showed repeated 22–27 second TCP dial and TLS handshake timeouts to repo-server.
- `19:34` Cilium health on `macarm01` identified `macintel01` as the only unreachable endpoint peer; the repo pod remained Ready with zero restarts.
- `19:38` Controlled OpenSSL tests timed out from `argocd-server` on `macarm01` but succeeded from `argocd-application-controller` on `macintel01`.
- `19:38` Source Cilium monitor captured the SYN leaving for `10.244.7.215:8081`; destination monitor captured normal same-node repo traffic but no packet from `10.244.5.135`.
- `19:39` Tailscale logs showed both peers repeatedly selecting the other's `cilium_host` address and timing out host TCP connections in both directions.
- `19:40` Upstream research confirmed there is no supported selective interface exclusion; Tailscale 1.98.2 does implement the test-only `TS_DEBUG_OMIT_LOCAL_ADDRS` knob.
- `20:01` Rendered and applied the knob to `macarm01`; the node returned Ready and advertised only a STUN endpoint.
- `20:02` Rendered and applied the knob to `macintel01`; it advertised STUN/port-mapped endpoints rather than `cilium_host`.
- `20:02` Cross-node TLS succeeded from `argocd-server` through repo Service `10.101.163.186:8081` and direct pod `10.244.7.215:8081`; both Argo pods were Ready with zero restarts and no new repo transport errors appeared.
