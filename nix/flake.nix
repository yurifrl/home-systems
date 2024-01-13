{
  description = "Description for the project";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = inputs@{ flake-parts, ... }:

    flake-parts.lib.mkFlake { inherit inputs; } {
      imports = [
        # To import a flake module
        # 1. Add foo to inputs
        # 2. Add foo as a parameter to the outputs function
        # 3. Add here: foo.flakeModule

      ];
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        # "aarch64-darwin"
        "x86_64-darwin"
      ];
      perSystem = { config, self', inputs', pkgs, system, ... }: {
        # Per-system attributes can be defined here. The self' and inputs'
        # module parameters provide easy access to attributes of the same
        # system.

        # Equivalent to  inputs'.nixpkgs.legacyPackages.hello;
        packages.default = pkgs.hello;
        # Install cowsay
        packages.cowsay = pkgs.cowsay;
      };
      flake = {
        # The usual flake attributes can be defined here, including system-
        # agnostic ones like nixosModule and system-enumerating ones, although
        # those are more easily expressed in perSystem.
        nixopsConfigurations.default = {
          inherit (inputs) nixpkgs;
          network.storage.legacy = { };
          # This is a nixops thing
          defaults = {
            imports = [ ./sd-image.nix ];
          };

          master =
            { config, pkgs, ... }:
            {
              # https://github.com/NixOS/nixops/blob/master/nix/options.nix
              deployment =
                {
                  targetUser = "nixos";
                  provisionSSHKey = true;
                  targetHost = "192.168.68.102";

                };

              environment.systemPackages = with pkgs; [ figlet ];

              networking.firewall.allowedTCPPorts = [ 80 22 ];

              services = {
                nginx = {
                  enable = true;
                  virtualHosts.vhost1 = {
                    default = true;
                    locations."/" = {
                      root = pkgs.writeTextDir "index.html" "Hello akavel's world!";
                    };
                  };
                };
              };


            };

          # n1 =
          #   { config, pkgs, ... }:
          #   {
          #     # Fake
          #     deployment.targetHost = "000.000.00.00";
          #   };
        };


        worker =
          { config, pkgs, ... }:
          {
            # Add your worker machine configuration here
            # For example, let's set its host to "192.168.68.103"
            deployment =
              {
                targetUser = "nixos";
                provisionSSHKey = true;
                targetHost = "192.168.68.103";
              };

            # Additional configurations go here...
          };
      };
    };
}

