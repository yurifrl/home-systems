# Things to solve

- [x] Load balancer ips
- [ ] storage on the control plane and the workers
- [ ] Zigbee dongle
- [x] make .live work
- [ ] make .tech work
- [ ] make .dev work
- [x] Handle the error where from a worker, the api server is not reachable, not sure what solved, but installing with interactive mode solved it, kinda, I think it's tailscale related.
- [ ] tailscale

# Talos workflow

## Setup

```bash
set -ex T_MASTER_IP 192.168.68.100
set -ex T_WORKER_1_IP 192.168.68.107
set -ex T_WORKER_2_IP 192.168.68.114
set -ex T_CLUSTER_NAME rpi
```

## Routine

```bash
# Control plane
talosctl apply-config -n $T_MASTER_IP -f talos/config/controlplane.yaml -p @talos/config/patches.yaml

# Workers
talosctl apply-config -n $T_WORKER_1_IP,$T_WORKER_2_IP -f talos/config/worker.yaml
```

## Talos commands

```bash
# Get manifests
talosctl -n rpi get manifests

# List files
talosctl -n rpi ls /etc/

# Set talosconfig endpoint and nodes parameters
talosctl config endpoint $T_MASTER_IP
talosctl config nodes $T_MASTER_IP $T_WORKER_1_IP $T_WORKER_2_IP
```

## Start over

```bash
cd ~/Downloads/images

# 
sudo diskutil unmountDisk /dev/disk5
sudo dd if=metal-arm64-1.8.4.raw of=/dev/disk5 conv=fsync bs=4M status=progress
sudo diskutil unmountDisk /dev/disk5

#
tpi flash -i ./rockship.raw -n 1
tpi flash -i ./rockship.raw -n 4
```

### Apply config

```bash
# Apply with insecure while in maintenance mode
# Control plane
talosctl apply-config -n $T_MASTER_IP -f talos/config/controlplane.yaml -p @talos/config/patches.yaml --insecure

# Workers
talosctl apply-config -n $T_WORKER_1_IP,$T_WORKER_2_IP -f talos/config/worker.yaml --insecure
```

## Setup From scratch

```bash
# Get your config, existing control plane or create a new one with
talosctl gen config $T_CLUSTER_NAME https://$T_MASTER_IP:6443 -o .

# Initial Apply needs to be with --insecure
talosctl apply-config -n $T_MASTER_IP -f talos/config/controlplane.yaml --insecure 

# Get kubeconfig
talosctl kubeconfig . -n $T_MASTER_IP
set -gx KUBECONFIG kubeconfig
```

## Image factory

```bash
# Get image ID
curl -X POST --data-binary @schematic-v1.yaml https://factory.talos.dev/schematics

curl -Lo metal-arm64_v1.8.4.raw.xz https://factory.talos.dev/image/$ID/v1.8.4/metal-arm64.raw.xz

xz -d metal-arm64_v1.8.4.raw.xz

sudo dd if=metal-arm64.raw of=/dev/disk5 conv=fsync bs=4M status=progress
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
