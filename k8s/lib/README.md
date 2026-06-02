# k8s/lib — archived / inert manifests

This directory holds manifests that are **intentionally not synced** by ArgoCD.

ArgoCD discovers Applications only under `k8s/applications/` — both the
`ApplicationSet` (`manifests/applicationset.yaml`, glob `k8s/applications/*.yaml`)
and the app-of-apps (`manifests/applications.yaml`, `path: k8s/applications`
`recurse: true`) scan that directory only. Files here are reference copies that
ArgoCD ignores.

## Contents

- `kube-prometheus-stack.yaml` — previous Prometheus + Alertmanager stack
  (Prometheus operator). Replaced by `k8s/applications/victoria-metrics-k8s-stack.yaml`.
- `grafana.yaml` — previous Grafana via the Bitnami `grafana-operator` plus
  dashboard mixins (kubernetes-mixin, kube-state-metrics, dotdc). Replaced by the
  Grafana bundled in the VictoriaMetrics k8s stack.

These were retired during the migration to VictoriaMetrics (single-node `VMSingle`
+ `VMAgent` + `VMAlert` + `VMAlertmanager` + bundled Grafana).

To roll back: move the desired file back into `k8s/applications/` and remove
`k8s/applications/victoria-metrics-k8s-stack.yaml`, then let ArgoCD reconcile.
