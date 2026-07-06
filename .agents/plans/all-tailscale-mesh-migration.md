# Plan: Homogenize the cilium mesh onto Tailscale endpoints (fix the recurring cross-node fault line)

Beads: `home-systems-h27`
Memory: `tp1-crossnode-return-vxlan-blackhole-2026-07-05`
Date: 2026-07-05

## 1. Problem (reproduced, root-caused)

Cilium derives each node's VXLAN tunnel endpoint from its k8s **InternalIP**. The cluster
currently runs a **mixed-endpoint mesh**:

| Node | InternalIP | Endpoint family | `--accept-routes` |
|------|-----------|-----------------|-------------------|
| dell01 (sole CP) | 192.168.68.100 | LAN | no |
| tp1, tp4 | 192.168.68.107/114 | LAN | no |
| pc01 (talos-pc01) | 192.168.68.104 | LAN | no |
| macarm01 | 100.91.131.67 | **Tailscale** | no (auto-picked TS IP) |
| macintel01 | 100.118.143.56 | **Tailscale** | yes |
| rpi01 | 100.102.238.43 | **Tailscale** | yes |

Every LAN-node ↔ TS-node pair is a **cross-family pair**: the LAN node sends VXLAN out
`tailscale0` (route to `100.x`) but the peer replies to the LAN node's LAN IP, arriving on
`end0`. That send-one-interface / receive-another asymmetry is dropped intermittently at the
**kernel VXLAN decap** (proven: reply arrives well-formed on `end0`, cilium never sees it,
zero BPF drops). It breaks per-node under control-plane churn — tp1 is broken now, tp4 is
fine with identical config — which is why a cilium restart "fixes" it and it recurs.

Reproduction (2026-07-05): pod-on-tp1 → macintel01/rpi01 pods = 100% loss; pod-on-**tp4** →
same targets = 0% loss. Fault is tp1-local, not a guest limitation.

## 2. Target architecture

Every node uses its **Tailscale IP as InternalIP** (the macarm01 model, cluster-wide). Then:
- every cilium tunnel endpoint is a `100.64.0.0/10` address,
- every VXLAN packet rides `tailscale0` symmetrically (send and receive on the same iface),
- there are **zero cross-family pairs left to corrupt**.

This is the *original* design intent — `k8s/applications/cilium.yaml` still says
`routingMode: tunnel ... nodes ride 100.x`. The LAN-IP pinning was a later mitigation for the
June DNS break; §4 shows that break's real cause is separately avoidable.

## 3. Validation of current state (what is already done)

- Guests (macintel01, rpi01) and macarm01 **already** register TS-IP-as-InternalIP. No change.
- KubePrism (`k8sServiceHost: localhost:7445`) already set in cilium — agents reach the
  apiserver node-locally, so InternalIP changes do **not** flap cilium against the CP VIP.
- `bpf.masquerade: false` + `hostDNS.forwardKubeDNSToHost: true` already set on every node —
  pod→ClusterDNS is answered node-locally by Talos hostDNS, never masqueraded onto the tailnet.
- etcd already advertises over `100.64.0.0/10` (dell01), and dell01 is the **sole** CP, so
  there is no cross-node etcd peering to destabilize.
- **Not yet done:** dell01, tp1, tp4, pc01 still pin `nodeIP.validSubnets: 192.168.68.0/24`.

## 4. Why the June 2026 DNS break does NOT gate this (risk deconstruction)

`.agents/contexts/remote pi.md` documents the break as **three layered failures**, only one of
which is TS-IP-as-nodeIP:

1. **PRIMARY killer:** `--accept-routes` on dell01 imported the **pod CIDR** `10.244.0.0/16`
   from advertised tailnet routes → hijacked pod return traffic → CoreDNS crashloop → cluster
   DNS down.
2. **Compounder:** kubelet picked TS IP as InternalIP → kube-proxy DNAT'd Service-VIP traffic
   to the TS IP → Tailscale netfilter dropped **pod-sourced** (non-TS-authorized) packets.
3. istio chart drift (unrelated).

Both #1 and #2 required the tailnet to be routing **cluster-internal CIDRs** and/or pods to
egress the tailnet SNAT'd. The current mitigations already neutralize them, and this migration
is designed to keep them neutralized:

- **We do NOT enable `--accept-routes` on the LAN nodes.** Reaching another node's `100.x` IP
  is native tailnet routing (CGNAT), which needs no route import. So the #1 mechanism (importing
  pod/Service CIDR) is never triggered by this change.
- **hostDNS answers ClusterDNS locally**, so pod→10.96.0.10 is not SNAT'd onto the tailnet →
  the #2 DNS path does not fire (empirically: rpi01/macintel01 run TS-IP-as-nodeIP with working
  DNS today).
- The stale `never the Tailscale IP` comments in tp1/rpi01/talos-pc01 templates predate the
  hostDNS mitigation and are contradicted by the live guests; they will be corrected.

## 5. Risks (ranked) and mitigations

