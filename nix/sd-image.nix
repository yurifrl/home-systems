{ lib, pkgs, ... }:
let
  # Define the script as a variable
  showVersionScript = pkgs.writeShellScriptBin "version" ''
    #!/bin/sh
    echo "Custom Version: 4.0"
  '';
in
{
  imports = [
    # Import SD card installer modules
    <nixpkgs/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix>
    # For nixpkgs cache
    <nixpkgs/nixos/modules/installer/cd-dvd/channel.nix>
    #
    ./rpi4-hardware-configuration.nix
    ./tailscale.nix
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
    jq

    showVersionScript
  ];

  # Networking configuration
  networking = {
    useDHCP = false;
    interfaces.eth0.useDHCP = true;
  };

  services.openssh = {
    enable = true;
    settings = {
      PermitRootLogin = lib.mkForce "prohibit-password";
      PasswordAuthentication = false;
      KbdInteractiveAuthentication = false;
      ChallengeResponseAuthentication = false;
    };
    extraConfig = "Compression no";
  };

  # SSH authorized keys for user 'nixos'
  users.extraUsers.nixos.openssh.authorizedKeys.keys = [
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAvaTuBhwuQHdjIP1k9YQk9YMqmGiOate19iXe6T4IL/"
  ];

  # Systemd service configuration for OpenSSH
  systemd.services.sshd.wantedBy = lib.mkOverride 40 [ "multi-user.target" ];

  security.sudo = {
    enable = true;
    wheelNeedsPassword = false;
    extraConfig = ''
      nixos ALL=(ALL) NOPASSWD: ALL
    '';
  };

  nix.extraOptions = ''
    experimental-features = nix-command flakes
  '';
}
