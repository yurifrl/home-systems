# nix/common.nix
{ pkgs, lib, ... }:
let
  # Define the script as a variable
  version = pkgs.writeShellScriptBin "version" ''
    #!/bin/sh
    echo "System Version: 10.0"
  '';

  # hs = import ./packages/hs.nix { inherit pkgs; };

in
{
  # System packages
  environment.systemPackages = with pkgs; [
    libraspberrypi
    raspberrypi-eeprom

    vim
    curl
    htop
    cowsay
    hello
    fortune
    jq 

    version   
    # hs
  ];

  system = {
    stateVersion = "23.05";
  };

  # Networking configuration
  networking = {
    nameservers = [ "8.8.8.8" "8.8.4.4" ];
    firewall.enable = false;
    interfaces.eth0.useDHCP = true;
  };

  services.openssh = {
    enable = true;
    settings = {
      # PermitRootLogin = lib.mkForce "prohibit-password";
      PasswordAuthentication = false;
      KbdInteractiveAuthentication = false;
      ChallengeResponseAuthentication = false;
    };
    extraConfig = "Compression no";
  };

  # SSH authorized keys for user 'nixos'
  users.extraUsers.nixos = {
    isNormalUser = true;
    group = "nixos";
    openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAvaTuBhwuQHdjIP1k9YQk9YMqmGiOate19iXe6T4IL/"
    ];
  };

  users.groups.nixos = {};

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
  
  console.keyMap = "us";
  time.timeZone = "America/Los_Angeles";
}
