{ config, lib, pkgs, ... }:

let
  k3sConfig = "/etc/rancher/k3s/k3s.yaml";
  secretScriptPath = "/data/secrets.sh";
in
{
  options = {
    services.secret-loader = {
      enable = lib.mkEnableOption "Secret Loader Service";
    };
  };

  config = lib.mkIf config.services.secret-loader.enable {
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
        # Run as nixos user
        User = "nixos";
        Group = "nixos";
        
        # Add necessary capabilities and security settings
        # Ensure the service can read the k3s config and secrets
        ReadOnlyPaths = [ k3sConfig ];
        ReadWritePaths = [ secretScriptPath ];

        # Add retry logic
        Restart = "on-failure";
        RestartSec = "30s";
        StartLimitBurst = "0";
      };

      # Restart the service on failure
      restartIfChanged = true;
      restartTriggers = [ secretScriptPath ];
    };
  };
} 