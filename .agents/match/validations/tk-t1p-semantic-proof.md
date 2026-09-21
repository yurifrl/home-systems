# tk-t1p validator semantic proof — vmsingle-vmks (kubectl port-forward svc/vmsingle-vmks 18428:8428, ctx admin@talos-default)
Date: 2026-09-06. All queries via /api/v1/query on live vmsingle. Simulation uses `vector(1)` as a stand-in for `ds_unavailable > 0` (identical vector-matching behavior: multi-label/zero-label LHS is irrelevant under `on()` matching).

## Cluster state at review time
- 7/7 nodes Ready (dell01,macarm01,macintel01,pc01,rpi01,tp1,tp4).

## Committed expr (9005818c): `ds > 0 unless on() count(kube_node_status_condition{condition="Ready",status!="true"}) > 0`
- unless-RHS alone, all nodes Ready:       count(...) = 28  -> non-empty -> suppresses ALWAYS
- simulated firing alert `vector(1) unless on() <RHS>`: EMPTY  -> a real DS regression right now would NOT page
- 7d range (5m subquery): count min=14, max=28 — NEVER 0. Clause is invariantly true.
- Root cause: kube-state-metrics emits status=true/false/unknown series per condition, scraped via 2 paths
  (job=kube-state-metrics + job=kubernetes-services-annotations). `status!="true"` filters series EXISTENCE:
  7 nodes x 2 non-true statuses x 2 scrape paths = 28. It never counts node health. No value filter => dead alert.

## Corrected expr: `ds > 0 unless on() count(kube_node_status_condition{condition="Ready",status="true"} == 0) > 0`
- now (all Ready): RHS empty; simulated firing alert -> FIRES (n=1)           [PASS]
- @ts 1788057000 (2026-08-30 02:30 UTC, real NotReady window): RHS non-empty (3) -> SUPPRESSED  [PASS]
(alt fix, same semantics: `count(kube_node_status_condition{condition="Ready",status!="true"} == 1) > 0` — also verified.)

## Other checks
- Diff scope: exactly nic-offload-fix.yaml expr + zigbee2mqtt.yaml for:15m (+description) + alertmanagerconfig.yaml inhibit_rules + tests. OK
- helm template: clean, 190 docs. helm unittest: 6/6 pass. OK — but new unit test matchRegex PINS the broken clause string.
- CRD (live vmalertmanagerconfigs.operator.victoriametrics.com v1beta1): inhibit_rules item props = exactly
  {equal, source_matchers, target_matchers}, all array. camelCase sourceMatchers/targetMatchers DO NOT exist. snake_case correct.
- KubeNodeNotReady/KubeNodeUnreachable defined in templates/monitoring/node-down.yaml via `max by (node)` -> node label present; inhibit equal:[node] will match.
- Watchdog test repair claim verified: victoriametrics-watchdog.yaml at main is kind: Deployment (56017b8b); old test pinned CronJob and would fail on main. Legit trivial repair.
