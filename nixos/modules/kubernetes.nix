{ config, lib, pkgs, ... }:

{
    # TODO, test rebuilding and flashing ssd with it
    # Enable required kernel modules for Ceph RBD
    boot.kernelModules = [ "rbd" ];

    # Configure kernel parameters for RBD features
    boot.kernel.sysctl = {
    "net.bridge.bridge-nf-call-iptables" = 1;
    "net.bridge.bridge-nf-call-ip6tables" = 1;
    };
}
