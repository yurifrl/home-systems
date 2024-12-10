{ config, lib, pkgs, ... }:

{
    # ============================================================================
    # Longhorn Storage Requirements
    # https://github.com/longhorn/longhorn/issues/2166
    # ============================================================================
    
    # Configure tmpfiles rules for required directories and symlinks
    systemd.tmpfiles.rules = [
        # Add symlink for standard paths that Longhorn expects
        "L+ /usr/local/bin - - - - /run/current-system/sw/bin/"
        # Ensure storage directories exist with correct permissions
        "d /var/lib/longhorn 0755 root root -"
        "d /var/lib/longhorn/engine 0755 root root -"
        "d /var/lib/longhorn/replica 0755 root root -"
    ];

    # Enable required kernel modules
    boot.kernelModules = [
        "dm_snapshot"
        "dm_mirror"
        "dm_thin_pool"
        "dm_crypt"
        "nfs"
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

    # Enable rpcbind which is required for NFS
    services.rpcbind.enable = true;

    # Configure NFS server
    services.nfs.server = {
        enable = true;
        # Fixed ports for firewall configuration
        lockdPort = 4001;
        mountdPort = 4002;
        statdPort = 4000;
        # Export configuration
        exports = ''
          /var/lib/longhorn *(rw,no_root_squash,no_subtree_check,insecure)
        '';
    };

    # Install NFS utilities
    environment.systemPackages = with pkgs; [
        nfs-utils
    ];
}
