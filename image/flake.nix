# OLD
{
  description = "A Simplified Nix flake for your Python project";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        packages = {
          default = pkgs.python3.withPackages (ps: with ps; [ click ]);
        };
      in
      {
        devShell = pkgs.mkShell {
          buildInputs = with pkgs; [ python3 python3Packages.click fish vim ];
        };

        defaultPackage = packages.default;
      }
    );
}

