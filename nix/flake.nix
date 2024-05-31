{
  description = "NixOS configuration for Raspberry Pi and VirtualBox VM on Intel Mac";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, ... }: {
    nixosConfigurations.rpi = nixpkgs.lib.nixosSystem {
      system = "aarch64-linux";
      modules = [
        "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix"
        "${nixpkgs}/nixos/modules/installer/cd-dvd/channel.nix"
        ./common.nix
        ./tailscale.nix
        ./machines/rpi/definition.nix
      ];
    };

    nixosConfigurations.vm = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        "${nixpkgs}/nixos/modules/installer/cd-dvd/installation-cd-minimal.nix"
        ./common.nix
        ./tailscale.nix
        ./machines/vm/definition.nix
      ];
    };

    defaultPackage.aarch64-linux = self.nixosConfigurations.rpi.config.system.build.sdImage;
    defaultPackage.x86_64-linux = self.nixosConfigurations.vm.config.system.build.isoImage;

    colmena = {
      meta = {
        nixpkgs = import nixpkgs {
          system = "aarch64-linux";
          overlays = [];
        };
      };

      defaults = { pkgs, lib, name, nodes, meta, ... }: {
        imports = [
          ./machines/${name}/definition.nix
          ./common.nix
          ./tailscale.nix
        ];

        deployment = {
          buildOnTarget = lib.mkDefault true;
        };
      };
      
      rpi = {
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
