{
  imports = [
    ./hardware-configuration.nix
    ./kubernetes.nix
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
}
