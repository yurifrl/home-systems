{
  master =
    { config, pkgs, ... }:
    {
      imports = [ ./sd-image.nix ];


      # network.storage.legacy = { };
      network = {
        description = "My network description";
        # storage.memory = { };
        storage.legacy = { };

      };

      master = { pkgs, ... }: {
        deployment = {
          targetUser = "nixos";
          provisionSSHKey = true;
          targetHost = "192.168.68.104";
        };

      };
    };
}
