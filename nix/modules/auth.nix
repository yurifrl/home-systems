{ pkgs, ... }:

let
  writeToFileScript = pkgs.writeScript "write-to-file" ''
    #!/bin/bash
    echo "Network is up" > /auth/network_status.txt
  '';
in
{

  networking.useDHCP = true;

  # User account for service
  users.users.service_account = {
    isNormalUser = true;
    home = "/auth";
    createHome = true;
  };

  # Systemd service to write to a file when the network is online
  systemd.services.write-to-file = {
    description = "Write to file when the network is online";
    after = [ "network-online.target" ];
    wants = [ "network-online.target" ];
    serviceConfig = {
      ExecStart = "${writeToFileScript}";
      User = "service_account";
      Group = "service_account";
      Restart = "on-failure";
    };
    wantedBy = [ "multi-user.target" ];
  };
}
