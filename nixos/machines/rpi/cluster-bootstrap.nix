{ config, pkgs, ... }:

{
  # This is for the main node only
  # This argo will be used to install everything else from a source like github.com/
  systemd.services.argo-setup = {
    description = "Bootstrap Kubernetes cluster with essential services";
    after = [ "k3s.service" ];
    wantedBy = [ "multi-user.target" ];
    path = with pkgs; [
      kubernetes-helm
      kubectl
    ];
    environment = {
      KUBECONFIG = "/etc/rancher/k3s/k3s.yaml";
    };
    script = ''
      # Wait for kubernetes to be ready
      until kubectl get nodes; do
        echo "Waiting for kubernetes to be ready..."
        sleep 5
      done

      # Add helm repos
      if ! helm repo list | grep -q "argo-cd"; then
        echo "Adding Argo CD helm repository..."
        helm repo add argo-cd https://argoproj.github.io/argo-helm
        helm repo update
      fi

      # Install/Upgrade ArgoCD
      if ! helm list -n argocd | grep -q "argocd"; then
        echo "Installing/Upgrading Argo CD..."

        helm upgrade --install argocd argo-cd/argo-cd \
          --create-namespace \
          --namespace argocd \
          --set server.ingress.enabled=true \
          --wait
      fi

      # Add additional cluster bootstrap steps here
      # For example:
      # - Install cert-manager
      # - Install ingress controller
      # - Install monitoring stack
    '';
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      Restart = "on-failure";
      RestartSec = "30s";
    };
  };

  # This is for bootstraped things that are to be installed via argo
  # This will be used to install things like cert-manager, istio, etc.
  systemd.services.cluster-bootstrap = {
    description = "Watch k8s directory for changes and apply";
    wantedBy = [ "multi-user.target" ];
    path = with pkgs; [
      inotify-tools
      kubectl
    ];
    environment = {
      KUBECONFIG = "/etc/rancher/k3s/k3s.yaml";
      WATCH_DIR = "/home/nixos/k8s";
    };
    script = ''
      echo "Starting cluster-bootstrap service..."
      mkdir -p $WATCH_DIR
      
      # Initial apply of all kubernetes manifests
      echo "Performing initial apply of kubernetes manifests..."
      kubectl apply -f $WATCH_DIR
      
      echo "Watching directory: $WATCH_DIR"
      while true; do
        echo "Waiting for changes in $WATCH_DIR..."
        inotifywait -r -e modify,create,delete,move --include '\.yaml$' $WATCH_DIR
        echo "Change detected in k8s directory, applying changes..."
        kubectl apply -f $WATCH_DIR
        echo "Changes applied successfully."
      done
    '';
    serviceConfig = {
      Type = "simple";
      Restart = "always";
      RestartSec = "10s";
    };
    after = [ "argo-setup.service" ];
  };
} 