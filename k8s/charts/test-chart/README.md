# Test Chart

This is a simple Helm chart created for testing purposes. It deploys a basic Nginx container with a configurable message exposed through an environment variable.

## Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas to deploy | `1` |
| `image.repository` | Container image repository | `nginx` |
| `image.tag` | Container image tag | `stable` |
| `image.pullPolicy` | Container image pull policy | `IfNotPresent` |
| `service.type` | Kubernetes service type | `ClusterIP` |
| `service.port` | Kubernetes service port | `80` |
| `testValue` | Test value to demonstrate configuration | `Hello from test chart!` |
| `resources` | Pod resource requests/limits | See `values.yaml` |

## Usage

This chart is intended to be used for testing chart repository functionality and Argo CD application deployments. 