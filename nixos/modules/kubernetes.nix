{ config, lib, pkgs, ... }:

{
    # ============================================================================
    # General Kubernetes Configuration
    # ============================================================================
    
    # # Enable required kernel modules
    # boot.kernelModules = [
    #     "overlay"
    #     "br_netfilter"
    #     "nf_conntrack"
    #     "iptable_nat"
    #     "iptable_filter"
    # ];

    # # Configure kernel parameters for networking
    # boot.kernel.sysctl = {
    #     "net.ipv4.ip_forward" = 1;
    #     "net.bridge.bridge-nf-call-iptables" = 1;
    #     "net.bridge.bridge-nf-call-ip6tables" = 1;
    # };

    # Configure kernel parameters for networking
    boot.kernel.sysctl = {
      "net.bridge.bridge-nf-call-iptables" = 1;
      "net.bridge.bridge-nf-call-ip6tables" = 1;
    };
}
