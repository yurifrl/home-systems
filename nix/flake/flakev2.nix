{
  description = "Tenzir nixops example";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs";
    flake-utils.url = "github:numtide/flake-utils";
    nixops-plugged.url = "github:lukebfox/nixops-plugged";
    tenzir.url = "github:tenzir/tenzir/stable";
  };

  outputs =
    { self
    , nixpkgs
    , nixops-plugged
    , flake-utils
    , tenzir
    , ...
    }:
    let
      pkgsFor = system:
        import nixpkgs {
          inherit system;
        };
    in
    {
      nixopsConfigurations.default = {
        inherit nixpkgs;
        network.description = "tenzir";

        tenzir = {
          imports = [ tenzir.nixosModules.tenzir ];
          nixpkgs.pkgs = pkgsFor "x86_64-linux";
          services.tenzir = {
            enable = true;
            openFirewall = true;
            settings = {
              tenzir = {
                # Ensure the service is reachable from the network.
                endpoint = "0.0.0.0:5158";

                # Write metrics to a UDS socket.
                enable-metrics = true;
                metrics = {
                  self-sink.enable = false;
                  uds-sink = {
                    enable = false;
                    path = "/tmp/tenzir-metrics.sock";
                    type = "datagram";
                  };
                };
              };
            };
          };

          deployment = {
            targetUser = "nixos";
            provisionSSHKey = true;
            targetHost = "192.168.68.104";
            targetEnv = "none";
          };
        };
      };
    }
    // flake-utils.lib.eachDefaultSystem (system:
    let
      pkgs = pkgsFor system;
    in
    {
      devShell = pkgs.mkShell {
        nativeBuildInputs = [
          nixops-plugged.defaultPackage.${system}
        ];
      };
    });
}
