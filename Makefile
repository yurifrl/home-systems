.PHONY: all argo-update repo-add
all: repo-add argo-update

repo-add:
	helm repo add argo-cd https://argoproj.github.io/argo-helm

argo-update:
	helm template -n argocd argocd argo-cd/argo-cd -f ./manifests/values/argocd.yaml --create-namespace --atomic > ./manifests/argocd.yaml

apply:
	talosctl -n 192.168.68.100 apply-config -f talos/config/controlplane.yaml -p  @talos/config/patches.yaml
	talosctl -n 192.168.68.114,192.168.68.107 apply-config -f talos/config/worker.yaml
	./talos/secrets-backup.sh