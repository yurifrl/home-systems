# Nix


```
nix flake check

nix build .#images.rpi --impure

sudo nix-channel --add https://nixos.org/channels/nixpkgs-unstable nixpkgs-unstable
sudo nix-channel --update

sudo nixos-rebuild switch --flake .#rpi --impure --show-trace 

sudo nixos-rebuild switch --flake .#rpi --impure --show-trace -I nixpkgs-unstable=https://nixos.org/channels/nixpkgs-unstable
```

# Kubernetes

## Troubleshooting


```bash
journalctl -u etcd.service
journalctl -u flannel.service
journalctl -u kube-apiserver.service
journalctl -u kube-controller-manager.service
journalctl -u kube-proxy.service
journalctl -u kube-scheduler.service

systemctl status etcd.service
systemctl status flannel.service
systemctl status kube-apiserver.service
systemctl status kube-controller-manager.service
systemctl status kube-proxy.service
systemctl status kube-scheduler.service


# Kubeadam
systemctl status kubelet
systemctl status kubeadm
systemctl status kube-apiserver
systemctl status kube-controller-manager
systemctl status cni-dhcp


journalctl -u kube-apiserver --no-pager
journalctl -u kube-controller-manager --no-pager
journalctl -u kubelet --no-pager
journalctl -u kubeadm --no-pager



sudo journalctl -u etcd.service \
-u flannel.service -u kube-apiserver.service -u kube-controller-manager.service -u kube-proxy.service -u kube-scheduler.service


rm -rf /var/lib/kubernetes/ /var/lib/etcd/ /var/lib/cfssl/ /var/lib/kubelet/ /var/lib/kubernetes/secrets/ /etc/kube-flannel/ /etc/kubernetes/

kubectl --server=https://10.1.1.2:6443 --certificate-authority=/var/lib/cfssl/ca.crt --client-certificate=/var/lib/cfssl/admin.crt --client-key=/var/lib/cfssl/admin.key get nodes
```

## References

- [thpham/magics](https://github.com/thpham/magics/blob/master/k8s-cluster/kubernetes/default.nix#L32):  
   NixOS configuration for setting up a Kubernetes cluster using custom Nix expressions.
- [Setting up Kubernetes on NixOS - Stack Overflow](https://stackoverflow.com/questions/49963703/setting-up-kubernetes-on-nixos):  
   A Stack Overflow discussion on challenges and methods for setting up Kubernetes on NixOS.
- [justinas/nixos-ha-kubernetes](https://github.com/justinas/nixos-ha-kubernetes/blob/master/modules/controlplane/apiserver.nix):  
   NixOS module configuration for deploying a high-availability Kubernetes control plane.
- [NixOS/nixpkgs](https://github.com/NixOS/nixpkgs/blob/master/nixos/modules/services/cluster/kubernetes/apiserver.nix):  
   The official NixOS module for managing the Kubernetes API server within the nixpkgs repository.
- [JD95/nixos-configuration](https://github.com/JD95/nixos-configuration/blob/main/containers/kube-master/flake.nix):  
   A Nix flake configuration for setting up a Kubernetes master node on NixOS.
- [kczulko/nixos-config: Config for my nixos setups](https://github.com/kczulko/nixos-config/tree/master):  
   A personal repository containing NixOS configurations, including Kubernetes setup.
- [kczulko/nixos-config](https://github.com/kczulko/nixos-config/blob/master/modules/kubernetes/k8s-dev-single-node.nix):  
   NixOS configuration for deploying a single-node Kubernetes development environment.

- [etcd not init etcd.pem with services.kubernetes.roles master · Issue #59364 · NixOS/nixpkgs](https://github.com/NixOS/nixpkgs/issues/59364)
- [BuildIt-Poland/immutable-web-platform](https://github.com/BuildIt-Poland/immutable-web-platform/blob/375b3c8c51fd289ef5329d5accd1bbc40a78dc23/nix/nixos/modules/kubernetes/kubelet.nix#L69)
- [rydnr/nixos-kubernetes: NixOS modules for Kubernetes](https://github.com/rydnr/nixos-kubernetes/tree/main)
  - Kubernetes from scratch on NixOS

### Kubernetes with Nix sample configurations
- [mynix/homeserver/modules/kubernetes/master.nix at main · ymgyt/mynix](https://github.com/ymgyt/mynix/blob/main/homeserver/modules/kubernetes/master.nix)
- [nixos-configs/nixos/roles/kubevirt-master.nix at master · heywoodlh/nixos-configs](https://github.com/heywoodlh/nixos-configs/blob/master/nixos/roles/kubevirt-master.nix)
- [nixos-configuration/containers/kube-master/flake.nix at main · JD95/nixos-configuration](https://github.com/JD95/nixos-configuration/blob/main/containers/kube-master/flake.nix)
- [Kubernetes: network malfunction after upgrading to 19.09 - Development - NixOS Discourse](https://discourse.nixos.org/t/kubernetes-network-malfunction-after-upgrading-to-19-09/4620/3)


### Kubeadm
- [Kubernetes: network malfunction after upgrading to 19.09 - Development - NixOS Discourse](https://discourse.nixos.org/t/kubernetes-network-malfunction-after-upgrading-to-19-09/4620/6)
- [nixos-configuration/modules/kubeadm/default.nix at master · BenSchZA/nixos-configuration](https://github.com/BenSchZA/nixos-configuration/blob/master/modules/kubeadm/default.nix)
- [nixos-stuff/modules/kubeadm/kubeadm-base.nix at master · arianvp/nixos-stuff](https://github.com/arianvp/nixos-stuff/blob/master/modules/kubeadm/kubeadm-base.nix)