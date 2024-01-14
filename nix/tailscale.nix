{ lib, pkgs, ... }:
let
  tailscaleTokenSource = builtins.readFile "/etc/secrets/tailscale-token";
in
{
  # Tailscale user and group creation
  users.users.tailscale = {
    isNormalUser = true;
    group = "tailscale";
  };

  users.groups.tailscale = { };

  environment.systemPackages = with pkgs; [
    jq
    tailscale
  ];

  # Add the secret file to the image
  environment.etc."secrets/tailscale-token".text = tailscaleTokenSource;
  environment.etc."secrets/tailscale-token".mode = "0400"; # Read-only for owner
  environment.etc."secrets/tailscale-token".user = "tailscale";
  environment.etc."secrets/tailscale-token".group = "tailscale";

  # Tailscale service configuration
  services.tailscale = {
    enable = true;
  };

  # Tailscale automatic connection setup
  systemd.services.tailscale-autoconnect = {
    description = "Automatic connection to Tailscale";
    after = [ "network-pre.target" "tailscale.service" ];
    wants = [ "network-pre.target" "tailscale.service" ];
    wantedBy = [ "multi-user.target" ];
    serviceConfig.Type = "oneshot";
    serviceConfig.User = "tailscale";
    script = ''
      sleep 2
      status="$(tailscale status -json | jq -r .BackendState)"
      if [ "$status" = "Running" ]; then
        echo "Procress running, exiting"
        exit 0
      fi
      echo "Starting tailscale"
      tailscale up -authkey $(cat /etc/secrets/tailscale-token)
    '';
  };
}
