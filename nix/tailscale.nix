{ pkgs, ... }:
{
  services.tailscale.enable = true;

  environment.systemPackages = with pkgs; [
    jq
    tailscale
  ];

  # # Add the secret file to the image
  # # environment.etc."secrets/tailscale-token".text = tailscaleTokenSource; # How to pass this If I wanted to?
  # environment.etc."secrets/tailscale-token".mode = "0400"; # Read-only for owner
  # environment.etc."secrets/tailscale-token".user = "tailscale";
  # environment.etc."secrets/tailscale-token".group = "tailscale";

  systemd.services.tailscale-autoconnect = {
    description = "Automatic connection to Tailscale";

    # make sure tailscale is running before trying to connect to tailscale
    after = [ "network-pre.target" "tailscale.service" ];
    wants = [ "network-pre.target" "tailscale.service" ];
    wantedBy = [ "multi-user.target" ];

    # set this service as a oneshot job
    serviceConfig.Type = "oneshot";

    # have the job run this shell script
    script = with pkgs; ''
      # wait for tailscaled to settle
      sleep 2

      # check if we are already authenticated to tailscale
      status="$(${tailscale}/bin/tailscale status -json | ${jq}/bin/jq -r .BackendState)"
      if [ $status = "Running" ]; then # if so, then do nothing
        exit 0
      fi

      # otherwise authenticate with tailscale
      ${tailscale}/bin/tailscale up -authkey $(cat /etc/secrets/tailscale-token)
    '';
  };
}