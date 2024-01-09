{ param ? "todo-placeholder" }:
{
  options = {

    # This is a nixops thing
    network = {
      description = "A nice network description";
      storage.legacy = {
        databasefile = "~/.nixops/deployments.nixops";
      };
    };

    # This is a nixops thing
    defaults = {
      imports = [ ../sd-image.nix ];
    };

    master =
      { config, pkgs, ... }:
      {
        deployment.targetHost = "192.168.68.104";

        environment.systemPackages = with pkgs; [ cowsay ];

        services = {
          nginx.enable = true;
        };
      };

    n1 =
      { config, pkgs, ... }:
      {
        # Fake
        deployment.targetHost = "192.168.68.105";
      };
  };

}
