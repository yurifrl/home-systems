{config, lib, pkgs, ...}:

{
  options = {
    services.k3s-cleanup = {
      enable = lib.mkEnableOption "k3s cleanup service";
    };
  };

  config = lib.mkIf config.services.k3s-cleanup.enable {
    systemd.services.k3s-cleanup = {
      description = "Cleanup k3s resources before start";
      before = [ "k3s.service" ];
      wantedBy = [ "k3s.service" ];
      path = with pkgs; [ util-linux procps ];
      
      serviceConfig = {
        Type = "oneshot";
        RemainAfterExit = "yes";
        ExecStart = pkgs.writeShellScript "k3s-cleanup" ''
          # Kill any remaining containerd-shim processes
          pkill containerd-shim || true
          
          # Give processes time to terminate
          sleep 2
          
          # Unmount any containerd rootfs mounts first
          for m in $(mount | grep '/run/k3s/containerd.*rootfs' | awk '{print $3}'); do
            umount -R "$m" || true
          done
          
          # Unmount any remaining shm mounts
          for m in $(mount | grep '/run/k3s/containerd' | awk '{print $3}'); do
            umount -R "$m" || true
          done
          
          # Remove containerd socket if it exists
          rm -f /run/k3s/containerd/containerd.sock
          
          # Clean up any stale containerd state
          rm -rf /run/k3s/containerd/*
          rm -rf /var/lib/rancher/k3s/agent/containerd/*
        '';
      };
    };
  };
} 