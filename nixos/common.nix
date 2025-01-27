{ pkgs, lib, pkgs-unstable, ... }:
let
  cowsayVersion = pkgs.callPackage ./packages/cowsay-version.nix { };
  diskTemplate = pkgs.callPackage ./packages/disk-template.nix { };
  sbcExporter = pkgs.callPackage ./packages/sbc-exporter.nix { };
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
    vim neovim curl jq 
    inetutils git fish sysz argocd
    nixfmt-rfc-style parted rsync

    # Packages
    istioctl
    # Custom packages
    cowsayVersion 
    diskTemplate
    sbcExporter
  ] ++ (with pkgs-unstable; [
    # Kubernetes tools
    kubectl kubernetes-helm cloudflared
  ]);

  # Shell configuration (frequently modified)
  programs.fish = {
    enable = true;
    shellAliases = {
      x = "exit";
      k = "kubectl";
    };
  };

  # User management (occasionally updated)
  users = {
    groups.nixos = { };
    extraUsers.nixos = {
      isNormalUser = true;
      group = "nixos";
      initialPassword = "nixos";
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
  # services.vscode-server.enable = true;

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
  console = {
    keyMap = "br-abnt2";
    font = "ter-v32n";
    packages = [ pkgs.terminus_font ];
  };
  time.timeZone = "America/Sao_Paulo";
  system.stateVersion = "25.05";
  environment.variables.EDITOR = "vim";

  programs.git = {
    enable = true;
    config = {
      alias = {
        a = "add";
        c = "commit -m";
        s = "!git status";
        ck = "checkout";
        wip = "!git add . && git commit -m 'wip :zap:' && git push";
        "undo-last-commit" = "reset HEAD~";
        l = "log --color --graph --pretty=format:'%Cred%h%Creset -%C(yellow)%d%Creset %s %Cgreen(%cr) %C(bold blue)<%an>%Creset' --abbrev-commit --";
      };
    };
  };
}
