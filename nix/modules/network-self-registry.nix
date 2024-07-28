{ ... }:

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

      # have the job run this shell script
      script = ''
        echo 'Network is up2!' > /var/log/network-self-registry.log
      '';
    };
  };
}
