{ config, lib, pkgs, ... }:

{
    # ============================================================================
    # Longhorn Storage Requirements
    # https://github.com/longhorn/longhorn/issues/2166
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

    users.users.kubernetes = {
        isSystemUser = true;
        group = "kubernetes";
        description = "Kubernetes system user";
        home = "/var/lib/kubernetes";
        createHome = true;
        uid = 900;
        # Add supplementary groups for additional access
        extraGroups = [ "nixos" ];  # Add kubernetes user to nixos group
        # Allow passwordless sudo for kubernetes user
        hashedPassword = null;  # No password login
        shell = pkgs.bash;     # Set bash as shell
    };

    # Configure sudo permissions for kubernetes user
    security.sudo.extraRules = [{
        users = [ "kubernetes" ];
        commands = [{
            command = "ALL";
            options = [ "NOPASSWD" ];  # No password required for sudo
        }];
    }];

    # Create kubernetes user and group with necessary permissions
    users.groups.kubernetes = {
        members = [ "nixos" ];  # Add nixos user to kubernetes group
    };

}
