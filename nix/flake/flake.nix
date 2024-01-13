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

          # network.storage.legacy = { };
          network = {
            description = "My network description";
            # storage.memory = { };
            storage.legacy = { };

          };
          # This is a nixops thing
          defaults = {
            imports = [ ./sd-image.nix ];
          };

          # master =
          #   { config, pkgs, ... }:
          #   {
          #     # https://github.com/NixOS/nixops/blob/master/nix/options.nix
          #     deployment = {
          #       targetUser = "nixos";
          #       provisionSSHKey = true;
          #       targetHost = "192.168.68.102";
          #     };
          #     environment.systemPackages = with pkgs; [ figlet ];
          #     networking.firewall.allowedTCPPorts = [ 80 22 ];
          #     services = {
          #       nginx = {
          #         enable = true;
          #         virtualHosts.vhost1 = {
          #           default = true;
          #           locations."/" = {
          #             root = pkgs.writeTextDir "index.html" "Hello akavel's world!";
          #           };
          #         };
          #       };
          #     };
          #   };

          # master = { pkgs, ... }:
          #   (import ./sd-image.nix { }) // {
          #     deployment.targetHost = "192.168.68.102"; # replace with your machine's IP

          #     networking.hostName = "my-hostname";
          #     networking.domain = "";
          #     # Allow SSH through the firewall - TODO: is it required or automatic?
          #     networking.firewall.allowedTCPPorts = [ 22 ];

          #     services.openssh.enable = true;
          #     users.users.root.openssh.authorizedKeys.keys = [
          #       "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPCfAwxYdoLR6YzoIx2+L593yLGpHaseGTCm3fxrshgD yurifl03@gmail.com"
          #     ];
          #   };

          master = { pkgs, ... }: {
            deployment = {
              targetUser = "nixos";
              provisionSSHKey = true;
              targetHost = "192.168.68.104";
            };
          };

          # n1 =
          #   { config, pkgs, ... }:
          #   {
          #     # Fake
          #     deployment.targetHost = "000.000.00.00";
          #   };
        };
      };
    };
}
