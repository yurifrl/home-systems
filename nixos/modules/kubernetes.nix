{ config, lib, pkgs, ... }:

{
    # ============================================================================
    # Longhorn Storage Requirements
    # ============================================================================
    
    # Kernel modules required by Longhorn for storage operations
    boot.kernelModules = [ 
      "iscsi_tcp"  # For Longhorn's iSCSI support
      "dm_snapshot" 
      "dm_mirror" 
      "dm_thin_pool"
      "nfs"        # For NFS support
      "nfs_v4"     # For NFSv4 support
    ];

    # Enable and configure iSCSI initiator for Longhorn
    services.openiscsi = {
      enable = true;
      name = "iqn.2024-01.org.nixos.initiator:${config.networking.hostName}";
    };

    # NixOS-specific fixes for Longhorn compatibility
    systemd.tmpfiles.rules = [
      "L+ /usr/local/bin - - - - /run/current-system/sw/bin/"  # Fix binary path resolution
    ];
    virtualisation.docker.logDriver = "json-file";  # Required for proper logging

    # Required system utilities for Longhorn operations
    systemd.services.kubelet = {
      path = [ 
        pkgs.bash 
        pkgs.openiscsi 
        pkgs.nfs-utils
        pkgs.util-linux
        pkgs.gnugrep
        pkgs.gawk
      ];
    };

    # ============================================================================
    # General Kubernetes Configuration
    # ============================================================================
    
    # Configure kernel parameters for networking
    boot.kernel.sysctl = {
      "net.bridge.bridge-nf-call-iptables" = 1;
      "net.bridge.bridge-nf-call-ip6tables" = 1;
    };

    # Create kubernetes user and group
    users.groups.kubernetes = {};

    users.users.kubernetes = {
        isSystemUser = true;
        group = "kubernetes";
        description = "Kubernetes system user";
        home = "/var/lib/kubernetes";
        createHome = true;
        uid = 900;
    };
}
