# Steps

```bash
talosctl apply-config --insecure --mode=interactive --nodes 192.168.68.100
talosctl kubeconfig .  --nodes 192.168.68.100
set -gx KUBECONFIG talos/kubeconfig

helm repo add argo-cd https://argoproj.github.io/argo-helm
helm repo update
helm upgrade -n argocd --install argocd argo-cd/argo-cd -f ./argo-values.yaml --wait --create-namespace --atomic
../hack/secrets.sh
k apply -f./applications.yaml
```


# References

- [Ansible Playbook for Talos](https://github.com/JamesTurland/JimsGarage/tree/main/Ansible/Playbooks/Talos)
    - [Video to go with it](https://www.youtube.com/watch?v=TP8hVq1lCxM)
