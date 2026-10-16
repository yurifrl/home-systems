# ax

![Version: 0.2.0](https://img.shields.io/badge/Version-0.2.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 92d297e](https://img.shields.io/badge/AppVersion-92d297e-informational?style=flat-square)

Deploys google/ax: ax-server (direct task executor) and Redis, into the Helm release namespace. ax-server talks to the agent-substrate control plane (api.ate-system.svc), so this chart must deploy after agent-substrate.

**Homepage:** <https://github.com/yurifrl/home-systems>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| yurifrlcw |  |  |

## Source Code

* <https://github.com/yurifrl/home-systems/tree/main/k8s/charts/ax>

## Model API key

ax-server resolves the Kubernetes Secret `gemini-api-secret` / key `GEMINI_API_KEY`
in the task atespace namespace and injects it into every ActorTemplate it creates.
The name is fixed by ax's secret contract (`internal/controller/reconciler.go`);
this chart provisions it through External Secrets Operator:

1. ESO installed with a 1Password-backed store — default
   `externalSecret.secretStore` is ClusterSecretStore `onepassword`.
2. A 1Password item named `externalSecret.name` (`ax-gemini-api-secret`) with a
   `GEMINI_API_KEY` field.

The materialized Secret lands in `externalSecret.namespace` — the atespace
namespace (`default`), NOT the release namespace. With
`externalSecret.enabled=false`, create the Secret there yourself.

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| ax.version | string | `"92d297e"` | The ax build version. Stamps the server image tag and appVersion expectations. The mirror-ate-images workflow (ko build) pushes exactly this tag; it must exist or pods stay ImagePullBackOff. |
| env.atenetRouterAddr | string | `"atenet-router.ate-system.svc.cluster.local:80"` | atenet-router ingress for task sockets. |
| externalSecret | object | `{"enabled":true,"name":"ax-gemini-api-secret","namespace":"default","refreshInterval":"1h","secretStore":{"kind":"ClusterSecretStore","name":"onepassword"},"target":{"creationPolicy":"Owner","deletionPolicy":"Retain","name":"gemini-api-secret"}}` | Model-provider API key, provisioned via External Secrets Operator from 1Password. ax-server resolves the K8s Secret `gemini-api-secret` (fixed ax contract, not configurable upstream) / key `GEMINI_API_KEY` in the task atespace's Kubernetes namespace — NOT the release namespace — and injects it into every ActorTemplate it creates for a task. The 1Password item must contain a field named GEMINI_API_KEY; by repo convention the item name equals the ExternalSecret name (see obs/, hermes/). |
| externalSecret.name | string | `"ax-gemini-api-secret"` | 1Password item AND ExternalSecret name (repo convention: identical). |
| externalSecret.namespace | string | `"default"` | Namespace of the materialized K8s Secret: the atespace namespace ax-server reads secrets from (the substrate `default` atespace maps to the Kubernetes `default` namespace here). |
| externalSecret.refreshInterval | string | `"1h"` | ESO refresh interval for the upstream item. |
| externalSecret.secretStore.kind | string | `"ClusterSecretStore"` | ESO store holding the 1Password provider; ClusterSecretStore in the ESO namespace is the repo-wide default. |
| externalSecret.target.name | string | `"gemini-api-secret"` | K8s Secret name ax-server reads. Fixed by ax's secret contract (internal/controller/reconciler.go geminiSecretName); change only together with an ax version that renames it. |
| imagePullSecrets | list | `[]` | imagePullSecrets attached to every ax pod spec (kind profile sets [{name: ax-registry}]). |
| images.redis | object | `{"repository":"docker.io/library/redis","tag":"7-alpine"}` | Official upstream redis, not ko-built. |
| images.server | object | `{"repository":"ghcr.io/yurifrl/ax/ax-server","tag":"92d297e@sha256:5734e79fa6277ad19343603c385f003faa245fb05eeef5d0396713ca39758f9a"}` | ko-built image, published to public ghcr by the mirror-ate-images workflow (there are no upstream ax images; upstream manifests reference ko://github.com/google/ax/cmd/ax-server). |
| nodeSelector | object | `{"kubernetes.io/arch":"amd64"}` | Pod nodeSelector for all ax workloads. Images are linux/amd64-only (ko single-arch builds); the home talos cluster is mixed-arch. |
| redis | object | `{"address":"","local":{"enabled":true}}` | Redis for ax-server's streams controller. By default the chart deploys its own ax-redis Deployment in the release namespace (requirepass off, emptyDir). Set local.enabled=false and address to reuse an existing instance -- the home talos cluster points ax-server at the shared cluster redis (redis.redis.svc.cluster.local:6379, db 0 of the shared index registry). |
| registry | object | `{"password":"","server":"ghcr.io","username":"yurifrl"}` | ghcr.io/yurifrl/ax/* is public: no pull Secret, no imagePullSecrets. registry.* only builds one when a private registry needs it. |
| replicas.server | int | `1` |  |
| snapshots.bucket | string | `""` | REQUIRED. Bucket ax-server records task snapshots into, forwarded to substrate ActorTemplates. On EKS use s3:// (substrate's ategcs parses any scheme and ATE_STORAGE_BACKEND=s3 serves it); a trailing slash keeps name concatenation unambiguous. Render fails when unset because the upstream default (a Google dev gs:// bucket) is meaningless off-GKE. |
| substrate.authority | string | `"api.ate-system.svc"` | TLS server name (SNI/hostname verification) and the ServiceAccount token audience for ax-server's substrate connection. |
| substrate.caFile | string | `"/run/servicedns-ca/trust-bundle.pem"` | Path of the cluster trust bundle mounted for verifying the substrate control plane. Served by agent-substrate's podcert servicedns signer. |
| substrate.endpoint | string | `"api.ate-system.svc.cluster.local:443"` | Control-plane endpoint ax-server connects to (gRPC over TLS). |
| template.atespace | string | `"default"` | Substrate atespace holding task ActorTemplates (a substrate concept, unrelated to the Kubernetes release namespace). Tasks run in the `default` atespace; ax-server reads its model-key Secret from the Kubernetes namespace matching this atespace. |
| template.name | string | `"default-template"` | ActorTemplate ax-server fetches from substrate for each task. |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
