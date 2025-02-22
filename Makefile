.PHONY: all argo-update repo-add
all: repo-add argo-update

repo-add:
	helm repo add argo-cd https://argoproj.github.io/argo-helm
	helm repo update

argo-update:
	helm template -n argocd argocd argo-cd/argo-cd -f ./manifests/values/argocd.yaml --create-namespace --atomic > ./manifests/argocd.yaml

apply:
	talosctl -n 192.168.68.100 apply-config -f talos/config/controlplane.yaml  -p  @talos/config/patches.yaml -p "$(cat talos/config/external-secrets.yaml | op inject)"
	talosctl -n 192.168.68.114 apply-config -f talos/config/tp4.yaml
	talosctl -n 192.168.68.107 apply-config -f talos/config/tp1.yaml
	talosctl -n 192.168.68.112 apply-config -f talos/config/pc01.yaml
	./talos/secrets-backup.sh


label-nodes:
	# For tp1
	kubectl label nodes talos-k3p-bue syscd.dev/storage=tp1 --overwrite

	# For tp4
	kubectl label nodes talos-jhz-4tr syscd.dev/storage=tp4 --overwrite

	# For rp1
	kubectl label nodes rpi syscd.dev/storage=rpi

	kubectl get nodes --show-labels | grep syscd.dev/storage