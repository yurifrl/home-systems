{
  description = "NixOS configuration for Raspberry Pi and VirtualBox VM on Intel Mac";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs, ... }: {
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
      x86_64-linux = self.nixosConfigurations.vm.config.system.build.isoImage;
    };

    colmena = {
      meta = {
        nixpkgs = import nixpkgs {
          system = "x86_64-linux";
          overlays = [];
        };
      };

      defaults = { lib, name, ... }: {
        imports = [
          ./common.nix
          ./modules
          ./machines/${name}/definition.nix
        ];

        deployment = {
          buildOnTarget = lib.mkDefault true;

          keys = {
            "tailscale-token" = {
              keyFile = "/src/secrets/${name}/tailscale-token";
              user = "tailscale";
              group = "tailscale";
              permissions = "0400";
            };
          };
        };
      };

      rpi = {
        deployment = {
          targetHost = "192.168.68.103";
          targetUser = "nixos";
        };
      };
      
      vm = {
        # Via tailscale
        deployment = {
          targetHost = "127.0.0.1";
          targetPort = 2222;
          #
          # targetHost = "100.94.23.120"; # Tailscale
          #
          targetUser = "nixos";
        };

      };
    };
  };
}
