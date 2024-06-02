{ ... }:
{
  imports = [
    ./hardware-configuration.nix
  ];

  services.k3s = {
    enable = true;
    role = "server";
    token = "foo";
    clusterInit = true;
  };
}