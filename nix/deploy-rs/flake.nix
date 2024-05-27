# nix build .#nixosConfigurations.rpi.config.system.build.sdImage
# nix flake check
{
  description = "NixOS configuration for Raspberry Pi";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    deploy-rs.url = "github:serokell/deploy-rs";
  };
  outputs = { self, nixpkgs, deploy-rs }: 
    let
      system = "aarch64-linux";
    in
    {
      deploy.nodes.example = {
        hostname = "192.168.68.108";
        profiles.hello = {
          sshUser = "nixos";
          user = "nixos";
          path = deploy-rs.lib.aarch64-linux.activate.nixos self.nixosConfigurations.rpi;
        };
      };
      checks = builtins.mapAttrs (system: deployLib: deployLib.deployChecks self.deploy) deploy-rs.lib;
      nixosConfigurations.rpi = nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix"
          "${nixpkgs}/nixos/modules/installer/cd-dvd/channel.nix"
          # ({ config, lib, pkgs, ... }: {
          #   imports = [ ./sd-image.nix ];
          # })
          ./sd-image.nix
          ./hardware-configuration.nix
          ./tailscale.nix
          { nixpkgs.config.warn-dirty = true; }
        ];
      };
    };
}
