{
  description = "NixOS configuration";
  # inputs MUST be an Attribute set that lists the .... You know
  inputs = {
    # Here we declare we need nixpkgs, notice we only say what branch
    # not the exact commit hash, flakes will take care of that 
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };
  # outputs MUST be a function that returns an Attribute set.
  # Everything else is just convention, convention that even some
  # Nix commands will assume are true.
  outputs = inputs: {
    # `nix run` and `nix build` will look for this attribute by default
    # `nix run` or `nix run .#default` should print "Hello world"
    packages.x86_64-linux.default = 
      inputs.nixpkgs.legacyPackages.x86_64-linux.hello;

    # You can switch to the system described in ./configuration with
    # `nixos-rebuild switch --flake .#hal9000`
    nixosConfigurations.hal9000 = inputs.nixpkgs.lib {
      specialArgs = {
        # remember "inherit x;" is the same as "x = x;"
        inherit inputs; 
      };
      system = "x86_64-linux";
      modules = [ ./configuration.nix ];
    };
  };
}