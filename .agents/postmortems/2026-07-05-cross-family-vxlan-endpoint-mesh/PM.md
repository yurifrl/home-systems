---
date: 2026-07-05
status: closed
incident_status: resolved
sessions:
  - 019f32ed-561a-715d-b14f-3c22a2055172
components:
  - cilium
  - tailscale
  - tp1
  - dell01
  - tp4
  - pc01
  - macintel01
  - rpi01
symptoms:
  - cross-node pod-to-pod 100% packet loss (tp1 to guests)
  - hermes cross-site RWX mount hung
  - pod reachable by ICMP underlay but overlay dead
  - one node reaches a peer, another node does not (pair/node-specific)
failure_mode: cross-family-vxlan-endpoint-mesh
affected_urls: []
beads: [home-systems-9xk]
memories: [cilium-cross-family-vxlan-mesh-2026-07-05]
supersedes: []
related:
  - 2026-07-05-pc01-vxlan-tx-checksum-offload
  - 2026-07-05-hermes-rwx-sharemanager-cross-site
  - 2026-07-05-pc01-tailscale-flag-drift-crashloop
  - 2026-07-12-tailscale-cilium-endpoint-recursion
---

# Postmortem: cross-family VXLAN endpoint mesh (tp1↔guest return-path blackhole)

- **Severity/Impact:** Recurring, multi-subsystem. Pods on tp1 (incl. hermes) could not reach pods/services on the cross-site guest nodes (macintel01, rpi01) — 100% loss one-way. Prior detonations of the same root fault: the hermes RWX mount hang and (adjacent) the argocd 502 saga. Duration: latent for weeks; the tp1↔guest instance was live at least since the 2026-07-05 control-plane churn until fixed this session.
- **Root cause (one line):** `cross-family-vxlan-endpoint-mesh` — the cluster ran a **mixed** cilium tunnel-endpoint mesh (guests registered their Tailscale `100.x` IP as k8s InternalIP, core nodes registered their LAN `192.168.68.x` IP). Every core↔guest pair is then a cross-family pair: the core node sends VXLAN out `tailscale0` but the peer replies to the core node's LAN IP arriving on `end0`, and that send-one-interface / receive-another asymmetry is dropped at the kernel VXLAN decap on nodes whose datapath state was disturbed (tp1 broke; tp4, with identical config, did not).

## What Happened
The handoff framed the problem as "cross-site guests can't be full cilium mesh members." Reproduction disproved that framing: from a pod on **tp4**, both guest pods answered at 0% loss; from a pod on **tp1**, both were 100% loss — so the fault was **tp1-node-local**, not a guest limitation. Packet capture showed tp1's echo request reaching the guest, the guest's reply arriving well-formed on tp1's LAN NIC `end0`, but cilium's from-overlay never seeing the decapped reply and logging **zero BPF drops** — the packet died at tp1's kernel VXLAN decap. All three core nodes had identical routes to the guest endpoints, so it was not config drift but node-local datapath state, made fragile by the underlying architecture: a heterogeneous endpoint mesh. The durable fix was to homogenize it — every node registers its Tailscale IP as InternalIP (the macarm01 model), so every tunnel endpoint is a `100.x` address, every VXLAN rides `tailscale0` symmetrically, and no cross-family pair remains.

## Detection Gap (how we catch it next time)
- **What the user saw first:** workloads on tp1 (hermes) failing to reach cross-site pods/services; earlier the same fault surfaced as a hung Longhorn RWX mount and argocd 502. No monitoring fired for cross-node pod connectivity.
- **How we detect it before the user next time:** cilium's own health probe already exposes the signal (`cilium-health status` showed the affected peer at `Endpoints 0/1`) as metric `cilium_node_connectivity_status{type="endpoint"}` — but **nothing scrapes cilium metrics today** (no VMServiceScrape exists; the only networking VMRule is nic-offload-fix). Primary alert candidate: enable cilium agent metrics + scrape + a critical VMRule on endpoint connectivity == 0 for >10m (probe lag is ~3min). The existing gatus `icmp://100.x` node checks would NOT have caught this (they test the underlay, which was healthy; the fault was overlay-only) and only cover the 4 LAN nodes.
- **Fix path once detected:** the homogeneous-mesh migration (done this session) is the durable fix. For a future regression the runbook below localizes node vs guest in two pings and confirms the mechanism with one tcpdump + `cilium monitor`.

## Mitigation (runbook — how to detect & fix this again)
Durable fix already applied: all 7 nodes on Tailscale InternalIPs (`nodeIP.validSubnets: 100.64.0.0/10` in each `nostos/templates/<node>.yaml`), cilium `MTU: 1230` (`k8s/applications/cilium.yaml`) so encapsulated frames fit tailscale0's 1280 underlay. LAN nodes stay `--accept-routes`-free; hostDNS answers ClusterDNS node-locally — together these keep the 2026-06-05 DNS break from recurring.