- **R1 — MTU fragmentation (HIGH, confirmed live).** `cilium_vxlan` MTU = **1450**, but
  `tailscale0` MTU = **1280**. Today LAN↔LAN VXLAN rides `end0` (1500) so 1450 fits. After
  migration LAN↔LAN VXLAN rides `tailscale0` (1280) → every packet > ~1230 B payload
  fragments or drops. **Mitigation: lower cilium `MTU` to `1230` BEFORE migrating any LAN
  node** (1280 − 50 VXLAN overhead). This is a global cilium value; roll it first and verify
  large-packet pod traffic still flows on the existing LAN mesh. This is the single most
  likely thing to silently half-break the cluster.
- **R2 — sole control-plane exposure (HIGH).** dell01 is the only CP. Changing its InternalIP
  rewrites how apiserver advertises. **Mitigations:** add dell01's TS IP `100.82.148.37` to
  BOTH `machine.certSANs` and `cluster.apiServer.certSANs` in the same render; migrate dell01
  **LAST**, only after every worker is proven on TS endpoints; use `nostos apply dell01
  --mode=no-reboot` where possible; keep LAN access as the break-glass path (dell01 keeps its
  `192.168.68.100/24` interface address regardless of InternalIP).
- **R3 — accept-routes temptation (HIGH if mishandled).** If anyone enables `--accept-routes`
  on a LAN node AND the tailnet approves a cluster CIDR, that reproduces the June outage.
  **Mitigation: LAN nodes stay `--accept-routes`-free; do not advertise `10.244.0.0/16` or
  `10.96.0.0/12` from any node; verify tailnet ACL route-approvals before starting.**
- **R4 — performance (MEDIUM).** All inter-node pod traffic becomes WireGuard-encrypted at
  1230 MTU. Tailscale normally negotiates a **direct** LAN path between co-LAN peers (not
  DERP), so latency stays near-LAN, but encryption CPU + smaller MTU reduce throughput.
  **Mitigation: measure on the first migrated worker (tp4); confirm `tailscale status` shows
  `direct` (not `relay`) between LAN peers; accept or abort based on numbers.**
- **R5 — Longhorn / RWX churn during rollouts (MEDIUM).** Each node migration reboots/renetworks
  a node; Longhorn replicas & RWX share-managers there detach. **Mitigation: migrate one node at
  a time, wait for Longhorn `nodes.longhorn.io` Ready + volumes `attached` before the next; do
  NOT cordon (per `never-cordon-without-approval`) without asking.**
- **R6 — istio-cni ambient on TS-endpoint nodes (MEDIUM).** Memory `cross-site-node-mesh-limitation`
  notes istio-cni must use KubePrism on these nodes or sandbox creation hangs. Already set
  cluster-wide, but **verify a fresh pod sandbox comes up on each migrated node** before moving on.
- **R7 — tp1 stale state won't self-clear (LOW).** tp1 already carries the corrupted datapath.
  Its migration reboot rebuilds it, so the migration doubles as the tp1 fix. If tp1 must be
  unblocked before the full migration, a cilium-agent restart on tp1 is the interim (needs
  approval; disruptive to hermes).

## 6. Migration plan (phased, one node at a time)

All template edits land in `nostos/templates/*.yaml`, are committed to git, and applied with
`nostos render <node>` + `nostos apply <node>`. No `kubectl apply` to the cluster.

### Phase 0 — MTU (no InternalIP change yet)
- Edit `k8s/applications/cilium.yaml`: `MTU: 1450` → `1230`. Commit; let ArgoCD sync; cilium
  DaemonSet rolls. Run the **Phase 0 test suite** on the still-LAN mesh. This de-risks R1
  independently of any node change.

### Phase 1 — pc01 (least-critical worker; a virtio VM already flaky)
- `talos-pc01.yaml`: `nodeIP.validSubnets: [100.64.0.0/10]`; add `100.101.182.40` to
  `machine.certSANs`; leave tailscale block unchanged (no accept-routes). Render, apply.
- Run **per-node test suite**.

### Phase 2 — tp4 (healthy worker; the perf/throughput reference)
- `tp4.yaml`: same nodeIP change; add tp4 TS IP to certSANs. Render, apply.
- Run **per-node test suite** + **R4 perf probe**.

### Phase 3 — tp1 (the currently-broken node; migration = its fix)
- `tp1.yaml`: same nodeIP change; add `100.90.9.55` to certSANs; delete the stale
  "never the Tailscale IP" comment. Render, apply (reboots → rebuilds datapath).
- Run **per-node test suite**, specifically re-run the original repro (pod-on-tp1 → guest pods
  must now be 0% loss).

### Phase 4 — dell01 (sole CP, LAST)
- `dell01.yaml`: `nodeIP.validSubnets: [100.64.0.0/10]`; add `100.82.148.37` to BOTH
  `machine.certSANs` and `cluster.apiServer.certSANs`. Render, apply `--mode=no-reboot` first;
  full reboot only if InternalIP does not take.
- Run **per-node test suite** + **control-plane test suite**.

