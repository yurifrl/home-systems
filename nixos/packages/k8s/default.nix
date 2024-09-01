{
  pkgs,
  lib,
  config,
  ...
}:
let
  # When using easyCerts=true the IP Address must resolve to the master on creation.
  # So use simply 127.0.0.1 in that case. Otherwise you will have errors like this https://github.com/NixOS/nixpkgs/issues/59364
  kubeMasterIP = "127.0.0.1";
  kubeMasterHostname = "api.kube.master";
  kubeMasterAPIServerPort = 6443;

  cfg = config.services.k8s;

  cowsayVersion = pkgs.callPackage ../../packages/cowsay-version.nix { };
in
{

  options.services.k8s = {
    enable = lib.mkEnableOption "k8s";
  };

  config = lib.mkIf cfg.enable {
    networking = {
      useDHCP = false;
      defaultGateway = "192.168.68.1";
      nameservers = [ "192.168.68.1" ];
      interfaces.enp6518.useDHCP = false;
      interfaces.enp6518.ipv4.addresses = [{
        address = "192.168.42.20";
        prefixLength = 24;
      }];
    };
    

    # Kubernetos
    # resolve master hostname
    networking.extraHosts = "${kubeMasterIP} ${kubeMasterHostname}";

    # packages for administration tasks
    environment.systemPackages = with pkgs; [
      kompose
      kubectl
      kubernetes

      cowsayVersion
    ];

    services.kubernetes = {
      roles = ["master" "node"];
      masterAddress = kubeMasterHostname;
      apiserverAddress = "https://${kubeMasterHostname}:${toString kubeMasterAPIServerPort}";
      easyCerts = true;
      apiserver = {
        securePort = kubeMasterAPIServerPort;
        advertiseAddress = kubeMasterIP;
      };

      # use coredns
      addons.dns.enable = true;
    };
  };
}


