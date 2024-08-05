{
  description = "NixOS configuration for Raspberry Pi and VirtualBox VM on Intel Mac";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    deploy-rs.url = "github:serokell/deploy-rs";
  };

  outputs = { self, nixpkgs, deploy-rs, ... }: {
    nixosConfigurations = {
      rpi = nixpkgs.lib.nixosSystem {
        system = "aarch64-linux";
        modules = [
          "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix"
          "${nixpkgs}/nixos/modules/installer/cd-dvd/channel.nix"
          ./common.nix
          ./modules
          ./machines/rpi/definition.nix
          ({ ... }: {
            sdImage.compressImage = false; # If true, will build a .zst compressed image.
          })
        ];
      };

      vm = nixpkgs.lib.nixosSystem {
        system = "x86_64-linux";
        modules = [
          "${nixpkgs}/nixos/modules/installer/cd-dvd/installation-cd-minimal.nix"
          ./common.nix
          ./modules
          ./machines/vm/definition.nix
        ];
      };
    };

    packages = {
      aarch64-linux.default = self.nixosConfigurations.rpi.config.system.build.sdImage;
      x86_64-linux.default = self.nixosConfigurations.vm.config.system.build.isoImage;
    };

    deploy.nodes.some-random-system = {
      hostname = "192.168.68.102";
      profiles = {
        system = {
          sshUser = "nixos";
          path = deploy-rs.lib.aarch64-linux.activate.nixos self.nixosConfigurations.rpi;
          # path = deploy-rs.lib.x86_64-linux.activate.nixos self.nixosConfigurations.rpi;
        };
      };
    };

    # This is highly advised, and will prevent many possible mistakes
    checks = builtins.mapAttrs (system: deployLib: deployLib.deployChecks self.deploy) deploy-rs.lib;
  };
}
