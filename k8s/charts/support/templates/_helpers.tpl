{{- define "support.slo.labels" -}}
prometheus: k8s
role: alert-rules
pyrra.dev/severity: critical
pyrra.dev/environment: production
pyrra.dev/app: {{ .app }}
pyrra.dev/chart: support
{{- end }}

{{/* Generate host entries based on domain settings */}}
{{- define "support.hosts" -}}
{{- $hostPrefix := .hostname | default .name }}
{{- $domains := .domains | default dict }}
{{- if .hosts }}
{{- range .hosts }}
- {{ . }}
{{- end }}
{{- else if .hostnameOverride }}
- {{ .hostnameOverride }}
{{- else }}
{{- if or (and (not (hasKey $domains "live")) $.Values.global.domains.live) (get $domains "live") }}
- {{ $hostPrefix }}.syscd.live
{{- end }}
{{- if or (and (not (hasKey $domains "tech")) $.Values.global.domains.tech) (get $domains "tech") }}
- {{ $hostPrefix }}.syscd.tech
{{- end }}
{{- if or (and (not (hasKey $domains "xyz")) $.Values.global.domains.xyz) (get $domains "xyz") }}
- {{ $hostPrefix }}.syscd.xyz
{{- end }}
{{- if or (and (not (hasKey $domains "dev")) $.Values.global.domains.dev) (get $domains "dev") }}
- {{ $hostPrefix }}.syscd.dev
{{- end }}
{{- end }}
{{- end }} 