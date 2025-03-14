{{/*
Expand the name of the chart.
*/}}
{{- define "support.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}


{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "support.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "support.labels" -}}
helm.sh/chart: {{ include "support.chart" . }}
{{ include "support.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "support.selectorLabels" -}}
app.kubernetes.io/name: {{ include "support.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
