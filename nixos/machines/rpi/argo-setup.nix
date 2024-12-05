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
    after = [ "k3s.service" ];
    requires = [ "k3s.service" ];
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
      echo "and then: helm upgrade -n argocd --install argocd argo-cd/argo-cd -f ${argoValuesPath} --wait --create-namespace --atomic"
      echo
    
      # Add helm repos
      echo "Checking if Argo CD helm repository is already added..."
      if ! helm repo list | grep -q 'argo-cd'; then
        echo "Adding Argo CD helm repository..."
        helm repo add argo-cd https://argoproj.github.io/argo-helm
      else
        echo "Argo CD helm repository already exists."
      fi
      helm repo update

      # Initialize failure tracking
      failed_steps=""
      failure_count=0

      # Install/Upgrade Argo CD
      echo "Installing/Upgrading Argo CD..."
      if ! helm upgrade --install argocd argo-cd/argo-cd \
        --create-namespace \
        --namespace argocd \
        --values ${argoValuesPath} \
        --atomic \
        --wait; then
        echo "Failed to install/upgrade Argo CD."
        failed_steps="$failed_steps\n- Helm install/upgrade"
        failure_count=$((failure_count + 1))
      fi

      # Apply root application manifest
      echo "Applying root application manifest..."
      if ! kubectl apply -f ${applicationsPath}; then
        echo "Failed to apply root application."
        failed_steps="$failed_steps\n- Root application deployment"
        failure_count=$((failure_count + 1))
      else
        echo "Root application applied successfully."
      fi

      # Wait for ArgoCD server to be ready
      echo "Waiting for ArgoCD server to be ready..."
      if ! kubectl -n argocd wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server --timeout=300s; then
        echo "ArgoCD server not ready after timeout."
        failed_steps="$failed_steps\n- ArgoCD server readiness"
        failure_count=$((failure_count + 1))
      else
        echo "ArgoCD server is ready"
      fi

      # Check if all steps failed
      if [ "$failure_count" -eq 3 ]; then
        echo "All critical steps failed:"
        echo -e "$failed_steps"
        echo "Deleting namespace and restarting service..."
        kubectl delete namespace argocd
        systemctl restart argo-setup
        exit 1
      elif [ "$failure_count" -gt 0 ]; then
        echo "Some steps failed ($failure_count failures):"
        echo -e "$failed_steps"
      fi

      # Print additional information
      echo "Listing installed Helm releases in 'argocd' namespace..."
      helm list -n argocd

      echo "Listing ConfigMaps in 'argocd' namespace..."
      kubectl get configmaps -n argocd
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