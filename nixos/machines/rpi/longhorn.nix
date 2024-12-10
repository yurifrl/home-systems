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
        # Add NFS modules explicitly
        "nfs"
        "nfsv4"
        "nfs_acl"
        "lockd"
        "sunrpc"
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

    # Add NFS kernel module support with explicit version support
    boot.supportedFilesystems = [ "nfs" "nfs4" ];
    boot.kernel.sysctl = {
        "fs.nfs.nlm_timeout" = 10;
        "fs.nfs.nlm_udpport" = 32768;
        "fs.nfs.nlm_tcpport" = 32768;
    };

    # Install NFS utilities and additional required packages
    environment.systemPackages = with pkgs; [
        nfs-utils
        nfs-utils.out
        kmod # For modprobe
    ];

    # Enable NFS server and client services
    services.nfs.server = {
        enable = true;
        exports = "";  # Configure if needed
    };
    services.nfs.client = {
        enable = true;
        nfsv4 = true;
    };
}
