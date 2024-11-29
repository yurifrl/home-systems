

Sample values

```yaml
env:
  TZ: "America/Sao_Paulo"
hostNetwork: true
# prometheus:
#   serviceMonitor:
#     enabled: false
probes:
  liveness:
    enabled: false
  readiness:
    enabled: false
  startup:
    enabled: true
persistence:
  config:
    enabled: true
    storageClass: "longhorn"
    accessMode: ReadWriteOnce
    size: "10Gi"
    # maybe use this
    # existingClaim:  # your-claim
    annotations:
      recurring-job-group.longhorn.io/default: enabled
addons:
  codeserver:
    enabled: true
    image:
      repository: codercom/code-server
      tag: 3.12.0
    workingDir: "/config"
    args:
      - --user-data-dir
      - "/config/.vscode"
      - --auth
      - "none"
    volumeMounts:
      - name: config
        mountPath: /config
tolerations:
  - key: "arm"
    operator: "Exists"
resources:
  limits:
    memory: 2500Mi
  requests:
    cpu: 100m
    memory: 1000Mi
postgresql:
  enabled: true
  postgresqlUsername: home-assistant
  postgresqlDatabase: home-assistant
  persistence:
    enabled: true
    storageClass: "longhorn"
    # maybe use this
    # existingClaim:  # your-claim
    annotations:
      recurring-job-group.longhorn.io/default: enabled
  volumePermissions:
    enabled: true
    image:
      repository: bitnami/minideb
      tag: buster-arm64
  image:
    registry: docker.io
    repository: postgres
    tag: 12
```