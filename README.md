# Home Systems

## Quick Start

```bash
ssh nixos@100.70.133.100

cd ...

# Switch configuration on Pi
sudo nixos-rebuild switch --flake .#rpi --impure --show-trace

# Set up kubectl aliases and permissions
alias k=kubectl
set -gx KUBECONFIG /etc/rancher/k3s/k3s.yaml
sudo chmod 644 /etc/rancher/k3s/k3s.yaml
k get nodes
```

### Initial Setup
```bash
# Set up 1Password access
op item get "Home Server" --fields "private key" --reveal > secrets/id_ed25519

# Copy your id_ed25519 to the pi and clone this repo

# Add unstable channel
sudo nix-channel --add https://nixos.org/channels/nixpkgs-unstable nixpkgs-unstable
sudo nix-channel --update
```

### Building and Deploying
```bash
# Check configuration
nix flake check

# Build AMD and Intel images
nix build .#packages.aarch64-linux.default .#packages.x86_64-linux.default --impure

# Build Pi image
nix build .#images.rpi --impure

# Deploy everywhere
docker compose run --rm deploy . -- --impure

# Switch configuration on Pi
nixos-rebuild switch --flake .#rpi --impure --show-trace 
```

### Kubernetes Setup
```bash
# Set up kubectl aliases and permissions
alias k=kubectl
set -gx KUBECONFIG /etc/rancher/k3s/k3s.yaml
sudo chmod 644 /etc/rancher/k3s/k3s.yaml
k get nodes
```

## TODO
- [ ] Find where kubelet config.yaml is
- [ ] Look into the init of kubeadm in default.nix
- [ ] Make so that the system never comes up without tailscale

## Kubernetes Troubleshooting

```bash
# Check service statuses
systemctl status kubelet.service
systemctl status kubeadm.service

# View logs for kubernetes services
journalctl -u kubelet.service
journalctl -u kubeadm.service

# Check all k8s services at once
sudo journalctl -u etcd.service \
-u flannel.service \
-u kube-apiserver.service \
-u kube-controller-manager.service \
-u kube-proxy.service \
-u kube-scheduler.service

# Clean kubernetes directories if needed
rm -rf /var/lib/kubernetes/ /var/lib/etcd/ /var/lib/cfssl/ /var/lib/kubelet/ \
/var/lib/kubernetes/secrets/ /etc/kube-flannel/ /etc/kubernetes/
```

## References

### Kubernetes with NixOS
- [Kubernetes the Hard Way with Nix](https://github.com/m1dugh/nix-cluster/tree/master)
- [thpham/magics](https://github.com/thpham/magics/blob/master/k8s-cluster/kubernetes/default.nix#L32)
- [justinas/nixos-ha-kubernetes](https://github.com/justinas/nixos-ha-kubernetes/blob/master/modules/controlplane/apiserver.nix)
- [NixOS/nixpkgs Kubernetes modules](https://github.com/NixOS/nixpkgs/blob/master/nixos/modules/services/cluster/kubernetes/apiserver.nix)
- [rydnr/nixos-kubernetes](https://github.com/rydnr/nixos-kubernetes/tree/main)

### Sample Configurations
- [ymgyt/mynix](https://github.com/ymgyt/mynix/blob/main/homeserver/modules/kubernetes/master.nix)
- [heywoodlh/nixos-configs](https://github.com/heywoodlh/nixos-configs/blob/master/nixos/roles/kubevirt-master.nix)
- [JD95/nixos-configuration](https://github.com/JD95/nixos-configuration/blob/main/containers/kube-master/flake.nix)

### Kubeadm Resources
- [BenSchZA/nixos-configuration](https://github.com/BenSchZA/nixos-configuration/blob/master/modules/kubeadm/default.nix)
- [arianvp/nixos-stuff](https://github.com/arianvp/nixos-stuff/blob/master/modules/kubeadm/kubeadm-base.nix)
- [addreas/homelab](https://github.com/addreas/homelab/tree/main)

### Additional Resources
- [BMC API Documentation](https://docs.turingpi.com/docs/turing-pi2-bmc-api#flash--firmware)
- [Storage Options](https://docs.turingpi.com/docs/turing-pi2-kubernetes-cluster-storage#option-2-the-longhorn)
- [Multicast DNS](https://en.wikipedia.org/wiki/Multicast_DNS)
- [Zero-configuration networking](https://en.wikipedia.org/wiki/Zero-configuration_networking#DNS-SD)