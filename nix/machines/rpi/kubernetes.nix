{
  # Enable Docker
  virtualisation.docker.enable = true;

  # Install kubeadm, kubectl, and kubelet
  environment.systemPackages = with pkgs; [
    kubernetes
    kubernetes-helm
  ];

  # Enable kubelet service
  services.kubernetes.roles = [
    "master"
    "node"
  ];
}
