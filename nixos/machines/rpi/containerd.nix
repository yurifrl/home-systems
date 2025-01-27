{ config, lib, pkgs, ... }:

{
  # Enable required kernel modules
  boot.kernelModules = [
    "overlay"
    "br_netfilter"
    "nf_conntrack"
    "iptable_nat"
    "iptable_filter"
  ];

  # Configure kernel parameters
  boot.kernel.sysctl = {
    "net.ipv4.ip_forward" = 1;
    "net.bridge.bridge-nf-call-iptables" = 1;
    "net.bridge.bridge-nf-call-ip6tables" = 1;
  };

  # Add proper capabilities and directories for k3s
  systemd.services.k3s = {
    serviceConfig = {
      AmbientCapabilities = [
        "CAP_NET_BIND_SERVICE"
        "CAP_NET_RAW"
        "CAP_SYS_ADMIN"
      ];
      RuntimeDirectory = "k3s/containerd";
      RuntimeDirectoryMode = "0755";
    };
  };
}