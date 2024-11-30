{ pkgs, lib, pkgs-unstable, ... }:
let
  cowsayVersion = pkgs.callPackage ./packages/cowsay-version.nix { };
in
{
  # Module imports (always first)
  imports = [
    ./modules/tailscale.nix
    ./modules/nix-status-check.nix
  ];

  # Package management (frequently updated)
  environment.systemPackages = with pkgs; [
    # Raspberry Pi specific
    libraspberrypi 
    raspberrypi-eeprom
    
    # System utilities
    vim neovim curl htop jq 
    inetutils git fish 
    nixfmt-rfc-style parted

    # Packages
    istioctl
    # Custom packages
    cowsayVersion 
  ] ++ (with pkgs-unstable; [
    # Kubernetes tools
    kubectl kubernetes-helm cloudflared
  ]);

  # Shell configuration (frequently modified)
  programs.fish = {
    enable = true;
    shellAliases = {
      k = "kubectl";
      snixos = "cd /home/nixos/home-systems/nixos && git pull origin main && sudo nixos-rebuild switch --flake .#rpi --impure --show-trace";
      argo-restart = "cd /home/nixos/home-systems/nixos && git pull origin main & sudo systemctl restart argo-setup & sudo journalctl -u argo-setup.service -f";
    };
  };

  # User management (occasionally updated)
  users = {
    groups.nixos = { };
    extraUsers.nixos = {
      isNormalUser = true;
      group = "nixos";
      openssh.authorizedKeys.keys = [
        "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAvaTuBhwuQHdjIP1k9YQk9YMqmGiOate19iXe6T4IL/"
      ];
    };
    users.root.openssh.authorizedKeys.keys = [
      "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAvaTuBhwuQHdjIP1k9YQk9YMqmGiOate19iXe6T4IL/"
    ];
  };

  # SSH configuration (occasional security updates)
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
  environment.etc."ssh/ssh_config".text = ''
    Host *
        StrictHostKeyChecking no
  '';
  systemd.services.sshd.wantedBy = lib.mkOverride 40 [ "multi-user.target" ];

  # Additional services (rarely changed)
  services.vscode-server.enable = true;

  # Security configuration (rarely changed)
  security.sudo = {
    enable = true;
    wheelNeedsPassword = false;
    extraConfig = ''
      nixos ALL=(ALL) NOPASSWD: ALL
    '';
  };

  # Nix configuration (rarely changed)
  nix = {
    extraOptions = ''
      experimental-features = nix-command flakes
    '';
    nixPath = [
      "nixpkgs=https://nixos.org/channels/nixpkgs-unstable"
      "nixos-config=/etc/nixos/configuration.nix"
    ];
  };
  programs.nix-ld.enable = true;

  # Base system configuration (rarely changed)
  console.keyMap = "us";
  time.timeZone = "America/Los_Angeles";
  system.stateVersion = "24.05";
  environment.variables.EDITOR = "vim";
}
