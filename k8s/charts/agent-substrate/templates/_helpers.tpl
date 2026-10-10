{{/*
agent-substrate shared helpers.
Mirrors substrate internal/versionlabel: the atelet DaemonSet name and the
ate.dev/substrate-version label are derived from the build version.
*/}}

{{- define "agent-substrate.name" -}}
{{- "agent-substrate" -}}
{{- end -}}

{{/*
nodeSelector rendered into every pod template. Mixed-arch home clusters need
arch pinning: the mirrored images are linux/amd64-only (ko single-arch builds),
so without affinity pods land on arm64 nodes and ErrImagePull. Override with
`nodeSelector: {}` on single-arch clusters.
*/}}
{{- define "agent-substrate.nodeSelector" -}}
{{- $defaults := default (dict "kubernetes.io/arch" "amd64") .Values.nodeSelector -}}
{{- toYaml $defaults -}}
{{- end -}}

{{/*
versionSuffix mirrors versionlabel.NameSuffix:
lowercase, map non-[a-z0-9] to '-', trim '-', max 30 chars,
otherwise "v" + first 10 hex chars of sha256(version).
Must stay byte-compatible with substrate/cmd/ate-setup so an ate-setup
managed cluster and this chart roll the same DaemonSet names.
*/}}
{{- define "agent-substrate.versionSuffix" -}}
{{- $version := .Values.substrate.version | toString | lower -}}
{{- $sanitized := regexReplaceAll "[^a-z0-9]" $version "-" | trimAll "-" -}}
{{- if or (eq (len $sanitized) 0) (gt (len $sanitized) 30) -}}
{{- printf "v%s" (substr 0 10 (sha256sum $version)) -}}
{{- else -}}
{{- $sanitized -}}
{{- end -}}
{{- end -}}
