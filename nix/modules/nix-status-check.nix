{ pkgs, ... }:

{
  environment.systemPackages = with pkgs; [ busybox ];

  systemd.services.static-server = {
    description = "Static HTTP Server";
    after = [ "network.target" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig = {
      ExecStart = "${pkgs.busybox}/bin/busybox httpd -f -p 8080 -h /tmp/static";
      ExecStartPre = ''
        mkdir -p /tmp/static
        echo "1.0.0" > /tmp/static/index.html
      '';
      Restart = "always";
      User = "nobody";
      Group = "nogroup";
    };
  };

  networking.firewall.allowedTCPPorts = [ 8080 ];
}
