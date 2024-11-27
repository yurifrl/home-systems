{ config, pkgs, ... }:

let
  k3sConfig = "/etc/rancher/k3s/k3s.yaml";
  applicationsPath = "/home/nixos/home-systems/k8s/applications.yaml";
  argoValues = ''
    server:
      ingress:
        enabled: true
  '';
in
{
  systemd.services.argo-setup = {
    description = "Bootstrap Kubernetes cluster with essential services";
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

      # Add helm repos
      if ! helm repo list | grep -q "argo-cd"; then
        echo "Adding Argo CD helm repository..."
        helm repo add argo-cd https://argoproj.github.io/argo-helm
        helm repo update
      fi

      # Create values.yaml from literal
      cat > /tmp/argocd-values.yaml <<EOF
      ${argoValues}
      EOF

      # Install/Upgrade ArgoCD
      if ! helm list -n argocd | grep -q "argocd"; then
        echo "Installing/Upgrading Argo CD..."
        helm upgrade --install argocd argo-cd/argo-cd \
          --create-namespace \
          --namespace argocd \
          --values /tmp/argocd-values.yaml \
          --wait
      fi

      # Create public repository reference
      if ! kubectl get secret home-systems-repo -n argocd; then
        echo "Creating public repository reference..."
        kubectl create secret generic home-systems-repo \
          --namespace argocd \
          --from-literal=url=https://github.com/yurifrl/home-systems \
          --from-literal=type=git || {
            echo "Failed to create repository reference"
            exit 1
          }
      fi

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