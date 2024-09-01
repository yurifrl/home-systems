{ pkgs, lib, ... }:
let
  # Import the unstable nixpkgs channel
  unstablePkgs = import <nixpkgs-unstable> { };
  #
  cowsayVersion = pkgs.callPackage ./packages/cowsay-version.nix { };
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
    nixfmt-rfc-style
    # Unstable packages
    # unstablePkgs.tailscale # https://github.com/NixOS/nixpkgs/blob/master/pkgs/servers/tailscale/default.nix
    # custom packages
    cowsayVersion
    # hs
  ];

  # services.tailscale.authKeyFile = "/etc/tailscale/auth.key";

  services.vscode-server.enable = true;

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

  environment.etc."ssh/ssh_config".text = ''
    Host *
        StrictHostKeyChecking no
  '';
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


  # Needed for vscode
  programs.nix-ld.enable = true;

  console.keyMap = "us";
  time.timeZone = "America/Los_Angeles";

  system.stateVersion = "24.05";
}
