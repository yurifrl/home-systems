{
  imports = [
    ./hardware-configuration.nix
    ./cluster-bootstrap.nix
  ];
  networking = {
    nameservers = [
      "8.8.8.8"
      "8.8.4.4"
    ];
    firewall.enable = false;
    interfaces.eth0 = {
      useDHCP = false;
      ipv4.addresses = [{
        address = "192.168.68.100";
        prefixLength = 24;
      }];
    };
    defaultGateway = "192.168.68.1";
    hostName = "nixos-1";
  };

  # https://github.com/rancher/rancher/issues/38849
  services.k3s = {
    enable = true;
    role = "server";
    extraFlags = [
      "--disable=traefik"
      "--flannel-backend=host-gw"
      # Static IP address for the k3s server node
      # Using 192.168.68.100 as a reserved static IP in the local network
      # This ensures stable networking for the k3s control plane
      "--tls-san=192.168.68.100"
      "--bind-address=0.0.0.0"
      "--advertise-address=192.168.68.100"
      "--node-ip=192.168.68.100"
      "--cluster-init"
    ];
  };

  environment.variables = {
    KUBECONFIG = "/etc/rancher/k3s/k3s.yaml";
  };
}
