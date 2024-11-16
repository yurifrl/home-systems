# nix-build -E '(import <nixpkgs> {}).callPackage ./cowsay-version.nix {}'  
{ cowsay, stdenv }:

let
  version = "0.0.32";
in
stdenv.mkDerivation {
  name = "cowsay-version";
  src = ../../.;

  phases = [ "installPhase" ];

  propagatedBuildInputs = [ cowsay ];
  buildInputs = [ cowsay ];

  installPhase = ''
    mkdir -p $out/bin
    echo '#!/bin/sh' > $out/bin/cv
    # TODO: make this work
    # echo '${cowsay}/bin/cowsay "Home Automation Systems Version $(cat $src/VERSION)"' >> $out/bin/cv
    echo '${cowsay}/bin/cowsay "Home Automation Systems Version ${version}"' >> $out/bin/cv
    chmod +x $out/bin/cv
  '';
}


