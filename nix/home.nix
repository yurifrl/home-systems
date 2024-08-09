{ pkgs, ... }:

{
  # Packages that should be installed to the user profile.
  home.packages = with pkgs; [
    neovim
  ];

  # Let home Manager install and manage itself.
  programs.home-manager.enable = true;
}