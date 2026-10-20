{{/*
agent-substrate shared helpers.
The official install names the DaemonSet plain `atelet`; version stamps live
only on image tags.
*/}}

{{- define "agent-substrate.name" -}}
{{- "agent-substrate" -}}
{{- end -}}

{{/*
nodeSelector rendered into every pod template. Mixed-arch home clusters need
arch pinning: the mirrored images are linux/amd64-only (ko single-arch builds),
so without affinity pods land on arm64 nodes and ErrImagePull. Override with
`nodeSelector: {}` on single-arch clusters.
Call as `include "agent-substrate.nodeSelector" (dict "root" $ "overrides" X)`
to use X instead of the default for upstream multi-arch images (postgres,
rustfs) that are safe on arm64.
*/}}
{{- define "agent-substrate.nodeSelector" -}}
{{- if hasKey . "overrides" -}}
{{- $overrides := index . "overrides" -}}
{{- $root := index . "root" -}}
{{- if ne $overrides nil -}}
{{- toYaml $overrides -}}
{{- else -}}
{{- $defaults := default (dict "kubernetes.io/arch" "amd64") $root.Values.nodeSelector -}}
{{- toYaml $defaults -}}
{{- end -}}
{{- else -}}
{{- $defaults := default (dict "kubernetes.io/arch" "amd64") .Values.nodeSelector -}}
{{- toYaml $defaults -}}
{{- end -}}
{{- end -}}
