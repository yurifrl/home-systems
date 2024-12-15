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
          specialArgs = {
            pkgs-unstable = import nixpkgs-unstable {
              system = "x86_64-linux";
              config.allowUnfree = true;
            };
          };
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

      deploy.nodes = {
        rpi = {
          hostname = "192.168.68.100";
          profiles.system = {
            sshUser = "root";
            path = deploy-rs.lib.aarch64-linux.activate.nixos self.nixosConfigurations.rpi;
            remoteBuild = true;
          };
        };
      };

      # This is highly advised, and will prevent many possible mistakes
      checks = builtins.mapAttrs (system: deployLib: deployLib.deployChecks self.deploy) deploy-rs.lib;
    };
}
