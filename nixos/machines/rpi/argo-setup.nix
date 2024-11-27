{ config, pkgs, ... }:

let
  k3sConfig = "/etc/rancher/k3s/k3s.yaml";
  applicationsPath = "/home/nixos/home-systems/k8s/applications.yaml";
  argoValuesPath = "/home/nixos/home-systems/hack/argo-values.yaml";
in
{
  systemd.services.argo-setup = {
    description = "Bootstrap Kubernetes cluster with essential services";
    # To restart this service and apply new values:
    # 1. Delete the argocd release: kubectl delete namespace argocd
    # 2. Restart the service: sudo systemctl restart argo-setup
    after = [ "k3s.service" ];
    wantedBy = [ "multi-user.target" ];
    path = with pkgs; [
      kubernetes-helm
      kubectl
    ];
    environment = {
      KUBECONFIG = k3sConfig;
    };
    script = ''
      # Wait for kubernetes to be ready
      until kubectl get nodes; do
        echo "Waiting for kubernetes to be ready..."
        sleep 5
      done

      # Print argo-values.yaml contents
      echo "Current argo-values.yaml contents:"
      cat ${argoValuesPath} | sed 's/^/  /'

      # Add helm repos
      if ! helm repo list | grep -q "argo-cd"; then
        echo "Adding Argo CD helm repository..."
        helm repo add argo-cd https://argoproj.github.io/argo-helm
        helm repo update
      fi

      echo "Installing/Upgrading Argo CD..."
      helm upgrade --install argocd argo-cd/argo-cd \
        --create-namespace \
        --namespace argocd \
        --values ${argoValuesPath} \
        --wait

      # TODO: Find a way to register private repos

      # Apply root application manifest
      echo "Applying root application manifest..."
      if ! kubectl apply -f ${applicationsPath}; then
        echo "Failed to apply root application"
        exit 1
      fi
      echo "Root application applied successfully."
    '';
    serviceConfig = {
      Type = "oneshot";
      RemainAfterExit = true;
      Restart = "on-failure";
      RestartSec = "30s";
    };
  };
} 