{
  description = "NixOps config of my servers";

  inputs = {
    # I used NixOS 22.11, as this matches what was recommended by the
    # nix-infect usage guide at the time of writing. And nix-infect was
    # what I used to install NixOS on my remote machine. 
    nixpkgs.url = "github:nixos/nixpkgs/release-22.11";
    flake-parts.url = "github:hercules-ci/flake-parts";

  };

  outputs = inputs@{ self, nixpkgs, flake-parts, ... }: flake-parts.lib.mkFlake { inherit inputs; } {




    systems = [
      "x86_64-linux"
      "aarch64-linux"
      "x86_64-darwin"
    ];

    perSystem = { pkgs, system, ... }: {
      nixopsConfigurations.default = {
        inherit (inputs) nixpkgs; # required! nixops complains if not present
        network.storage.legacy = { }; # required! nixops complains if not present

        ## TODO: here we will specify all the "regular" NixOps properties,
        ## like network.description, machine definitions, etc.
        ## ...

      };
      # Equivalent to  inputs'.nixpkgs.legacyPackages.hello;
      packages.default = pkgs.hello;

      # packages.default = pkgs.callPackage ./package.nix { };

      # packages.x86_64-linux.hello = nixpkgs.legacyPackages.x86_64-linux.hello;

      # packages.x86_64-linux.default = self.packages.x86_64-linux.hello;

      # packages.x86_64-darwin.default = self.packages.x86_64-darwin.hello;

      # packages.x86_64-darwin.hello = nixpkgs.legacyPackages.x86_64-darwin.hello;


    };

  };
}


