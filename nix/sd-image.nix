{ config, pkgs, ... }:

{
  boot.loader.generic-extlinux-compatible.enable = true;
  boot.loader.generic-extlinux-compatible.useGenImage = true;
  boot.kernelPackages = pkgs.linuxPackages_rpi4;

  fileSystems."/" = {
    device = "/dev/disk/by-label/NIXOS_SD";
    fsType = "ext4";
  };

  # Example configurations
  networking.hostName = "rpi4";
  time.timeZone = "UTC";
  services.openssh.enable = true;

  users.users.root = {
    password = "root";  # Replace with a secure password
  };
}
