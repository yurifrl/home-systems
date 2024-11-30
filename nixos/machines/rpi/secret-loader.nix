{ config, lib, pkgs, ... }:

let
  # Define the path to the script you want to execute
  secretScriptPath = "/etc/secrets/sync.sh";
in
{
  systemd.services.secret-loader = {
    description = "Secret Loader Service";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];

    serviceConfig = {
      ExecStart = "${secretScriptPath}";
      # Ensure the script is executable
      ExecStartPre = "${pkgs.coreutils}/bin/chmod +x ${secretScriptPath}";
      # Run as a specific user, e.g., 'root' or another user
      User = "root";
      # Optionally, specify a working directory
      WorkingDirectory = "/etc/secrets";
      
      # Add retry logic
      Restart = "on-failure";  # Only restart if the service exits with non-zero status
      RestartSec = "30s";
      StartLimitIntervalSec = "0";
      StartLimitBurst = "0";
    };

    # Restart the service on failure
    restartIfChanged = true;
    restartTriggers = [ secretScriptPath ];
  };
} 