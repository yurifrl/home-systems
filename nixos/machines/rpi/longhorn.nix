{ config, lib, pkgs, ... }:

{
    # ============================================================================
    # Longhorn Storage Requirements
    # https://github.com/longhorn/longhorn/issues/2166
    # ============================================================================
    
    # Required packages for Longhorn
    environment.systemPackages = with pkgs; [
        open-iscsi
        nfs-utils
        cryptsetup
    ];

    # Enable required kernel modules
    boot.kernelModules = [
        "dm_crypt"    # For encryption support
        "iscsi_tcp"   # For iSCSI support
    ];

    # Enable required services
    services = {
        open-iscsi = {
            enable = true;
            name = "iqn.2024-01.org.nixos:${config.networking.hostName}";
        };
        nfs.server.enable = true;
    };

    # Ensure mount points exist
    systemd.tmpfiles.rules = [
        "d /var/lib/longhorn 0700 root root -"
        "d /host/etc 0755 root root -"
        "L+ /host/etc/os-release - - - - /etc/os-release"
    ];
}
