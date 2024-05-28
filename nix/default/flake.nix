# nix build .#nixosConfigurations.rpi.config.system.build.sdImage
# nix flake check
{
  description = "NixOS configuration for Raspberry Pi";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };
  outputs = { self, nixpkgs }: 
    let
      system = "aarch64-linux";
    in
    {
      nixosConfigurations.rpi = nixpkgs.lib.nixosSystem {
        inherit system;
        modules = [
          "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix"
          "${nixpkgs}/nixos/modules/installer/cd-dvd/channel.nix"
          # ({ config, lib, pkgs, ... }: {
          #   imports = [ ./sd-image.nix ];
          # })
          ../sd-image.nix
          ../hardware-configuration.nix
          ../tailscale.nix
          { nixpkgs.config.warn-dirty = true; }
        ];
      };
    };
}