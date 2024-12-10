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
          /storage         *(rw,fsid=0,no_subtree_check,no_root_squash)
          /var/lib/longhorn *(rw,no_root_squash,no_subtree_check,insecure)
        '';
    };

    # Open required firewall ports for NFSv3 and NFSv4
    networking.firewall = {
        allowedTCPPorts = [ 111 2049 4000 4001 4002 20048 ];
        allowedUDPPorts = [ 111 2049 4000 4001 4002 20048 ];
    };

    # Install NFS utilities
    environment.systemPackages = with pkgs; [
        nfs-utils
    ];

    # Ensure storage directory exists and has correct permissions
    systemd.tmpfiles.rules = [
        "d /storage 0755 root root -"
        "d /var/lib/longhorn 0755 root root -"
    ];
}
