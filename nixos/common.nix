{ pkgs, lib, pkgs-unstable, ... }:
let
  # My packages
  cowsayVersion = pkgs.callPackage ./packages/cowsay-version.nix { };
in
{
  # Import tailscale module
  imports = [
    ./modules/tailscale.nix
    ./modules/nix-status-check.nix
  ];

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
    istioctl
  ] ++ (with pkgs-unstable; [
    # Unstable packages
    kubectl
    cloudflared
    kubernetes-helm
  ]) ++ [
    # Custom packages
    cowsayVersion
    # hs
  ];

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
    # shell = pkgs.fish; 
  };
  # programs.fish.enable = true;

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

  # Set vim as default editor
  environment.variables.EDITOR = "vim";

  system.stateVersion = "24.05";

  # Enable fish shell
  programs.fish = {
    enable = true;
    shellAliases = {
      k = "kubectl";
    };
  };
}