### Phase 5 — cleanup
- Optionally set macarm01 explicit `nodeIP.validSubnets: [100.64.0.0/10]` (currently relies on
  auto-pick) so it's declarative, not incidental.
- Update memories `cross-site-node-mesh-limitation`, `macintel01-roaming-dhcp` to reflect the
  homogeneous mesh; close `home-systems-h27`.

## 7. Test suites

Reusable helpers: a netshoot **overlay** pod (NOT hostNetwork) pinned per node via `nodeName`,
and a hostNetwork netshoot for underlay/tcpdump. Always test against **live** pod IPs
(`kubectl get pod -o wide`, Running only) — stale pod IPs are the documented red-herring.

### Phase 0 test suite (MTU, pre-migration, LAN mesh intact)
1. `kubectl -n kube-system get cm cilium-config -o jsonpath='{.data.mtu}'` == `1230`.
2. `ip link show cilium_vxlan` MTU == 1230 on 3 nodes.
3. Large-packet pod-to-pod across the existing LAN mesh (forces a full-MTU frame):
   `kubectl exec <podA> -- ping -c3 -s 1200 -M do <podB-IP>` == 0% loss, no "frag needed".
4. In-cluster DNS still resolves: `kubectl exec <pod> -- nslookup kubernetes.default` OK.
5. No new cilium drops: `cilium monitor --type drop` quiet during the pings.
   **Gate:** all pass → proceed. Any fail → revert MTU, stop.

### Per-node test suite (run after EACH of Phases 1–4)
Let `N` = the just-migrated node.
1. **InternalIP flipped:** `kubectl get node N -o wide` INTERNAL-IP is the `100.x` address;
   `kubectl get ciliumnode N -o jsonpath='{.spec.addresses}'` shows the TS IP as InternalIP.
2. **Node Ready + cilium Ready:** `kubectl get node N`; `cilium-health status` from another
   node shows N Endpoints `1/1` (allow ~3 min cache lag — trust a live ping over the cache).
3. **Bidirectional overlay to every family** (0% loss BOTH directions):
   - N-pod → a LAN-core pod, LAN-core pod → N-pod
   - N-pod → a guest pod (macintel01 + rpi01), guest pod → N-pod
   - `for tgt in <live IPs>; do kubectl exec <N-pod> -- ping -c3 -W2 $tgt; done`
4. **TCP not just ICMP:** `kubectl exec <N-pod> -- nc -zvw3 <a-service-backend-IP> <port>` OK.
5. **Large frame over the tailscale underlay:** `ping -c3 -s 1200 -M do <cross-node-pod>` == 0%.
6. **DNS from an N-pod:** `nslookup kubernetes.default` and an external name both resolve.
7. **Fresh sandbox (R6):** `kubectl run smoke-N --image=busybox --overrides nodeName=N -- sleep 30`
   reaches Ready (proves istio-cni/ambient sandbox creation works on N).
8. **Return-path sanity (the original bug):** hostNetwork tcpdump on N shows VXLAN in/out on
   `tailscale0` only (no `end0`/`tailscale0` split); `cilium monitor -v` on N shows BOTH the
   outbound request AND the inbound reply for a cross-node ping.
9. **Longhorn (R5):** `kubectl get nodes.longhorn.io N` READY True; any volumes with a replica
   on N return to `attached`/`healthy`.
   **Gate:** all pass → next phase. Any fail → §8 rollback for node N.

### Control-plane test suite (Phase 4 only, additional)
1. `kubectl get --raw='/healthz'` == ok; `kubectl get nodes` responds (apiserver serving).
2. `talosctl -n <dell01> get members` / etcd healthy, single member, leader present.
3. apiserver cert presents the TS IP SAN: `openssl s_client -connect 100.82.148.37:6443` (or
   via KubePrism) shows `100.82.148.37` in SANs; no x509 SAN errors in kubelet/agent logs.
4. KubePrism still serving: every node's cilium-agent stays Ready (no apiserver-reach flap).
5. ArgoCD reconciles (`kubectl -n argocd get applications` Synced/Healthy); external
   `argocd.syscd.live` == 302.
   **Gate:** all pass → done. Any fail → dell01 is break-glass-recoverable over LAN
   `192.168.68.100`; revert nodeIP, re-apply, reboot.

## 8. Rollback (per node)
- Revert that node's `nodeIP.validSubnets` to `192.168.68.0/24` (LAN nodes) in the template,
  `nostos render <node>` + `nostos apply <node>` (reboot for InternalIP to re-take). The node
  returns to the prior mixed-mesh state — degraded but known. dell01 is always reachable over
  its retained LAN interface `192.168.68.100` for break-glass.
- Phase 0 MTU is independently revertible via git revert of the cilium.yaml value.

## 9. Open questions to resolve before starting
- Confirm the Tailscale **ACL / route approvals** do not approve any cluster CIDR (guards R3).
- Confirm `api.k8s.lan` resolution + any LAN-hardcoded consumers won't break when dell01
  advertises on its TS IP (KubePrism localhost path suggests fine; verify).
- Decide acceptable throughput floor for R4 before migrating tp4 (set a number).
