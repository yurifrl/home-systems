
# Troubleshooting


```bash
journalctl -u etcd.service
journalctl -u flannel.service
journalctl -u kube-apiserver.service
journalctl -u kube-controller-manager.service
journalctl -u kube-proxy.service
journalctl -u kube-scheduler.service

sudo journalctl -u etcd.service \
-u flannel.service -u kube-apiserver.service -u kube-controller-manager.service -u kube-proxy.service -u kube-scheduler.service

systemctl status etcd.service && \
systemctl status flannel.service && \
systemctl status kube-apiserver.service && \
systemctl status kube-controller-manager.service && \
systemctl status kube-proxy.service && \
systemctl status kube-scheduler.service

rm -rf /var/lib/kubernetes/ /var/lib/etcd/ /var/lib/cfssl/ /var/lib/kubelet/ /var/lib/kubernetes/secrets/ /etc/kube-flannel/ /etc/kubernetes/

kubectl --server=https://10.1.1.2:6443 --certificate-authority=/var/lib/cfssl/ca.crt --client-certificate=/var/lib/cfssl/admin.crt --client-key=/var/lib/cfssl/admin.key get nodes
```

# References

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