{{- define "ax.partOf" -}}
ax
{{- end -}}

{{/*
nodeSelector for every ax pod template. Images are linux/amd64-only (ko
single-arch builds) and the target cluster is mixed-arch, so pin to amd64
unless overridden.
*/}}
{{- define "ax.nodeSelector" -}}
{{- $sel := dict "kubernetes.io/arch" "amd64" -}}
{{- if hasKey .Values "nodeSelector" -}}
{{- $sel = .Values.nodeSelector -}}
{{- end -}}
{{- if not (empty $sel) -}}
{{- toYaml $sel -}}
{{- end -}}
{{- end -}}
