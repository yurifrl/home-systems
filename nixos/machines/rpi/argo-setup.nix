{ config, pkgs, ... }:

let
  k3sConfig = "/etc/rancher/k3s/k3s.yaml";
  applicationsPath = "/home/nixos/home-systems/k8s/applications.yaml";
  argoValuesPath = "/home/nixos/home-systems/k8s/values/argo.yaml";
in
{
  systemd.services.argo-setup = {
    description = "Bootstrap Kubernetes cluster with essential services";
    # To restart this service and apply new values:
    # 1. Delete the argocd release: kubectl delete namespace argocd
    # 2. Restart the service: sudo systemctl restart argo-setup
    after = [ "k3s.service" "secret-loader.service" ];
    requires = [ "secret-loader.service" ];
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

      # Print values argo.yaml contents
      echo "Current argo values.yaml contents:"
      cat ${argoValuesPath} | sed 's/^/  /'

      echo "Will run: helm repo add argo-cd https://argoproj.github.io/argo-helm; helm repo update"
      echo "and then: helm upgrade -n argocd --install argocd argo-cd/argo-cd -f ${argoValuesPath} --wait --create-namespace"
      echo
    
      # Add helm repos
      echo "Adding Argo CD helm repository..."
      helm repo add argo-cd https://argoproj.github.io/argo-helm
      helm repo update

      echo "Installing/Upgrading Argo CD..."
      helm upgrade --install argocd argo-cd/argo-cd \
        --create-namespace \
        --namespace argocd \
        --values ${argoValuesPath} \
        --wait

      # Wait for ArgoCD server to be ready
      echo "Waiting for ArgoCD server to be ready..."
      until kubectl -n argocd wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server --timeout=300s; do
        echo "ArgoCD server not ready yet, retrying..."
        sleep 10
      done
      echo "ArgoCD server is ready"

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
      RemainAfterExit = "yes";
      TimeoutStartSec = "0";
      
      # Add more robust error handling
      Restart = "on-failure";
      RestartSec = "30s";
      # Limit restart attempts to 5 times within 10 minutes
      StartLimitIntervalSec = "600";
      StartLimitBurst = "5";
      
      # Run as root to ensure proper permissions
      User = "root";
    };

    # Add restart triggers
    restartIfChanged = true;
    restartTriggers = [ applicationsPath argoValuesPath ];
  };
} 