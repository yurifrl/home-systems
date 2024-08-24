
{ ... }:

{
  imports = [
    ./hardware-configuration.nix
    ./kubernetes.nix
    ./disko-config.nix
  ];


  networking = {
    nameservers = [ "8.8.8.8" "8.8.4.4" ];
    firewall.enable = false;
    interfaces.eth0.useDHCP = true;
    # interfaces.eth0 = {
    #   ipv4.addresses = [
    #     {
    #       address = "192.168.68.105";
    #       prefixLength = 24;
    #     }
    #   ];
    # };
    defaultGateway = {
      address = "192.168.68.1";
      interface = "eth0";
    };
  };
}
