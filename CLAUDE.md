# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is a home lab infrastructure repository managing a Kubernetes cluster running on Talos Linux with bare metal nodes (Raspberry Pi, Turing Pi, and x86 machines). The infrastructure uses GitOps principles with ArgoCD for application deployment, Istio service mesh in ambient mode, and various home automation services.

## Architecture

### Infrastructure Layers

1. **Talos Linux**: Bare metal Kubernetes OS running on:
   - Control plane: Raspberry Pi (192.168.68.100)
   - Workers: Turing Pi RK1 nodes (192.168.68.107, 192.168.68.114)
   - GPU worker: x86 PC (192.168.68.104) with NVIDIA GPU support

2. **Kubernetes**: Multi-node cluster managed by Talos
   - Service mesh: Istio Ambient mode with ztunnel
   - Monitoring: Prometheus + Grafana stack
   - Storage: Local path provisioner

3. **GitOps**: ArgoCD manages all applications
   - `k8s/applications/`: ArgoCD Application manifests
   - `k8s/charts/`: Local Helm charts
   - Apps auto-sync from git repository

### Key Components

**Support Chart** (`k8s/charts/support/`):
- Reusable Helm chart that provides common resources
- Templates: PVCs, PVs, ExternalSecrets, VirtualServices, ConfigMaps, Deployments, StatefulSets, Services, SLOs
- Used as a dependency by most applications to reduce duplication

**Custom Charts** (`k8s/charts/`):
- `appdaemon`: Home Assistant automation daemon
- `bind9`: Internal DNS server
- `echotube`: Private YouTube alternative
- `support-argo-workflows`: Argo Workflows with workflow templates
- `support-grafana`: Grafana dashboards and configuration
- `support-cluster`: Cluster-wide utilities

**ArgoCD Application Pattern**:
Applications typically reference two sources:
1. Their specific chart from `k8s/charts/*`
2. The `support` chart for common resources (PVCs, secrets, virtual services)

Example structure from `k8s/applications/argo-workflows.yaml`:
```yaml
sources:
  - path: k8s/charts/support-argo-workflows
    repoURL: https://github.com/yurifrl/home-systems.git
  - path: k8s/charts/support
    repoURL: https://github.com/yurifrl/home-systems.git
    helm:
      valuesObject:
        externalSecrets: [...]
```

### Secrets Management

- 1Password for secret storage
- External Secrets Operator syncs secrets to Kubernetes
- `task talos:onepassword` injects secrets into Talos configs using `op inject`
- Talos configs in `talos/` contain `op://` references, injected versions go to `talos/op/`

### Service Mesh & Networking

- **Istio Ambient Mode**: Sidecar-less service mesh using ztunnel
- **MetalLB**: LoadBalancer for bare metal
- **Istio Gateway**: Ingress gateway for external traffic
- **VirtualServices**: Defined via support chart for routing
- **Cloudflare Tunnel**: Exposes services externally

### Monitoring & Observability

- **Prometheus**: Metrics collection
- **Grafana**: Dashboards and visualization
- **Loki**: Log aggregation (referenced in hack/lib)
- **Pyrra**: SLO monitoring
- **Blackbox Exporter**: Endpoint monitoring
- **SLOs**: Defined in support chart templates and reference examples in `hack/reference/slos/`

## Development Workflow

### Task Runner

