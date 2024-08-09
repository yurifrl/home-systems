
{ ... }:

{
  imports = [
    ./hardware-configuration.nix
    # ./kubernetes.nix
  ];

  networking = {
    interfaces.eth0 = {
      ipv4.addresses = [
        {
          address = "192.168.68.102";
          prefixLength = 24;
        }
      ];
    };
    defaultGateway = {
      address = "192.168.68.1";
      interface = "eth0";
    };
  };
}
