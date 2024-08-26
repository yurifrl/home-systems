{
  imports = [
    ./hardware-configuration.nix
    ./kubernetes-configuration.nix
  ];
  networking = {
    nameservers = [
      "8.8.8.8"
      "8.8.4.4"
    ];
    firewall.enable = false;
    interfaces.eth0.useDHCP = true;

    defaultGateway = {
      address = "192.168.68.107";
      interface = "eth0";
    };
  };
  ## nucles/default.nix
  networking.hostName = "nucle1";

  services.kubeadm.init = {
    enable = true;
    bootstrapTokenFile = "/var/secret/kubeadm-bootstrap-token";
  };
}
