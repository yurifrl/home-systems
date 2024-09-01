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
    hostName = "nixos";
  };

  services.k3s = {
    services.k3s.enable = true;
    services.k3s.role = "server";
  };
}
