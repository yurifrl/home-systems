---
name: "Rightsize"
description: "Right-size workloads from Goldilocks/VPA recommendations vs current requests"
category: Ops
tags: [ops, resources, vpa, goldilocks]
---

Right-size the platform from the in-cluster Goldilocks/VPA recommendations.

1. Read (read-only):
   ```bash
   kubectl get vpa -A -o json
   kubectl get deploy,statefulset,daemonset -A -o json
   ```
2. Join each VPA `status.recommendation.containerRecommendations[].target`
   against the workload's current `resources.requests`. Humanize memory to Mi.
3. Print a table: `WORKLOAD  CONTAINER  CPU(cur->rec)  MEM(cur->rec)  FLAG`,
   flagging `NO-REQ` where no requests are set. Lead with the count of `NO-REQ`.

Guardrails:
- Propose **requests** only. Skip memory limits — VPA `upperBound` is unreliable
  until there are several clean days of metrics.
- Apply only via GitOps (chart values + commit). Never `kubectl patch` requests —
  ArgoCD selfHeal reverts it.
