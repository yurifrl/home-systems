{ lib, pkgs, ... }:
{
  imports = [
    # Import SD card installer modules
    <nixpkgs/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix>
    # For nixpkgs cache
    <nixpkgs/nixos/modules/installer/cd-dvd/channel.nix>
  ];

  # Configuration options
  sdImage.compressImage = false; # If true, will build a .zst compressed image.
  # sdImage.enable = true; # What does this do?
  system.stateVersion = "23.05"; # Define the NixOS version

  # System packages
  environment.systemPackages = with pkgs; [
    hello
  ];
}
