{{- define "support.slo.labels" -}}
prometheus: k8s
role: alert-rules
pyrra.dev/severity: critical
pyrra.dev/environment: production
pyrra.dev/app: {{ .app }}
pyrra.dev/chart: support
{{- end }} 