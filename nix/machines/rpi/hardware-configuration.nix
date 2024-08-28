{
  pkgs,
  config,
  lib,
  ...
}:
{
  # This causes an overlay which causes a lot of rebuilding
  environment.noXlibs = lib.mkForce false;
  # "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64.nix" creates a
  # disk with this label on first boot. Therefore, we need to keep it. It is the
  # only information from the installer image that we need to keep persistent
  # Does notthing
  fileSystems."/" = {
    device = "/dev/disk/by-label/NIXOS_SD";
    fsType = "ext4";
  };
  # # Adding a Btrfs partition
  # fileSystems."/mnt/btrfs" = {
  #   device = "/dev/disk/by-label/MY_BTRFS_LABEL";
  #   fsType = "btrfs";
  #   options = [
  #     "noatime"
  #     "compress=zstd"
  #   ];
  # };
  boot = {
    kernelPackages = lib.mkForce pkgs.linuxPackages_latest;
    loader = {
      generic-extlinux-compatible.enable = lib.mkDefault true;
      grub.enable = lib.mkDefault false;
    };
  };
  nix.settings = {
    experimental-features = lib.mkDefault "nix-command flakes";
    trusted-users = [
      "root"
      "@wheel"
    ];
  };
}
