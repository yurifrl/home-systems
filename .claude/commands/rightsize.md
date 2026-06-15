---
name: "Rightsize"
description: "Right-size workloads from Goldilocks/VPA recommendations vs current requests"
category: Ops
tags: [ops, resources, vpa, goldilocks, requests, limits]
---

Right-size the platform's resource requests/limits using the in-cluster
Goldilocks/VPA recommendations (recommendation-only VPAs already exist for every
workload in `Off` mode).

**Input**: the argument after `/rightsize` is an optional namespace. No argument
= whole cluster.

## What to do

1. Pull the data (read-only):
   ```bash
   kubectl get vpa -A -o json
   kubectl get deploy,statefulset,daemonset -A -o json
   ```
   Filter VPAs to the requested namespace if one was given.

2. For each VPA's `status.recommendation.containerRecommendations[]`, join its
   `target` (recommended request) against the workload's **current**
   `resources.requests` (match by namespace + targetRef kind/name + container).
   Memory values may be raw bytes — humanize to Mi.

3. Print a table: `NAMESPACE  WORKLOAD  CONTAINER  CPU(cur->rec)  MEM(cur->rec)  FLAG`.
   Flag `NO-REQ` when the container has no requests set at all.

4. Lead with the headline: how many containers have **no requests** (these are
   why the scheduler over-packs nodes and why utilization HPAs read `<unknown>`).

## Guardrails / caveats (state these)

- The recommended **request** (`target`) is trustworthy. The recommended memory
  **limit** would be VPA's `upperBound`, which is wildly inflated when history is
  thin (<1d) — do NOT propose limits until a few clean days of metrics exist.
- Trust requires a healthy metrics pipeline: `metrics-server` (serves
  `metrics.k8s.io`) and `vmsingle` (VictoriaMetrics TSDB) both up.
- This is a **report + proposal** command. Apply changes only via GitOps: emit a
  `resources:` snippet for the app's chart values, then let the user commit. Never
  `kubectl edit`/patch requests directly — ArgoCD selfHeal will revert it.

## Optional output mode

If asked for "values", emit per container instead of a table:
```yaml
        - name: <container>
          requests: { cpu: <rec_cpu>, memory: <rec_mem> }
```
(omit `limits` until history is sufficient).
