{{- define "ax.partOf" -}}
ax
{{- end -}}

{{/*
nodeSelector for every ax pod template. Images are linux/amd64-only (ko
single-arch builds) and the target cluster is mixed-arch, so pin to amd64
unless overridden.
*/}}
{{- define "ax.nodeSelector" -}}
{{- $defaults := default (dict "kubernetes.io/arch" "amd64") .Values.nodeSelector -}}
{{- toYaml $defaults -}}
{{- end -}}
