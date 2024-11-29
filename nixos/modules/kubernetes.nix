{ config, lib, pkgs, ... }:

{
  # Enable required kernel modules for Ceph RBD
  boot.kernelModules = [ "rbd" ];
  
  # Configure kernel parameters for RBD features
  boot.kernel.sysctl = {
    "net.bridge.bridge-nf-call-iptables" = 1;
    "net.bridge.bridge-nf-call-ip6tables" = 1;
  };

  # Install required packages
  environment.systemPackages = with pkgs; [
    lvm2  # Required for Rook Ceph
  ];
}
