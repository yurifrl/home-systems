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
    ];
  };

  environment.variables = {
    KUBECONFIG = "/etc/rancher/k3s/k3s.yaml";
  };
}
