{ config, lib, pkgs, ... }:

{
    boot.kernelModules = [ 
      "iscsi_tcp"  # For Longhorn
      "dm_snapshot" 
      "dm_mirror" 
      "dm_thin_pool"
    ];

    # Configure kernel parameters for RBD features
    boot.kernel.sysctl = {
    "net.bridge.bridge-nf-call-iptables" = 1;
    "net.bridge.bridge-nf-call-ip6tables" = 1;
    };

    # Create kubernetes user and group
    users.groups.kubernetes = {};

    users.users.kubernetes = {
        isSystemUser = true;
        group = "kubernetes";
        description = "Kubernetes system user";
        home = "/var/lib/kubernetes";
        createHome = true;
        uid = 900;
    };

    # Longhorn requires nfs-utils and iscsi-initiator-utils
    systemd.services.kubelet = {
      path = [ 
        pkgs.bash 
        pkgs.openiscsi 
        pkgs.nfs-utils
        pkgs.util-linux
        pkgs.gnugrep
        pkgs.gawk
      ];
      serviceConfig = {
        MountFlags = "shared";
      };
    };
}
