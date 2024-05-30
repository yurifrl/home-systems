{
  description = "NixOS configuration for Raspberry Pi";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, ... }: 
  let
    system = "aarch64-linux";

    commonModules = [
      "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix"
      "${nixpkgs}/nixos/modules/installer/cd-dvd/channel.nix"
      { nixpkgs.config.warn-dirty = true; }
      # ({ config, lib, pkgs, ... }: {
      #   imports = [ ./sd-image.nix ];
      # })
      ./sd-image.nix
      ./tailscale.nix
      ./machines/rpi.nix
    ];
  in
  {
    nixosConfigurations.rpi = nixpkgs.lib.nixosSystem {
      inherit system;
      modules = commonModules;
    };

    defaultPackage.${system} = self.nixosConfigurations.rpi.config.system.build.sdImage;
    defaultPackage.x86_64-linux = self.nixosConfigurations.rpi.config.system.build.sdImage;

    colmena = {
      meta = {
        nixpkgs = import nixpkgs {
          system = "aarch64-linux";
          overlays = [];
        };
      };

      defaults = { pkgs, lib, name, nodes, meta, ... }: {
        imports = commonModules;

        deployment = {
          buildOnTarget = lib.mkDefault true;
        };
      };
      
      proto = {
        deployment = {
          targetHost = "192.168.68.108";
          targetUser = "nixos";
        };
        boot.isContainer = true;
        time.timeZone = "America/Los_Angeles";
      };
    };
  };
}
