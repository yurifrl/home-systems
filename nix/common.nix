{ pkgs, lib, ... }:

let
  unstablePkgs = import <nixpkgs-unstable> { };
  cowsayVersion = pkgs.callPackage ./packages/cowsay-version.nix {};
  hs = pkgs.callPackage ./packages/hs.nix {};
in
{
  environment.systemPackages = with pkgs; [
    libraspberrypi
    raspberrypi-eeprom
    vim
    curl
    htop
    jq
    inetutils
    git
    fish
    cowsayVersion
    hs
    unstablePkgs.tailscale
  ];

  networking = {
    nameservers = [ "8.8.8.8" "8.8.4.4" ];
    interfaces.eth0.useDHCP = false;
    interfaces.eth0.ipv4.addresses = [ { address = "192.168.68.102"; prefixLength = 24; } ];
    defaultGateway.address = "192.168.68.1";
    defaultGateway.interface = "eth0";
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

  users.extraUsers.nixos = {
    isNormalUser = true;
    group = "nixos";
    openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAvaTuBhwuQHdjIP1k9YQk9YMqmGiOate19iXe6T4IL/"
    ];
  };

  users.groups.nixos = { };

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

  system.stateVersion = "23.05";
}
