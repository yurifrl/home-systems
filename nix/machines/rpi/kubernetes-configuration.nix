{ pkgs, ... }:
let
  hosts = [
    "sergio.localdomain"
    "nucles.localdomain"
  ];
in
{
  imports = [
    ../packages/kube
  ];

  # System config
  networking.domain = "localdomain";
  networking.useNetworkd = true;

  systemd.network.enable = true;
  systemd.network.networks.lan.name = "en*";
  systemd.network.networks.lan.DHCP = "yes";
  systemd.network.wait-online.anyInterface = true;
  services.resolved.dnssec = "false";

  security.sudo.wheelNeedsPassword = false;

  nix.gc.automatic = true;
  nix.gc.options = "--delete-older-than 14d";

  # Kube
  services.kubeadm = {
    enable = true;
    package = pkgs.kubernetes;
    kubelet.enable = true;
    init.initConfig = { };

    init.clusterConfig = {
      clusterName = "nucles";
      controlPlaneEndpoint = "nucles.localdomain:6443";
      apiServer.certSANs = hosts;
      proxy.disabled = true;
      controllerManager.extraArgs = [
        {
          name = "bind-address";
          value = "0.0.0.0";
        }
      ];
      scheduler.extraArgs = [
        {
          name = "bind-address";
          value = "0.0.0.0";
        }
      ];
    };

    init.kubeletConfig = {
      serializeImagePulls = false;
      allowedUnsafeSysctls = [ "net.ipv4.conf.all.src_valid_mark" ];
      shutdownGracePeriod = "5m";
      shutdownGracePeriodCriticalPods = "1m";
      cpuCFSQuota = false;
    };

    upgrade.enable = true;
    upgrade.upgradeConfig = { };
  };

  environment.systemPackages = [ pkgs.kubernetes ];
}
