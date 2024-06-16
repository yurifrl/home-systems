{ pkgs, ... }:

{
  config = {
    systemd.services.network-self-registry = {
      description = "Network Self Registry";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      # serviceConfig = {
      #   ExecStart = "${pkgs.writeShellScriptBin "network-self-registry" ''echo 'Network is up!' > /var/log/network-self-registry.log''}";
      #   StandardOutput = "journal";
      #   StandardError = "journal";
      # };
      wantedBy = [ "multi-user.target" ];
      enable = true;

      # set this service as a oneshot job
      serviceConfig.Type = "oneshot";

      # have the job run this shell script
      script = with pkgs; ''
        echo 'Network is up2!' > /var/log/network-self-registry.log
      '';
    };
  };
}
