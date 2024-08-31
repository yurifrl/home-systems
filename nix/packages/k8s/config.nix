{
  pkgs,
  lib,
  config,
  ...
}:
{
  options.services.k8s.init = with lib; {
    enable = mkEnableOption "kubernetes init";
  };
}
