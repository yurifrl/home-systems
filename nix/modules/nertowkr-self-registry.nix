{ pkgs, ... }:
{
  # nertowkr-self-registry user and group creation
  users = {
    users.nertowkr-self-registry = {
      isNormalUser = true;
      group = "nertowkr-self-registry";
    };
    groups.nertowkr-self-registry = { };
  };
  
  systemd.services.nertowkr-self-registry-autoconnect = {
    description = "Log network up time";
    after = [ "network-online.target" ];
    wants = [ "network-online.target" ];

    # set this service as a oneshot job
    serviceConfig.Type = "oneshot";

    # have the job run this shell script
    script =  "${pkgs.bash}/bin/bash -c 'echo $(date) >> /var/log/network-up.log'";
  };

    # Enable the service
  services.nertowkr-self-registry.enable = true;
}
