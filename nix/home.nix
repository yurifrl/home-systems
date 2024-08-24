{ pkgs, ... }:

{
  # Packages that should be installed to the user profile.
  home.packages = with pkgs; [
    neovim
  ];

  # basic configuration of git, please change to your own
  programs.git = {
    enable = true;
    userName = "Yuri";
    userEmail = "yuri@placeholder.com";
    aliases = {
      s = "status";
    };
  };

  # Let home Manager install and manage itself.
  programs.home-manager.enable = true;

  home-manager.users.nixos.home.stateVersion = "24.05";
}
