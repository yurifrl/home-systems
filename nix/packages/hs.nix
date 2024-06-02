{
  description = "A flake for multi-architecture Go binary deployment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-22.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };

        goBinary = pkgs.stdenv.mkDerivation {
          name = "hs";
          src = ./src; # Your Go source code directory
          buildInputs = [ pkgs.go ];
          buildPhase = ''
            go build -o $out/bin/hs
          '';
          installPhase = ''
            mkdir -p $out/bin
            cp hs $out/bin/
          '';
        };

        image = pkgs.pkgsCross.${system}.buildImage {
          name = "hs";
          contents = [ goBinary ];
        };
      in
      {
        packages.default = goBinary;
        defaultPackage.x86_64-linux = image;
        defaultPackage.aarch64-linux = image;
      }
    );
}
