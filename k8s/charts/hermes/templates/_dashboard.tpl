{{/*
The dashboard container. Rendered into its own Deployment, or (dashboard.sidecar)
into the agent pod: hermes 0.23's dashboard chat attaches to the gateway owner at
the 127.0.0.1 endpoint it publishes on the shared volume, so it must share the
gateway's network namespace.
*/}}
{{- define "hermes-agent.dashboardContainer" -}}
{{- $ctx := .ctx -}}
{{- $volume := .volume -}}
- name: hermes-dashboard
  image: "{{ $ctx.Values.image.repository }}:{{ $ctx.Values.image.tag }}"
  imagePullPolicy: {{ $ctx.Values.image.pullPolicy }}
  command: ["hermes", "dashboard", "--host", "0.0.0.0", "--port", {{ $ctx.Values.dashboard.port | quote }}, "--no-open"]
  ports:
    - name: dashboard
      containerPort: {{ $ctx.Values.dashboard.port }}
  env:
    {{- range $key, $value := $ctx.Values.env }}
    - name: {{ $key }}
      value: {{ printf "%v" $value | quote }}
    {{- end }}
    - name: HERMES_HOME
      value: {{ $ctx.Values.persistence.mountPath | quote }}
    - name: HOME
      value: {{ printf "%s/home" $ctx.Values.persistence.mountPath | quote }}
    - name: HERMES_DASHBOARD_PUBLIC_URL
      value: {{ printf "https://%s" (first $ctx.Values.dashboard.hosts) | quote }}
    {{- if and $ctx.Values.service.enabled $ctx.Values.apiServer.enabled }}
    - name: GATEWAY_HEALTH_URL
      value: {{ $ctx.Values.dashboard.healthUrl | default (printf "http://%s:%v" (include "hermes-agent.fullname" $ctx) $ctx.Values.apiServer.port) | quote }}
    {{- end }}
  envFrom:
    - secretRef:
        name: {{ include "hermes-agent.secretName" $ctx }}
  volumeMounts:
    - name: {{ $volume }}
      mountPath: {{ $ctx.Values.persistence.mountPath }}
    - name: tmp
      mountPath: /tmp
  securityContext:
    {{- $dashboardSecurityContext := deepCopy $ctx.Values.securityContext }}
    {{- $_ := set $dashboardSecurityContext "readOnlyRootFilesystem" false }}
    {{- toYaml $dashboardSecurityContext | nindent 4 }}
  resources:
    {{- toYaml $ctx.Values.dashboard.resources | nindent 4 }}
{{- end -}}
