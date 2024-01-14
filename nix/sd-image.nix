{ lib, pkgs, ... }:
let
  # Define the script as a variable
  showVersionScript = pkgs.writeShellScriptBin "version" ''
    #!/bin/sh
    echo "Custom Version: 3.0"
  '';
in
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
    vim
    curl
    htop
    cowsay
    hello
    fortune

    showVersionScript
  ];

  # Networking configuration
  networking = {
    useDHCP = false;
    interfaces.eth0.useDHCP = true;
  };

  services = {
    # SSH service configuration
    openssh = {
      enable = true;
      settings.PermitRootLogin = "yes";
    };
    # NTP time synchronization
    timesyncd.enable = true;
  };


  # SSH authorized keys for user 'nixos'
  users.extraUsers.nixos.openssh.authorizedKeys.keys = [
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPCfAwxYdoLR6YzoIx2+L593yLGpHaseGTCm3fxrshgD yurifl03@gmail.com"
  ];

  # Systemd service configuration for OpenSSH
  systemd.services.sshd.wantedBy = lib.mkOverride 40 [ "multi-user.target" ];

  # Define system architecture for ARM
  nixpkgs.localSystem = {
    system = "aarch64-linux";
    config = "aarch64-unknown-linux-gnu";
  };


  security.sudo = {
    enable = true;
    wheelNeedsPassword = false;
    extraConfig = ''
      nixos ALL=(ALL) NOPASSWD: ALL
    '';
  };
}