Uses [Task](https://taskfile.dev) for automation. Main taskfiles:

**Talos Operations** (`task talos:*`):
- `task talos:dashboard` - Open Talos dashboard
- `task talos:onepassword` - Inject 1Password secrets into configs
- `task talos:apply` - Apply configs to all nodes
- `task talos:apply-controlplane` - Apply control plane config only
- `task talos:tailscale-status` - Check Tailscale VPN status

**ArgoCD Operations** (`task argo:*`):
- `task argo:repo-add` - Add ArgoCD Helm repo
- `task argo:update` - Update ArgoCD manifests
- `task argo:apply` - Apply ArgoCD to cluster

**Turing Pi Operations** (`task turing:*`):
Available in `taskfiles/turing.yml`

**Proxmox Operations** (`task proxmox:*`):
Available in `taskfiles/proxmox.yml`

### Common Commands

**Talos**:
```bash
# Dashboard for all nodes
talosctl -n 192.168.68.100,192.168.68.114,192.168.68.107,192.168.68.115 dashboard

# Apply configuration
talosctl -n <node-ip> apply-config -f talos/op/<config-file>

# Get manifests
talosctl -n <node-ip> get manifests

# View logs
talosctl -n <node-ip> logs <service-name>
```

**Kubernetes**:
```bash
# Standard kubectl commands
kubectl get pods -A
kubectl logs -n <namespace> <pod>

# ArgoCD
kubectl get applications -n argocd
kubectl get applicationsets -n argocd
```

**Istio**:
```bash
# Check ztunnel logs for routing
kubectl -n istio-system logs -l app=ztunnel -f | grep -E "inbound|outbound"

# Get virtual services
kubectl get virtualservices -A
```

### Testing

**Argo Workflows**: The `support-argo-workflows` chart includes workflow templates in `files/` directory:
- `miniflux-youtube-subscribe`: Subscribes Miniflux to YouTube channels
- Workflows are deployed as ConfigMaps and can be submitted via Argo UI

**DNS Testing**: `hack/bind9-test/test-dns.sh` for testing BIND9 DNS

**Reference Manifests**: `hack/reference/` contains example configurations:
- SLOs examples in `hack/reference/slos/`
- Checkly monitoring examples
- Prometheus rules

### Adding a New Service

1. Create ArgoCD Application in `k8s/applications/<service>.yaml`
2. If custom resources needed, create chart in `k8s/charts/<service>/`
3. Use `support` chart as second source for common resources:
   ```yaml
   sources:
     - path: k8s/charts/<service>
       repoURL: https://github.com/yurifrl/home-systems.git
     - path: k8s/charts/support
       repoURL: https://github.com/yurifrl/home-systems.git
       helm:
         valuesObject:
           externalSecrets: [...]
           virtualServices: [...]
   ```
4. For new domains, follow checklist in README.md:
   - Add DNS entry in Cloudflare
   - Add domain in Cloudflare tunnel
   - Add domain in Istio ingress gateway
   - Add domain in Istio virtual service

### Home Automation

**AppDaemon** (`appdaemon/`):
- Python-based automation for Home Assistant
- Apps in `appdaemon/apps/`
- Configuration in `appdaemon/apps/apps.yaml`
- Local development with `docker-compose` using `appdaemon/compose.yaml`

**Automations** (`automations/`):
- Python automation scripts
- Uses `uv` for dependency management
- `mqtt_watch.py`: MQTT monitoring

## Important Patterns

### Multi-Source ArgoCD Applications

Most applications use multiple sources pattern:
1. Custom chart with application-specific resources
2. Support chart providing common infrastructure (PVCs, secrets, virtual services)

This reduces duplication and standardizes resource definitions.

### 1Password Secret Injection

Talos configs use `op://` URI references that are injected before applying:
```bash
task talos:onepassword  # Injects secrets from 1Password
task talos:apply        # Applies injected configs
```

Never commit files in `talos/op/` - they contain actual secrets.

### Istio Ambient Mode

Uses ztunnel for L4 networking without sidecars. Services are added to mesh via labels, not injection.

Check ztunnel logs to debug connectivity:
```bash
kubectl -n istio-system logs -l app=ztunnel -f | grep -E "inbound|outbound"
```

## Node-Specific Details

**Control Plane** (192.168.68.100):
- Raspberry Pi
- Config: `talos/controlplane-192.168.68.100.yaml`
- Runs Kubernetes control plane components

**Worker Nodes**:
- tp1 (192.168.68.107): Turing Pi RK1, ARM64
- tp4 (192.168.68.114): Turing Pi RK1, ARM64
- pc01 (192.168.68.104): x86 with NVIDIA GPU
- Configs: `talos/nodes/*.yaml`

Talos factory images include platform-specific extensions (NVIDIA drivers, QEMU guest agent, Tailscale).

## Documentation

Additional docs in `docs/`:
- `argocd.md`: ArgoCD password management
- `talos.md`: Detailed Talos setup and troubleshooting
- `istio.md`: Istio installation and ztunnel debugging
- `proxmox.md`, `turingpi.md`, `nas.md`: Hardware setup
- `dns.md`: DNS configuration
- `home-assistant.md`: Home Assistant integration