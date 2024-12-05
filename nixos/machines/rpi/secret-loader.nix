{ config, lib, pkgs, ... }:

let
  k3sConfig = "/etc/rancher/k3s/k3s.yaml";
  # Define the path to the script you want to execute
  secretScriptPath = "/data/secrets.sh";
in
{
  systemd.services.secret-loader = {
    description = "Secret Loader Service";
    after = [ "network.target" "k3s.service" ];
    requires = [ "k3s.service" ];
    wantedBy = [ "multi-user.target" ];
    path = with pkgs; [
      kubectl
    ];
    environment = {
      KUBECONFIG = k3sConfig;
    };
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = "yes";
      TimeoutStartSec = "0";

      ExecStart = "${pkgs.bash}/bin/bash ${secretScriptPath}";
      # Ensure the script is executable
      ExecStartPre = "${pkgs.coreutils}/bin/chmod +x ${secretScriptPath}";
      # Run as a specific user, e.g., 'root' or another user
      User = "root";

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