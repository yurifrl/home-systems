{
  pkgs,
  lib,
  config,
  ...
}:
{
  options.services.k8s.enable = with lib; {
    enable = mkEnableOption "kubernetes enable";
  };
}
