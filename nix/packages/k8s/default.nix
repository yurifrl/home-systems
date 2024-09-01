{
  pkgs,
  lib,
  config,
  ...
}:
let
  cfg = config.services.k8s;
in
{
  imports = [
    ./config.nix
  ];

  options.services.k8s = {
    enable = lib.mkEnableOption "k8s";

  };

  config = lib.mkIf cfg.enable {

    # Do stuff here
  };
}