To diagnose a suspected recurrence (a service flapping/hanging where its pod lands on one node):
1. **Localize node vs peer:** from a netshoot **overlay** pod (nodeName-pinned, NOT hostNetwork) on the suspect node, `ping`/`nc` a live pod on the target; repeat from a *different* node to the *same* target. If node-A fails but node-B succeeds → node-A-local fault (not the target).
2. **Confirm the mechanism:** hostNetwork netshoot on the target, `tcpdump -ni any 'udp port 8472 and host <suspect-TS-IP>'` — if the reply VXLAN leaves the target but `cilium monitor -v` on the suspect shows only the outbound request (never the inbound reply) and `cilium monitor --type drop` is empty → packet dies at kernel VXLAN decap.
3. **Verify endpoints are homogeneous:** `kubectl -n kube-system exec <any-cilium> -c cilium-agent -- cilium node list` — every `IPv4 Address` must be a `100.x`. A `192.168.68.x` endpoint = a node regressed to a LAN InternalIP; re-apply its nostos template (`nodeIP.validSubnets: 100.64.0.0/10`) and restart its cilium agent.
4. **Interim unblock (if migration state is intact):** `kubectl -n kube-system delete pod <cilium-agent-on-suspect>` rebuilds its datapath; if it doesn't hold, reboot the node.

Applying a nodeIP change: edit template → `go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml render <node>` (confirm certSANs include the node's TS IP) → `apply <node> --yes` (re-registers InternalIP live, no reboot needed) → delete that node's cilium pod so it re-derives its endpoint → verify `cilium node list`. dell01 (sole CP) keeps its `192.168.68.100` interface for LAN break-glass.

## Dead Ends
- **The handoff's whole framing — "cross-site guests can't be mesh members" — was wrong.** The guests were reachable from every healthy node; tp1 was the broken one. Chasing the guest-limitation theory would have led to fencing guests off (compute-only) instead of fixing the real fault.
- **Stale pod IP `10.244.7.101`** gave a false "total blackhole (ICMP + TCP both dead)" first reading — it was a dead/recreated pod IP. Re-testing against live IPs showed the true one-way-return signature. (Same red-herring class flagged in the pc01 offload PM.)
- **cilium-health `Endpoints 0/1` cache** initially showed all guests (incl. the healthy macarm01) as unreachable — the cache lags ~3min; a live ping is authoritative.
- **VXLAN TX checksum-offload (the pc01 fault)** was a tempting match, but tp1 also fails to bare-metal rpi01 (no virtio, no offload), so the common factor was tp1, not a guest NIC.
- **Routing-asymmetry-as-config-bug** theory: all three core nodes had identical `dev tailscale0` routes to the guest endpoints, so it wasn't a per-node route difference — it was node-local datapath state on top of the architectural mismatch.
- **The templates' own comments** ("never register the Tailscale IP / DNS-break 2026-06-05") were misleading: the real June break was `--accept-routes` importing the pod/Service CIDR, not TS-IP-as-nodeIP — proven by rpi01/macintel01 already running TS-IP-as-nodeIP with working DNS.

## Timeline
### 2026-07-05
- `~15:55` Handoff received: recurring cross-site fault framed as "guests can't be mesh members."
- `~15:57` Reproduced: pod-on-tp1 → macintel01 & rpi01 pods = 100% loss; pod-on-tp4 → same targets = 0% loss → fault localized to **tp1**, not the guests.
- `~15:58` tcpdump on macintel01: tp1's echo request arrives + pod replies; reply egresses toward tp1.
- `~16:03` tcpdump on tp1 `end0`: guest reply VXLAN arrives well-formed (all seqs, no checksum error).
- `~16:06` `cilium monitor -v` on tp1: only the outbound request seen, never the inbound reply; `--type drop` empty; rp_filter=0 → packet dies at kernel VXLAN decap. Root cause: mixed LAN/TS endpoint mesh → cross-family asymmetry.
- `~16:20` Plan written (`.agents/plans/all-tailscale-mesh-migration.md`); confirmed live risk cilium_vxlan MTU 1450 vs tailscale0 MTU 1280.
- `~16:40` Phase 0: cilium MTU 1450→1230 committed (`ecae6d02`), cilium agents rolled node-by-node; verified PMTU boundary + DNS + no drops.
- `~17:00` Phase 1 blocked: pc01 found off-tailnet (ext-tailscale crashloop — expired auth key + stale `--accept-routes` in running config); user restored it (tailscale0=100.101.182.40).
- `~17:15` Phase 1: pc01 → InternalIP 100.101.182.40; 0% loss all pairs.
- `~17:25` Phase 2: tp4 → 100.112.10.120; 0% loss; perf ref ~236 Mbit/s.
- `~17:35` Phase 3: tp1 → 100.90.9.55; **tp1→guests 100%→0% loss** (reproduced fault fixed); hermes healthy.
- `~17:45` Phase 4: dell01 (sole CP) → 100.82.148.37; apiserver healthz ok throughout, cert carries TS SAN.
- `~17:50` Mesh fully homogeneous (all `cilium node list` endpoints `100.x`); full matrix 0% loss; DNS ok; argocd.syscd.live 302. Templates committed+pushed (`23c9313a`).
