{ pkgs, lib, ... }:
let
  # Import the unstable nixpkgs channel
  unstablePkgs = import <nixpkgs-unstable> { };
  #
  cowsayVersion = pkgs.callPackage ./packages/cowsay-version.nix {};
  hs = pkgs.callPackage ./packages/hs.nix {};
in
{
  # System packages
  environment.systemPackages = with pkgs; [
    # Raspberry Pi packages
    libraspberrypi
    raspberrypi-eeprom
    # Basic packages
    vim
    curl
    htop
    jq
    inetutils
    git
    fish
    # custom packages
    cowsayVersion
    hs
    # Unstable packages
    unstablePkgs.tailscale # https://github.com/NixOS/nixpkgs/blob/master/pkgs/servers/tailscale/default.nix
  ];

  # Networking configuration
  networking = {
    nameservers = [ "8.8.8.8" "8.8.4.4" ];
    firewall.enable = false;
    interfaces.eth0.useDHCP = false;
  };

  services.openssh = {
    enable = true;
    ports = [ 22 ];
    settings = {
      PasswordAuthentication = false;
      KbdInteractiveAuthentication = false;
      ChallengeResponseAuthentication = false;
      AllowUsers = null;
      UseDns = true;
      # PermitRootLogin = lib.mkForce "prohibit-password";
      PermitRootLogin = "prohibit-password"; # "yes", "without-password", "prohibit-password", "forced-commands-only", "no"
  };
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

  nix.extraOptions = ''
    experimental-features = nix-command flakes
  '';

  console.keyMap = "us";
  time.timeZone = "America/Los_Angeles";

  system = {
    stateVersion = "23.05";
  };
}

