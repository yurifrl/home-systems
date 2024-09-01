{
  imports = [
    ./hardware-configuration.nix
    ../../packages/k8s
  ];
  networking = {
    nameservers = [
      "8.8.8.8"
      "8.8.4.4"
    ];
    firewall.enable = false;
    interfaces.eth0.useDHCP = true;

    # defaultGateway = {
    #   address = "192.168.68.107";
    #   interface = "eth0";
    # };
  };
  networking.hostName = "nixos";

  services.k8s = {
    enable = true;
  };
}
