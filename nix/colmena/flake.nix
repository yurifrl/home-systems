{
  description = "NixOS configuration for Raspberry Pi";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
  };
  outputs = { self, nixpkgs }: 

    {
      # nixosConfigurations.rpi = nixpkgs.lib.nixosSystem {
      #   inherit system;
      #   modules = [
      #     "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix"
      #     "${nixpkgs}/nixos/modules/installer/cd-dvd/channel.nix"
      #     # ({ config, lib, pkgs, ... }: {
      #     #   imports = [ ./sd-image.nix ];
      #     # })
      #     ../sd-image.nix
      #     ../hardware-configuration.nix
      #     ../tailscale.nix
      #     { nixpkgs.config.warn-dirty = false; }
      #   ];
      # };
      colmena = {
        meta = {
          # nixpkgs = nixpkgs.legacyPackages.aarch64-linux;
        };
        # defaults = { pkgs, lib, name, nodes, meta, ... }: {
        #   imports = [
        #     "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64-installer.nix"
        #     "${nixpkgs}/nixos/modules/installer/cd-dvd/channel.nix"
        #     # ({ config, lib, pkgs, ... }: {
        #     #   imports = [ ./sd-image.nix ];
        #     # })
        #     ../sd-image.nix
        #     ../hardware-configuration.nix
        #     ../tailscale.nix
        #     { nixpkgs.config.warn-dirty = false; }
        #   ];
        # };

        host-b = {
          deployment = {
            targetHost = "192.168.68.10";
            targetUser = "nixos";
          };
          boot.isContainer = true;
          time.timeZone = "America/Los_Angeles";
        };
      };
  };
}
