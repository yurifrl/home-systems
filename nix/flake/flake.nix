{
  description = "NixOps config of my servers";

  inputs = {
    # I used NixOS 22.11, as this matches what was recommended by the
    # nix-infect usage guide at the time of writing. And nix-infect was
    # what I used to install NixOS on my remote machine. 
    nixpkgs.url = "github:nixos/nixpkgs/release-23.11";
  };

  outputs = { self, ... }@inputs: {
    nixopsConfigurations.default = {
      inherit (inputs) nixpkgs; # required! nixops complains if not present
      network.storage.legacy = { }; # required! nixops complains if not present

      ## TODO: here we will specify all the "regular" NixOps properties,
      ## like network.description, machine definitions, etc.
      ## ...

    };
  };
}
