{ pkgs, pkgs-unstable, ... }:
{
  environment.systemPackages = with pkgs; [
    jq
    pkgs-unstable.tailscale
  ];

  # Tailscale service configuration
  services.tailscale = {
    enable = true;
    package = pkgs-unstable.tailscale;
  };

  systemd.services.tailscale-autoconnect = {
    description = "Automatic connection to Tailscale";

    # make sure tailscale is running before trying to connect to tailscale
    after = [
      "network-pre.target"
      "tailscale.service"
    ];
    wants = [
      "network-pre.target"
      "tailscale.service"
    ];
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

      # otherwise authenticate with tailscale and accept DNS
      ${tailscale}/bin/tailscale up -authkey $(cat /etc/secrets/tailscale-auth.key)
    '';
  };

  # Tailscale user and group creation
  users = {
    users.tailscale = {
      isNormalUser = true;
      group = "tailscale";
    };
    groups.tailscale = { };
  };
}
