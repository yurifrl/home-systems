# nix-build -E '(import <nixpkgs> {}).callPackage ./cowsay-version.nix {}'  
{ writeShellScriptBin, cowsay }:

let
  version = "0.0.34";
  
  script = ''
    #!/bin/sh
    ${cowsay}/bin/cowsay "Home Automation Systems Version ${version}"
  '';
in
writeShellScriptBin "cowsay-version" script


