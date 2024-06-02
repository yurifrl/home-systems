{ ... }:
{
  imports = [
    ./hardware-configuration.nix
  ];

  services.k3s = {
    enable = true;
    role = "server";
    token = "foo";
    serverAddr = "https://100.94.23.120:6443";
  };
}