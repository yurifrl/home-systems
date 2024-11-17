{
  imports = [
    ./hardware-configuration.nix
  ];
  networking = {
    nameservers = [
      "8.8.8.8"
      "8.8.4.4"
    ];
    firewall.enable = false;
    interfaces.eth0.useDHCP = true;
    hostName = "nixos-1";
  };

  # https://github.com/rancher/rancher/issues/38849
  services.k3s = {
    enable = true;
    role = "server";
    extraFlags = [
      "--disable=traefik"
      "--flannel-backend=host-gw"
      "--tls-san=192.168.68.100"
      "--bind-address=192.168.68.100"
      "--advertise-address=192.168.68.100"
      "--node-ip=192.168.68.100"
      "--cluster-init"
    ];
  };

  environment.variables = {
    KUBECONFIG = "/etc/rancher/k3s/k3s.yaml";
  };
}
