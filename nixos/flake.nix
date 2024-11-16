{
  description = "NixOS configuration for Raspberry Pi and VirtualBox VM on Intel Mac";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    nixpkgs-unstable.url = "github:nixos/nixpkgs/nixos-unstable";
    deploy-rs.url = "github:serokell/deploy-rs";
    nixos-hardware.url = "github:nixos/nixos-hardware";
    vscode-server.url = "github:nix-community/nixos-vscode-server";
  };

  outputs =
    {
      self,
      nixpkgs,
      nixpkgs-unstable,
      deploy-rs,
      home-manager,
      nixos-hardware,
      vscode-server,
      ...
    }:
    rec {

      nixosConfigurations = {
        rpi = nixpkgs.lib.nixosSystem {
          system = "aarch64-linux";
          specialArgs = {
            pkgs-unstable = import nixpkgs-unstable {
              system = "aarch64-linux";
              config.allowUnfree = true;
            };
          };
          modules = [
            "${nixpkgs}/nixos/modules/profiles/minimal.nix"
            nixos-hardware.nixosModules.raspberry-pi-4
            ./common.nix
            ./machines/rpi
            vscode-server.nixosModules.default
          ];
        };

        vm = nixpkgs.lib.nixosSystem {
          system = "x86_64-linux";
          modules = [
            "${nixpkgs}/nixos/modules/installer/cd-dvd/installation-cd-minimal.nix"
            ./common.nix
            ./machines/vm
          ];
        };
      };
      # Spliting this makes switching faster
      images = {
        rpi =
          (self.nixosConfigurations.rpi.extendModules {
            modules = [
              "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64.nix"
              {
                disabledModules = [ "profiles/base.nix" ];
                sdImage.compressImage = false;
              }
            ];
          }).config.system.build.sdImage;
      };

      packages.x86_64-linux.pi-image = images.rpi;
      packages.aarch64-linux.pi-image = images.rpi;

      deploy = {
        nodes = {
          rpi = {
            name = "rpi";
            hostname = "192.168.68.107";
            profiles = {
              system = {
                sshUser = "root";
                # remoteBuild = true;
                remoteBuild = false;
                path = deploy-rs.lib.aarch64-linux.activate.nixos self.nixosConfigurations.rpi;
              };
            };
          };
        };
      };

      # This is highly advised, and will prevent many possible mistakes
      checks = builtins.mapAttrs (system: deployLib: deployLib.deployChecks self.deploy) deploy-rs.lib;
    };
}
