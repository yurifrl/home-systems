---
name: oom-check
description: Find pods crashing or restarting because their memory limits are too low (OOMKilled) in the home-systems Kubernetes cluster. Use when apps are CrashLoopBackOff, restarting, or you suspect a resource limit is undersized. Cross-references live limits against VPA recommendations to suggest a corrected limit.
---

# oom-check — detect apps killed by too-low memory limits

An OOMKilled container is one whose memory **limit** was set below what it
actually needs: the kernel kills it, the kubelet restarts it, and it usually
ends up `CrashLoopBackOff`. This skill finds those pods and tells you what the
limit *should* be, using the cluster's own VPA recommendations as evidence.

## Cluster gotcha (read first)

`api.k8s.lan:6443` round-robins across all three control planes (dell01
`.100`, tp1 `.107`, tp4 `.114`). When a control plane is memory-starved its
kube-apiserver crashloops, so `kubectl` against the VIP fails intermittently.
Pin kubectl to a **specific healthy apiserver** and retry. The bundled script
does this automatically.

Check which apiservers are healthy:

```bash
for ip in 192.168.68.100 192.168.68.107 192.168.68.114
  talosctl -n $ip -e $ip get staticpodstatus 2>/dev/null | grep apiserver
end
```

The one with `READY=True` is your best `--server` target.

## Run the check

```bash
.agents/skills/oom-check/oom-check.sh
```

It prints, for every container that has been OOMKilled or is restarting:

```
NAMESPACE  POD/CONTAINER            RESTARTS  LASTREASON   LIMIT    VPA_TARGET  SUGGESTED_LIMIT
litellm    litellm/litellm          14        OOMKilled    2Gi      1850Mi      3Gi
```

- **LIMIT** — the memory limit currently set (or `none` → covered only by the namespace LimitRange).
- **VPA_TARGET** — steady-state memory VPA measured (the trustworthy signal).
- **SUGGESTED_LIMIT** — `VPA_TARGET` rounded up ×1.5 (headroom for spikes).

> Do NOT use VPA `upperBound` for limits — with sparse history it explodes to
> absurd values (litellm's upperBound was once 255Gi). Always size limits from
> `target × headroom`, never `upperBound`.

## Fixing a finding

This is a GitOps cluster — **never `kubectl apply`**. Set the corrected limit
in the workload's chart/Application and let ArgoCD sync:

- support-chart apps: the `resources:` block under the `deployments:`/`statefulSets:` entry in `k8s/applications/<app>.yaml`.
- own/upstream charts: the chart's `resources` values key.
- fleet default: `k8s/charts/support-cluster/values.yaml` → `limitRanges` (applies to app namespaces that set no explicit limit).

## Notes

- Cross-site nodes (macarm01, macintel01, pc01) may show `<unknown>` in
  `kubectl top` — metrics-server can't scrape them. VPA data still works.
- A freshly-restarted VPA recommender returns a flat floor
  (`target == lowerBound == upperBound`) until it re-aggregates ~an hour of
  history; treat flat values as "no data yet," not as a real recommendation.
