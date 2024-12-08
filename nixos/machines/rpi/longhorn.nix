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

    # Add required packages to system environment
    environment.systemPackages = with pkgs; [
      openiscsi
      nfs-utils
      util-linux
      lsb-release    # Add LSB release information
      systemd        # Ensure systemd utilities are available
    ];

    # NixOS-specific fixes for Longhorn compatibility
    virtualisation.docker.logDriver = "json-file";

    # Required system utilities for Longhorn operations
    systemd.services.kubelet = {
      path = [ 
        pkgs.bash 
        pkgs.openiscsi 
        pkgs.nfs-utils
        pkgs.util-linux
        pkgs.gnugrep
        pkgs.gawk
        pkgs.lsb-release
        pkgs.coreutils
      ];
    };

    # Ensure OS release information is available
    environment.etc."os-release".text = ''
      NAME="NixOS"
      ID=nixos
      VERSION="${config.system.stateVersion}"
      VERSION_ID="${config.system.stateVersion}"
      PRETTY_NAME="NixOS ${config.system.stateVersion}"
      HOME_URL="https://nixos.org/"
    '';

    # Ensure iSCSI service starts before Longhorn
    systemd.services."longhorn-manager" = {
      after = [ "iscsid.service" ];
      requires = [ "iscsid.service" ];
      environment = {
        OS_RELEASE_PATH = "/etc/os-release";
      };
    };
}
