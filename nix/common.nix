{ pkgs, lib, ... }:
let
  # Import the unstable nixpkgs channel
  # unstablePkgs = import <nixpkgs-unstable> { };
  #
  cowsayVersion = pkgs.callPackage ./packages/cowsay-version.nix {};
  # hs = pkgs.callPackage ./packages/hs.nix {};
in
{
  # System packages
  environment.systemPackages = with pkgs; [
    # Raspberry Pi packages
    libraspberrypi
    raspberrypi-eeprom
    # Basic packages
    vim
    neovim
    curl
    htop
    jq
    inetutils
    git
    fish
    # Unstable packages
    # unstablePkgs.tailscale # https://github.com/NixOS/nixpkgs/blob/master/pkgs/servers/tailscale/default.nix
    # custom packages
    cowsayVersion
    # hs
  ];

  environment.etc."ssh/ssh_config".text = ''
    Host *
        StrictHostKeyChecking no
  '';

  # Networking configuration
  networking = {
    nameservers = [ "8.8.8.8" "8.8.4.4" ];
    firewall.enable = false;
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
  users.extraUsers.nixos = {
    isNormalUser = true;
    group = "nixos";
    openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAvaTuBhwuQHdjIP1k9YQk9YMqmGiOate19iXe6T4IL/"
    ];
  };

  users.users.root.openssh.authorizedKeys.keys = [
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAvaTuBhwuQHdjIP1k9YQk9YMqmGiOate19iXe6T4IL/"
  ];
  
  users.groups.nixos = { };

  # Systemd service configuration for OpenSSH
  systemd.services.sshd.wantedBy = lib.mkOverride 40 [ "multi-user.target" ];

  security.sudo = {
    enable = true;
    wheelNeedsPassword = false;
    extraConfig = ''
      nixos ALL=(ALL) NOPASSWD: ALL
    '';
  };

  nix = {
    extraOptions = ''
      experimental-features = nix-command flakes
    '';

    nixPath = [
      "nixpkgs=https://nixos.org/channels/nixpkgs-unstable"
      "nixos-config=/etc/nixos/configuration.nix"
    ];
  };

  console.keyMap = "us";
  time.timeZone = "America/Los_Angeles";

  system = {
    stateVersion = "23.05";
  };
}

