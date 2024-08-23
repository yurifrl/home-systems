# https://discourse.nixos.org/t/kubernetes-network-malfunction-after-upgrading-to-19-09/4620/3
{ pkgs, lib, config, ... }: let 
  cfg = config.services.kubeadm;
in {
  options.services.kubeadm = {
    enable = lib.mkEnableOption "kubeadm";
    role = lib.mkOption {
      type = lib.types.enum ["master" "node" ];
    };
    apiserverAddress = lib.mkOption {
      type = lib.types.str;
      description = ''
        The address on which we can reach the masters. Could be loadbalancer
      '';
    };
    bootstrapToken = lib.mkOption {
      type = lib.types.str;
      description = ''
        The master will print this to stdout after being set up.
      '';
    };
    nodeip = lib.mkOption {
      type = lib.types.str;
    };
    discoveryTokenCaCertHash = lib.mkOption {
      type = lib.types.str;
    };
  };
  config = lib.mkIf cfg.enable {

    boot.kernelModules = [ "overlay" "br_netfilter" ];
    boot.kernel.sysctl = {
      "net.bridge.bridge-nf-call-iptables" = 1;
      "net.bridge.bridge-nf-call-ip6tables" = 1;
      "net.ipv4.ip_forward" = 1;
    };

    environment.systemPackages = with pkgs; [
      containerd
      cri-tools
      docker
      ethtool
      gitMinimal
      iproute
      iptables
      openssh
      socat
      thin-provisioning-tools
      utillinux
    ];

    virtualisation.containerd = {
      enable = true;
      configFile = ./containerd-config.toml;
    };

    systemd.services.kubeadm = {
      wantedBy = [ "multi-user.target" ];
      after = [ "kubelet.service" ];
      postStart = lib.mkIf (cfg.role == "master")
        ''
          KUBECONFIG=/etc/kubernetes/admin.conf kubectl -n kube-public get cm cluster-info -o json | jq -r '.data.kubeconfig' > /etc/kubernetes/cluster-info.cfg
          chmod a+r /etc/kubernetes/cluster-info.cfg
        '';

      # These paths are needed to convince kubeadm to bootstrap
      path = with pkgs; [ kubernetes jq gitMinimal openssh cri-tools containerd utillinux iproute ethtool thin-provisioning-tools iptables socat ];
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = true;
        # Makes sure that its only started once, during bootstrap
        ConditionPathExists = "!/var/lib/kubelet/config.yaml";
        StateDirectory = "kubelet";
        ConfigurationDirectory = "kubernetes";
        ExecStart = {
          master = ''
            ${pkgs.kubernetes}/bin/kubeadm init \
              --pod-network-cidr=10.244.0.0/16 \
              --apiserver-advertise-address=${cfg.apiserverAddress} \
              --ignore-preflight-errors='all' \
              --token ${cfg.bootstrapToken} \
              --token-ttl 0 \
              --upload-certs
          '';
          node = ''
            ${pkgs.kubernetes}/bin/kubeadm join ${cfg.apiserverAddress} \
              --token ${cfg.bootstrapToken}  \
              --discovery-token-unsafe-skip-ca-verification \
              --ignore-preflight-errors all \
              --discovery-token-ca-cert-hash ${cfg.discoveryTokenCaCertHash}
          '';
        }.${cfg.role};
      };
    };
    systemd.services.kubelet = {
      description = "Kubernetes Kubelet Service";
      wantedBy = [ "multi-user.target" ];

      path = with pkgs; [ gitMinimal openssh docker utillinux iproute ethtool thin-provisioning-tools iptables socat cni ];

      serviceConfig = {
        StateDirectory = "kubelet";

        # This populates $KUBELET_KUBEADM_ARGS and is provided
        # by kubeadm init and join
        EnvironmentFile = "-/var/lib/kubelet/kubeadm-flags.env";

        Restart = "always";
        RestartSec = 10;

        ExecStart = ''
          ${pkgs.kubernetes}/bin/kubelet \
            --kubeconfig=/etc/kubernetes/kubelet.conf \
            --bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf \
            --config=/var/lib/kubelet/config.yaml \
            $KUBELET_KUBEADM_ARGS
        '';
      };
    };
  };
}