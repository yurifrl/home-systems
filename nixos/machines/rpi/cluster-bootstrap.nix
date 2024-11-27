{ config, pkgs, ... }:

{
  # This is for the main node only
  systemd.services.cluster-bootstrap = {
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
} 