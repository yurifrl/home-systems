{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.services.raspberrypi-exporter;
in {
  options.services.raspberrypi-exporter = {
    enable = mkEnableOption "Raspberry Pi metrics exporter for Prometheus";
    
    textfileDirectory = mkOption {
      type = types.str;
      default = "/var/lib/node_exporter/textfile_collector";
      description = "Directory where metrics will be written for node_exporter to collect";
    };

    interval = mkOption {
      type = types.str;
      default = "1m";
      description = "How often to collect metrics";
    };
  };

  config = mkIf cfg.enable {
    assertions = [{
      assertion = config.hardware.raspberry-pi ? revisionCode;
      message = "The raspberrypi-exporter service is only supported on Raspberry Pi hardware.";
    }];

    systemd.services.raspberrypi-exporter = {
      description = "Raspberry Pi Metrics Exporter";
      after = [ "network.target" ];
      
      serviceConfig = {
        Type = "oneshot";
        ExecStart = "${pkgs.raspberrypi-exporter}/bin/raspberrypi_exporter";
        DynamicUser = true;
        StateDirectory = "node_exporter/textfile_collector";
        StateDirectoryMode = "0770";
        RuntimeDirectory = "raspberrypi-exporter";
        RuntimeDirectoryMode = "0770";
        CacheDirectory = "raspberrypi-exporter";
        CacheDirectoryMode = "0770";
      };
    };

    systemd.timers.raspberrypi-exporter = {
      description = "Timer for Raspberry Pi Metrics Exporter";
      wantedBy = [ "timers.target" ];
      
      timerConfig = {
        OnBootSec = "1m";
        OnUnitActiveSec = cfg.interval;
      };
    };

    # Ensure node_exporter is installed and configured
    services.prometheus.exporters.node = {
      enable = true;
      enabledCollectors = [ "textfile" ];
      extraFlags = [
        "--collector.textfile.directory=${cfg.textfileDirectory}"
      ];
    };

    # Create node-exporter group if it doesn't exist
    users.groups.node-exporter = {};
  };
}