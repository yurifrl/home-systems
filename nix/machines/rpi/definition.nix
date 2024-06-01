{ ... }:
{
  imports = [
    ./hardware-configuration.nix
  ];

  sdImage.compressImage = false; # If true, will build a .zst compressed image.
  # sdImage.enable = true; # What does this do?
}