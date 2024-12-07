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
}
