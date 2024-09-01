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

  services.k3s = {
    enable = true;
    role = "server";
  };
}
