# agent-substrate

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: a0b680d](https://img.shields.io/badge/AppVersion-a0b680d-informational?style=flat-square)

Deploys the agent-substrate (ate-system) platform on EKS: ate-api-server, ate-controller, atelet DaemonSet, atenet-router, agentgateway, CRDs, SandboxConfigs and RBAC. Replaces `ate-setup deploy ate-system`.

**Homepage:** <https://github.com/agent-substrate/substrate>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| yurifrlcw |  |  |

## Source Code

* <https://github.com/nsxbet/helm-charts/tree/main/charts/agent-substrate>

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| atelet.extraArgs | list | `[]` | localhost-registry-replacement=kind-registry:5000. |
| atelet.gcpAuthForImagePulls | bool | `false` | Pass GCP credentials for pulling actor images from GCR/Artifact Registry. MUST be false on EKS: node IAM handles ECR pulls; atelet's in-process go-containerregistry pulls are anonymous for non-GCP registries. |
| crds.install | bool | `true` |  |
| imagePullSecrets | list | `[]` | dockerconfigjson Secret (created when registry.password is set) so kind nodes without node IAM can pull the mirrored private images. |
| images.agentgateway | object | `{"repository":"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/agentgateway","tag":"v0.0.0-alpha.9f9744cf"}` | atenet-router runs the agentgateway image (see values.images.agentgateway); there is no separate atenet server image in the cluster manifests. |
| images.agentgateway.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/agentgateway"` | Upstream agentgateway (egress proxy), mirrored into ECR. Pinned by substrate's ate-install manifests (the image atenet-router runs). |
| images.ateapi.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/ateapi"` | ate-api-server image (ko build of substrate/cmd/ateapi). Mirrored to ECR by the ko mirror script (see scripts in the PR). |
| images.ateapi.tag | string | `"a0b680d"` |  |
| images.atecontroller.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/atecontroller"` |  |
| images.atecontroller.tag | string | `"a0b680d"` |  |
| images.atelet.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/atelet"` |  |
| images.atelet.tag | string | `"a0b680d"` |  |
| images.awscli.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/aws-cli"` | Bucket-bootstrap Job image (rustfs.yaml). |
| images.awscli.tag | string | `"2.17.0"` |  |
| images.jaeger.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/jaeger"` | Jaeger all-in-one trace store + UI (otel.collector.enabled). |
| images.jaeger.tag | string | `"1.55"` |  |
| images.otelcollector.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/otel-collector-contrib"` | In-cluster OTLP collector (otel.collector.enabled), digest-pinned upstream kind overlay (kind/otel-collector.yaml). |
| images.otelcollector.tag | string | `"0.157.0"` |  |
| images.podcert.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/podcertcontroller"` | podcertificate-controller (publishes the podidentity/servicedns ClusterTrustBundles the atelet/ate-controller projected volumes use). |
| images.podcert.tag | string | `"a0b680d"` |  |
| images.postgres.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/postgres"` | Bundled PostgreSQL (see postgres.bundled), digest-pinned upstream. |
| images.postgres.tag | string | `"18-alpine"` |  |
| images.rustfs.repository | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com/ate/rustfs"` | S3-compatible object store for local clusters (storage.rustfs.enabled). |
| images.rustfs.tag | string | `"1.0.0-beta.3"` |  |
| otel.collector | object | `{"enabled":false}` | Deploy the in-cluster collector + jaeger (kind profile, see templates/otel-collector.yaml). EKS keeps false: the pre-existing observability-namespace collector receives exports. |
| otel.exporterOtlpAddress | string | `"http://opentelemetry-collector.observability.svc.cluster.local:4317"` | OTLP exporter address used by atenet-router directly. |
| otel.exporterOtlpEndpoint | string | `"http://opentelemetry-collector.observability.svc.cluster.local:4317"` | OTLP endpoint for ate-otel-config (ate-api-server, ate-controller, atelet). Point at the cluster collector; empty string disables export only if the collector isn't deployed. |
| otel.extraEnv | object | `{}` | Additional data keys merged into the ate-otel-config ConfigMap. The kind profile sets OTEL_METRIC_EXPORT_INTERVAL, OTEL_METRIC_EXPORT_TIMEOUT (both 10000) and OTEL_LOGS_EXPORTER=otlp, mirroring the upstream overlay. |
| podMonitoring.enabled | bool | `false` | Deploy PodMonitoring CR (Google-managed K8s operator CRD) for atenet-router /stats/prometheus scraping. Enable only if the gke-managed-prometheus operator runs on the cluster. |
| postgres.bundled | bool | `true` | Deploy the bundled PostgreSQL StatefulSet. ate-setup's default deploy does the same for ate-system; set false when pointing the DSN at RDS or an external server. |
| postgres.connectionString | string | `""` | Full DSN override; empty uses the bundled server's default. Rendered into the ate-api-server-secret-envvars Secret (optional envFrom source). |
| postgres.resources | object | `{"limits":{"cpu":"16","memory":"2Gi"},"requests":{"cpu":"2","memory":"1Gi"}}` | Upstream sizing sustains control-plane activation load. Local clusters override (kind overlay: 250m/1Gi requests, 2cpu limits). |
| postgres.schema | string | `"public"` | ate-api-server rejects an empty schema (ate-setup default: public). |
| postgres.storage | string | `"500Gi"` | Headroom so WAL churn cannot fill the volume (full pg_wal = PANIC). Substrate base is 500Gi; local clusters override to 1Gi. |
| registry.password | string | `""` | ECR token. Empty creates no Secret (EKS: node IAM authenticates). |
| registry.server | string | `"710045723375.dkr.ecr.us-east-1.amazonaws.com"` | Private registry backing every image. Only used to build the ate-registry pull Secret (see imagePullSecrets); never persisted. |
| registry.username | string | `"AWS"` |  |
| sandbox.gvisor.assets.amd64.gvisor.sha256 | string | `"d547d81401461fd1c679c5c4fa0a6c2b8ef7dc3c22ce23c9e25dcc4c69cfd06f"` |  |
| sandbox.gvisor.assets.amd64.gvisor.url | string | `"gs://gvisor/releases/nightly/2026-09-02/x86_64/gvisor.tar.zstd"` |  |
| sandbox.gvisor.assets.arm64.gvisor.sha256 | string | `"a64916f9813ce7e4841a30480a599337f7dda07b421c6bf0123db2212aa7d1df"` |  |
| sandbox.gvisor.assets.arm64.gvisor.url | string | `"gs://gvisor/releases/nightly/2026-09-02/aarch64/gvisor.tar.zstd"` |  |
| sandbox.gvisor.enabled | bool | `true` | Apply the cluster-wide gvisor SandboxConfig (name: gvisor-default). |
| sandbox.gvisor.pauseImage | string | `"registry.k8s.io/pause:3.10.2@sha256:f548e0e8e3dc1896ca956272154dde3314e8cc4fde0a57577ee9fa1c63f5baf4"` |  |
| sandbox.microvm.assets.amd64.cloud-hypervisor.sha256 | string | `"448af3d4e59b22c2987f7df94c213ad40fb53a10d437e42b5ee6c4fce7c29ecc"` |  |
| sandbox.microvm.assets.amd64.cloud-hypervisor.url | string | `"kata-assets/cloud-hypervisor"` |  |
| sandbox.microvm.assets.amd64.kata-image.sha256 | string | `"96497f64da1de9c7473fef46c3d29ddd0805d334731cf9d903a21a5b2c33cefb"` |  |
| sandbox.microvm.assets.amd64.kata-image.url | string | `"kata-assets/rootfs.img"` |  |
| sandbox.microvm.assets.amd64.kata-kernel.sha256 | string | `"8e9dbc3a6e4c26adb089d23d31201edebce905c7f3e31aa67b32bdd41df1b86f"` |  |
| sandbox.microvm.assets.amd64.kata-kernel.url | string | `"kata-assets/vmlinux"` |  |
| sandbox.microvm.assets.amd64.virtiofsd.sha256 | string | `"15b2e72a78cc08a9bd8a6943e89fb69c88cb3cbeb63069efceade835342ac7d4"` |  |
| sandbox.microvm.assets.amd64.virtiofsd.url | string | `"kata-assets/virtiofsd"` |  |
| sandbox.microvm.assets.arm64.cloud-hypervisor.sha256 | string | `"f192b510eea1c710cbc439d716bb0573c223fc463dbe3e6523788a2b7ef62850"` |  |
| sandbox.microvm.assets.arm64.cloud-hypervisor.url | string | `"kata-assets/cloud-hypervisor"` |  |
| sandbox.microvm.assets.arm64.kata-image.sha256 | string | `"7e3bdd500eb92de17c377bd84f8ed57e50d6bb68c7ec08c88f5e1916cd49f30d"` |  |
| sandbox.microvm.assets.arm64.kata-image.url | string | `"kata-assets/rootfs.img"` |  |
| sandbox.microvm.assets.arm64.kata-kernel.sha256 | string | `"4960f6cf781274eaa445fc9f88bb8c5ce951649278bc9caa13afb1e99cfe80e6"` |  |
| sandbox.microvm.assets.arm64.kata-kernel.url | string | `"kata-assets/vmlinux"` |  |
| sandbox.microvm.assets.arm64.virtiofsd.sha256 | string | `"3aa9240045d2261930ef04138fe05cf288f94c867e63e5d0845758bac5e3d458"` |  |
| sandbox.microvm.assets.arm64.virtiofsd.url | string | `"kata-assets/virtiofsd"` |  |
| sandbox.microvm.bucket | string | `"ate-cluster-bucket"` |  |
| sandbox.microvm.enabled | bool | `false` | Apply the microvm (cloud-hypervisor/kata) SandboxConfig. Opt-in: requires the kata asset set staged in the cluster bucket first. |
| sandbox.microvm.pauseImage | string | `"registry.k8s.io/pause:3.10.2@sha256:f548e0e8e3dc1896ca956272154dde3314e8cc4fde0a57577ee9fa1c63f5baf4"` |  |
| storage.backend | string | `"s3"` | Object-store backend for Actor data/snapshots. On EKS: s3 (atelet + ate-api-server read AWS credentials from the pod/node IRSA). |
| storage.rustfs.accessKey | string | `"rustfsadmin"` |  |
| storage.rustfs.enabled | bool | `false` | Deploy the in-cluster S3-compatible store + inject static AWS credentials into ate-api-server/atelet. Local clusters only. |
| storage.rustfs.endpoint | string | `"http://rustfs.ate-system.svc:9000"` |  |
| storage.rustfs.region | string | `"us-east-1"` |  |
| storage.rustfs.secretKey | string | `"rustfsadmin"` |  |
| storage.rustfs.storage | string | `"1Gi"` |  |
| substrate | object | `{"version":"a0b680d"}` | The substrate build version. Stamps the ate.dev/substrate-version label, names the atelet DaemonSet (atelet-<suffix>, see versionSuffix helper) and is the default image tag. Bumping this value triggers an atelet DS rollout. |
| validatingAdmissionPolicy.enabled | bool | `true` |  |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
