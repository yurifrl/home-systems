{
  imports = [
    ./hardware-configuration.nix
    ./kubernetes-configuration.nix
  ];
  networking = {
    nameservers = [
      "8.8.8.8"
      "8.8.4.4"
    ];
    firewall.enable = false;
    interfaces.eth0.useDHCP = true;

    defaultGateway = {
      address = "192.168.68.107";
      interface = "eth0";
    };
  };
  ## nucles/default.nix copied from that repo
  networking.hostName = "nucle1";

  services.kubeadm.init = {
    enable = true;
    bootstrapTokenFile = "/var/secret/kubeadm-bootstrap-token";
  };

  # System config copied from that repo
  networking.domain = "localdomain";
  networking.useNetworkd = true;

  systemd.network.enable = true;
  systemd.network.networks.lan.name = "en*";
  systemd.network.networks.lan.DHCP = "yes";
  systemd.network.wait-online.anyInterface = true;
  services.resolved.dnssec = "false";

  security.sudo.wheelNeedsPassword = false;

  nix.gc.automatic = true;
  nix.gc.options = "--delete-older-than 14d";
}
