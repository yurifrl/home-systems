{
  description = "Base system for raspberry pi 4";
  inputs = {
    nixpkgs.url = "nixpkgs/nixos-unstable";
    nixos-generators = {
      url = "github:nix-community/nixos-generators";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs =
    {
      self,
      nixpkgs,
      nixos-generators,
      ...
    }:
    {
      nixosModules = {
        system = {
          #boot.zfs.enabled = false; # error: The option `boot.zfs.enabled' is read-only, but it's set multiple times.
          #boot.zfs.enabled = nixpkgs.lib.mkDefault false; # error: The option `boot.zfs.enabled' is read-only, but it's set multiple times.
          #boot.supportedFilesystems = [ "ext4" ]; # Does not have any effect, because it only adds `ext4`, not deletes anything else from the array.

          # Copied from https://github.com/NixOS/nixpkgs/blob/f3565a2c088883636f198550eac349ed82c6a2b3/nixos/modules/installer/sd-card/sd-image-aarch64-new-kernel-no-zfs-installer.nix#L6 "
          # This works, however it is a hack:
          # Makes `availableOn` fail for zfs, see <nixos/modules/profiles/base.nix>.
          # This is a workaround since we cannot remove the `"zfs"` string from `supportedFilesystems`.
          # The proper fix would be to make `supportedFilesystems` an attrset with true/false which we
          # could then `lib.mkForce false`
          #nixpkgs.overlays = [(final: super: {
          #  zfs = super.zfs.overrideAttrs(_: {
          #    meta.platforms = [];
          #  });
          #})];

          # Disabling the whole `profiles/base.nix` module, which is responsible
          # for adding ZFS and a bunch of other unnecessary programs:
          disabledModules = [
            "profiles/base.nix"
          ];

          system.stateVersion = "23.11";
        };
        users = {
          users.users = {
            admin = {
              password = "admin";
              isNormalUser = true;
              extraGroups = [ "wheel" ];
            };
          };
        };
      };

      packages.aarch64-linux = {
        sdcard = nixos-generators.nixosGenerate {
          system = "aarch64-linux";
          format = "sd-aarch64";
          modules = [
            self.nixosModules.system
            self.nixosModules.users
          ];
        };
      };
    };
}
