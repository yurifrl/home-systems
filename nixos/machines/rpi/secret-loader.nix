{ config, lib, pkgs, ... }:

let
  # Define the path to the script you want to execute
  secretScriptPath = "/etc/secrets/sync.sh";
in
{
  options.secret-loader.enable = lib.mkOption {
    type = lib.types.bool;
    default = false;
    description = "Enable the secret loader service to execute a script.";
  };

  config = lib.mkIf config.secret-loader.enable {
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
        Restart = "always";
        RestartSec = "30s";
        StartLimitIntervalSec = "0"; # Disable the start limit interval
        StartLimitBurst = "0";       # Disable the start limit burst
      };

      # Restart the service on failure
      restartIfChanged = true;
      restartTriggers = [ secretScriptPath ];
    };
  };
}