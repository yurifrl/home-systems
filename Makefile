
argo-update:
	helm template -n argocd argocd argo-cd/argo-cd -f ./manifests/values/argocd.yaml --create-namespace --atomic > ./manifests/argocd.yaml