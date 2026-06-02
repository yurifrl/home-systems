{{- define "hermes-agent.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes-agent.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "hermes-agent.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes-agent.tenantLabelKey" -}}
tenant.hermes.ai/id
{{- end -}}

{{- define "hermes-agent.tenantLabels" -}}
{{- if .Values.tenant.id }}
{{ include "hermes-agent.tenantLabelKey" . }}: {{ .Values.tenant.id | quote }}
{{- end }}
{{- with .Values.tenant.labels }}
{{ toYaml . }}
{{- end }}
{{- end -}}

{{- define "hermes-agent.labels" -}}
helm.sh/chart: {{ include "hermes-agent.chart" . }}
{{ include "hermes-agent.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: hermes-agent
{{- $tenantLabels := include "hermes-agent.tenantLabels" . }}
{{- if $tenantLabels }}
{{ $tenantLabels }}
{{- end }}
{{- end -}}

{{- define "hermes-agent.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hermes-agent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "hermes-agent.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "hermes-agent.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "hermes-agent.configMapName" -}}
{{- printf "%s-config" (include "hermes-agent.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes-agent.bootstrapConfigMapName" -}}
{{- default (include "hermes-agent.configMapName" .) .Values.bootstrap.existingConfigMap -}}
{{- end -}}

{{- define "hermes-agent.generatedSecretName" -}}
{{- printf "%s-secrets" (include "hermes-agent.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes-agent.secretName" -}}
{{- if .Values.externalSecret.enabled -}}
{{- default (include "hermes-agent.generatedSecretName" .) .Values.externalSecret.target.name -}}
{{- else if .Values.secrets.existingSecret -}}
{{- .Values.secrets.existingSecret -}}
{{- else -}}
{{- include "hermes-agent.generatedSecretName" . -}}
{{- end -}}
{{- end -}}

{{- define "hermes-agent.pvcName" -}}
{{- printf "%s-data" (include "hermes-agent.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "hermes-agent.servicePorts" -}}
{{- $ports := list -}}
{{- if gt (len .Values.service.ports) 0 -}}
  {{- $ports = .Values.service.ports -}}
{{- else -}}
  {{- if .Values.apiServer.enabled -}}
    {{- $ports = append $ports (dict "name" "api-server" "port" (.Values.apiServer.port | int) "targetPort" (.Values.apiServer.port | int) "containerPort" (.Values.apiServer.port | int) "protocol" "TCP") -}}
  {{- end -}}
  {{- if .Values.webhook.enabled -}}
    {{- $ports = append $ports (dict "name" "webhook" "port" (.Values.webhook.port | int) "targetPort" (.Values.webhook.port | int) "containerPort" (.Values.webhook.port | int) "protocol" "TCP") -}}
  {{- end -}}
  {{- if .Values.telegramWebhook.enabled -}}
    {{- $ports = append $ports (dict "name" "telegram-webhook" "port" (.Values.telegramWebhook.port | int) "targetPort" (.Values.telegramWebhook.port | int) "containerPort" (.Values.telegramWebhook.port | int) "protocol" "TCP") -}}
  {{- end -}}
{{- end -}}
{{- $ports | toJson -}}
{{- end -}}

{{- define "hermes-agent.primaryServicePortNumber" -}}
{{- $servicePorts := include "hermes-agent.servicePorts" . | fromJsonArray -}}
{{- if and .Values.service.enabled (gt (len $servicePorts) 0) -}}
{{- (index $servicePorts 0).port -}}
{{- else -}}
{{- fail "service.enabled=true with either explicit service.ports entries or enabled apiServer/webhook/telegramWebhook ports is required for ingress, httpRoute, or virtualService routing" -}}
{{- end -}}
{{- end -}}
