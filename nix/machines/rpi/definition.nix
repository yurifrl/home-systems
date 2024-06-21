
{ ... }:

{
  imports = [
    ./hardware-configuration.nix
  ];

  networking = {
    interfaces.eth0 = {
      ipv4.addresses = [
        {
          address = "192.168.68.101";
          prefixLength = 24;
        }
      ];
    };
    defaultGateway = {
      address = "192.168.68.1";
      interface = "eth0";
    };
    firewall.allowedTCPPorts = [
      6443 # k3s: required so that pods can reach the API server (running on port 6443 by default)
    ];
    firewall.allowedUDPPorts = [
      # 8472 # k3s, flannel: required if using multi-node for inter-node networking
    ];
  };

  services.k3s = {
    enable = true;
    role = "server";
    extraFlags = toString [
      # "--kubelet-arg=v=4" # Optionally add additional args to k3s
    ];
  };
}
