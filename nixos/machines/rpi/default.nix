{ pkgs, pkgs-unstable, ... }:
{
  imports = [
    ./hardware-configuration.nix
    ./argo-setup.nix
    ./secret-loader.nix
    ./longhorn.nix
    ../../modules/kubernetes.nix
  ];

  # Add RPI-specific packages
  environment.systemPackages = with pkgs; [
    htop
    btop
    k9s
    velero
  ];

  # Use unstable k3s
  services.k3s.package = pkgs-unstable.k3s;

  # Configure storage directory for k3s
  systemd.tmpfiles.rules = [
    "d /storage 0755 k3s k3s -"
    "d /storage/test-volume 0755 k3s k3s -"
  ];

  # Ensure k3s user and group exist
  users.users.k3s = {
    isSystemUser = true;
    group = "k3s";
    uid = 1000;  # Make sure this matches with your k3s setup
  };

  users.groups.k3s = {};

  # Add RPI-specific shell aliases
  programs.fish.shellAliases = {
    xablaunixos = "cd /home/nixos/home-systems/nixos && git pull origin main && sudo nixos-rebuild switch --flake .#rpi --impure --show-trace";
  };

  networking = {
    nameservers = [
      "8.8.8.8"
      "8.8.4.4"
    ];
    firewall.enable = false;
    interfaces.eth0 = {
      useDHCP = false;
      ipv4.addresses = [{
        address = "192.168.68.100";
        prefixLength = 24;
      }];
    };
    defaultGateway = "192.168.68.1";
    hostName = "nixos-1";
  };

  # https://github.com/rancher/rancher/issues/38849
  services.k3s = {
    enable = true;
    role = "server";
    tokenFile = "/data/k3s-token";
    extraFlags = [
      "--disable=traefik"
      "--disable=servicelb"
      # "--disable=local-storage"
      "--disable-cloud-controller"
      "--flannel-backend=host-gw"
      # Static IP address for the k3s server node
      # Using 192.168.68.100 as a reserved static IP in the local network
      # This ensures stable networking for the k3s control plane
      "--tls-san=192.168.68.100"
      "--bind-address=0.0.0.0"
      "--advertise-address=192.168.68.100"
      "--node-ip=192.168.68.100"
      "--cluster-init"
      "--write-kubeconfig-mode=644"
    ];
  };

  environment.variables = {
    KUBECONFIG = "/etc/rancher/k3s/k3s.yaml";
  };
}
