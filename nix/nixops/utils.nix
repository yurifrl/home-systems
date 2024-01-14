# utils.nix
{ pkgs }:

{
  showVersionScript = pkgs.writeShellScriptBin "version" ''
    #!/bin/sh
    echo "Custom Version: 2.0"
  '';
}
