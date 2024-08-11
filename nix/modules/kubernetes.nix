{ pkgs, ... }:
let
  # When using easyCerts=true the IP Address must resolve to the master on creation.
 # So use simply 127.0.0.1 in that case. Otherwise you will have errors like this https://github.com/NixOS/nixpkgs/issues/59364
  # kubeMasterIP = "10.1.1.2";
  # kubeMasterHostname = "api.kube";
  # kubeMasterAPIServerPort = 6443;

  unstablePkgs = import <nixpkgs-unstable> { };


  kubeMasterIP = "127.0.0.1";
  kubeMasterHostname = "kube-master";
  kubeMasterAPIServerPort = 6443;
in
{
  # packages for administration tasks
  environment.systemPackages = with pkgs; [
    kompose
    kubectl
    unstablePkgs.kubernetes
    # unstablePkgs.containerd
  ];

  # resolve master hostname
  networking.extraHosts = "${kubeMasterIP} ${kubeMasterHostname}";

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

    # needed if you use swap
    kubelet.extraOpts = "--fail-swap-on=false";
  };
}