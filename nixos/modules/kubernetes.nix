{ config, lib, pkgs, ... }:

{
    # ============================================================================
    # General Kubernetes Configuration
    # ============================================================================
    
    # Configure kernel parameters for networking
    boot.kernel.sysctl = {
      "net.bridge.bridge-nf-call-iptables" = 1;
      "net.bridge.bridge-nf-call-ip6tables" = 1;
    };
}
