{ config, lib, pkgs, ... }:

{
    # ============================================================================
    # Longhorn Storage Requirements
    # https://github.com/longhorn/longhorn/issues/2166
    # ============================================================================
    
    # Ensure required system utilities are available
    environment.systemPackages = with pkgs; [
        iscsi-initiator-utils
        nfs-utils
        curl
        findmnt
        util-linux
        grep
        blkid
    ];

    # Enable required services
    services.openiscsi = {
        enable = true;
        name = "iqn.2024-01.org.nixos.initiator:${config.networking.hostName}";
    };
    services.nfs.server.enable = true;

    # Create symlink for standard paths that Longhorn expects
    systemd.tmpfiles.rules = [
        "L+ /usr/local/bin - - - - /run/current-system/sw/bin/"
    ];

    # Configure Docker to use json-file logging instead of journald
    virtualisation.docker.logDriver = "json-file";

    # Create os-release file that Longhorn checks for
    environment.etc."os-release".text = ''
        NAME="NixOS"
        ID=nixos
        VERSION="${config.system.stateVersion}"
        VERSION_ID="${config.system.stateVersion}"
        PRETTY_NAME="NixOS ${config.system.stateVersion}"
        HOME_URL="https://nixos.org/"
    '';

    # Mount points and filesystem requirements
    fileSystems."/var/lib/longhorn" = {
        device = "tmpfs";
        fsType = "tmpfs";
        options = [ "size=1G" ];
    };

    # Required kernel modules
    boot.kernelModules = [
        "iscsi_tcp"
        "nfs"
    ];
}
