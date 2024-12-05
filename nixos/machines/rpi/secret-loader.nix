{ config, lib, pkgs, ... }:

let
  k3sConfig = "/etc/rancher/k3s/k3s.yaml";
  # Define the path to the script you want to execute
  secretScriptPath = "/data/secrets.sh";
in
{
  systemd.services.secret-loader = {
    description = "Secret Loader Service";
    after = [ "network.target" "k3s.service" "argo-setup.service" ];
    requires = [ "k3s.service" "argo-setup.service" ];
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
      # Run as kubernetes user
      User = "kubernetes";
      Group = "kubernetes";
      
      # Add necessary capabilities and security settings
      SupplementaryGroups = [ "kubernetes" ];
      # Ensure the service can read the k3s config and secrets
      ReadOnlyPaths = [ k3sConfig ];
      ReadWritePaths = [ secretScriptPath ];

      # Add retry logic
      Restart = "on-failure";
      RestartSec = "30s";
      StartLimitIntervalSec = "0";
      StartLimitBurst = "0";
    };

    # Restart the service on failure
    restartIfChanged = true;
    restartTriggers = [ secretScriptPath ];
  };
} 