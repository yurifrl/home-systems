# Things to solve

- [x] Load balancer ips
- [ ] storage on the control plane and the workers
- [ ] Zigbee dongle
- [x] make .live work
- [ ] make .tech work
- [ ] make .dev work

- [ ] tailscale

# Steps

```bash
# Get your config, existing control plane or create a new one with
talosctl gen config $CLUSTER_NAME https://192.168.68.100:6443 -o .

# Initial Apply needs to be with --insecure
talosctl apply-config -n 192.168.68.100 -f controlplane.yaml --insecure 

# Get kubeconfig
talosctl kubeconfig . -n 192.168.68.100
set -gx KUBECONFIG kubeconfig

# Kubernetes is running
helm repo add argo-cd https://argoproj.github.io/argo-helm
helm repo update
helm upgrade -n argocd --install argocd argo-cd/argo-cd -f ./argo-values.yaml --wait --create-namespace --atomic
../hack/secrets.sh
k apply -f./applications.yaml
```

## Image part

```bash
# Get image ID
curl -X POST --data-binary @schematic-v1.yaml https://factory.talos.dev/schematics

curl -Lo metal-arm64_v1.8.4.raw.xz https://factory.talos.dev/image/$ID/v1.8.4/metal-arm64.raw.xz

xz -d metal-arm64_v1.8.4.raw.xz

sudo dd if=metal-arm64.raw of=/dev/disk5 conv=fsync bs=4M status=progress
```


## Talos commands
```bash
# Get config
talosctl gen config $CLUSTER_NAME https://192.168.68.100:6443 -o .

# Apply config with patch
talosctl apply-config -n 192.168.68.100 -f controlplane.yaml -p @patches.yaml --insecure 

# Apply config interactively
talosctl apply-config  -n 192.168.68.100 --mode=interactive --insecure

# Get kubeconfig
talosctl kubeconfig .  -n 192.168.68.100

# Get manifests
talosctl -n rpi get manifests

# List files
talosctl -n rpi ls /etc/

# Set configs
talosctl config endpoint 192.168.68.100
talosctl config nodes 192.168.68.100 192.168.68.114 192.168.68.107
```

## Apply config

```bash
set -ex TL_CP_IP 192.168.68.100
set -ex TL_WTP1_IP 192.168.68.107
set -ex TL_WTP4_IP 192.168.68.114

# Control plane
talosctl apply-config -n $TL_CP_IP -f controlplane.yaml --insecure

# Worker tp1
talosctl apply-config -n $TL_WTP1_IP -f config01/worker.yaml --insecure

# Worker tp4
talosctl apply-config -n $TL_WTP4_IP -f config01/worker.yaml --insecure
```

# References

- [Ansible Playbook for Talos](https://github.com/JamesTurland/JimsGarage/tree/main/Ansible/Playbooks/Talos)
    - [Video to go with it](https://www.youtube.com/watch?v=TP8hVq1lCxM)
- [Homelab: Installation and Configuration of Talos on Raspberry Pi | by Alfonso fortunato - Freedium](https://freedium.cfd/https://medium.com/@alfor93/homelab-installation-and-configuration-of-talos-on-raspberry-pi-dee256527b9f)
    - Updated raspberry guide
- [Raspberry Pi 4 - Cannot install - 1.7.6+ · Issue #9283 · siderolabs/talos](https://github.com/siderolabs/talos/issues/9283) 
	- guy with the same issue
- [xvzf/homeassistant-yellow-talos](https://github.com/xvzf/homeassistant-yellow-talos/blob/main/README.md?plain=1)
	- Talos example with home assistant
