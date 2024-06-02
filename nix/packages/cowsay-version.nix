{
  description = "A very basic flake with cowsay outputting version info";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
  };

  outputs = { self, nixpkgs }: {

    packages.x86_64-linux.cowsay-version = nixpkgs.legacyPackages.x86_64-linux.stdenv.mkDerivation {
      name = "cowsay-version";
      buildInputs = [ nixpkgs.legacyPackages.x86_64-linux.cowsay ];

      buildCommand = ''
        mkdir -p $out/bin

        # Create a script that runs cowsay with the contents of the version file
        echo "#!${nixpkgs.legacyPackages.x86_64-linux.stdenv.shell}" > $out/bin/cowsay-version
        echo "${nixpkgs.legacyPackages.x86_64-linux.cowsay}/bin/cowsay \\"$(cat ${/src/VERSION})\\"" >> $out/bin/cowsay-version

        chmod +x $out/bin/cowsay-version
      '';
    };

    packages.x86_64-linux.default = self.packages.x86_64-linux.cowsay-version;
  };
}
