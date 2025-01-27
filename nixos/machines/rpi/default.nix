{ pkgs, ... }:
{
  imports = [
    ./hardware-configuration.nix
    ./argo-setup.nix
    ./secret-loader.nix
    ./longhorn.nix
    ./inotify.nix
    ../../modules/kubernetes.nix
  ];

  # Enable Argo CD setup service
  services.argo-setup.enable = false;

  # Enable Secret Loader service
  services.secret-loader.enable = false;

  # Add RPI-specific packages
  environment.systemPackages = with pkgs; [
    htop
    btop
    k9s

    #
    velero
  ];

  # Add RPI-specific shell aliases
  programs.fish.shellAliases = {
    argo-restart = "cd /home/nixos/home-systems/nixos && git pull origin main & sudo systemctl restart argo-setup & sudo journalctl -u argo-setup.service -f";
    xablaunixos = "cd /home/nixos/home-systems/nixos && git pull origin main && sudo nixos-rebuild switch --flake .#rpi --impure --show-trace";
    xablaunixosupgrade = "cd /home/nixos/home-systems/nixos && git pull origin main && sudo nix-channel --update && sudo nixos-rebuild switch --flake .#rpi --impure --show-trace --upgrade";
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
      "--disable=local-storage"
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
