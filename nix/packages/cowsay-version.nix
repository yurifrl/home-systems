# nix-build -E '(import <nixpkgs> {}).callPackage ./cowsay-version.nix {}'  
{ cowsay, stdenv }:

stdenv.mkDerivation {
  name = "cowsay-version";
  src = ./.;

  phases = [ "installPhase" ];

  propagatedBuildInputs = [ cowsay ];
  buildInputs = [ cowsay ];
  
  installPhase = ''
    mkdir -p $out/bin
    echo '#!/bin/sh' > $out/bin/cowsay-version
    echo '${cowsay}/bin/cowsay "Home Automation Systems Version $(cat ${/src/VERSION})"' >> $out/bin/cowsay-version
    chmod +x $out/bin/cowsay-version
  '';
}