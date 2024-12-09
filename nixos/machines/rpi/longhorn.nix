{ config, lib, pkgs, ... }:

{
    # ============================================================================
    # Longhorn Storage Requirements
    # https://github.com/longhorn/longhorn/issues/2166
    # ============================================================================
    
    # Add symlink for standard paths that Longhorn expects
    systemd.tmpfiles.rules = [
        "L+ /usr/local/bin - - - - /run/current-system/sw/bin/"
    ];

    # Enable required kernel modules
    boot.kernelModules = [
        "dm_snapshot"
        "dm_mirror"
        "dm_thin_pool"
        "dm_crypt"
    ];

    # Enable and configure iSCSI service
    services.openiscsi = {
        enable = true;
        name = "iqn.2024-01.org.nixos:${config.networking.hostName}";
    };

    # Configure Docker to use json-file logging instead of journald
    virtualisation.docker = {
        enable = true;
        logDriver = "json-file";
    };

    # Add NFS kernel module support
    boot.supportedFilesystems = [ "nfs" "nfs4" ];

    # Install NFS utilities
    environment.systemPackages = with pkgs; [
        nfs-utils
    ];
}
