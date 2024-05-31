{ pkgs, ... }:
let
  version-theta = pkgs.writeShellScriptBin "version-theta" ''
    #!/bin/sh
    echo "System Theta Version: 1.0"
  '';
in
{
  environment.systemPackages = [
    version-theta   
  ];
}