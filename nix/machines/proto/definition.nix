{ pkgs, ... }:
let
  version-proto = pkgs.writeShellScriptBin "version-proto" ''
    #!/bin/sh
    echo "System Proto Version: 1.0"
  '';
in
{
  imports = [
    ./hardware-configuration.nix
  ];

  environment.systemPackages = [
    version-proto   
  ];
}