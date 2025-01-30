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

## Raspberry Pi

Your image schematic ID is: 04d4078b3c5d84c71491d9d7acfb48423098225eb2037448a56c0da12cf379a6
```yaml
overlay:
    image: siderolabs/sbc-raspberrypi
    name: rpi_generic
customization:
    systemExtensions:
        officialExtensions:
            - siderolabs/tailscale
```
First Boot
Use the following disk image for Raspberry Pi Series:

Disk Image
https://factory.talos.dev/image/04d4078b3c5d84c71491d9d7acfb48423098225eb2037448a56c0da12cf379a6/v1.9.3/metal-arm64.raw.xz
Upgrading Talos Linux
To upgrade Talos Linux on the machine, use the following image:
factory.talos.dev/installer/04d4078b3c5d84c71491d9d7acfb48423098225eb2037448a56c0da12cf379a6:v1.9.3



## Rockship

Your image schematic ID is: df156b82096feda49406ac03aa44e0ace524b7efe4e1f0e144a1e1ae3930f1c0
```yaml
overlay:
    image: siderolabs/sbc-rockchip
    name: turingrk1
customization: {}
```

First Boot
Use the following disk image for Turing RK1:

Disk Image
https://factory.talos.dev/image/df156b82096feda49406ac03aa44e0ace524b7efe4e1f0e144a1e1ae3930f1c0/v1.9.3/metal-arm64.raw.xz
Upgrading Talos Linux
To upgrade Talos Linux on the machine, use the following image:
factory.talos.dev/installer/df156b82096feda49406ac03aa44e0ace524b7efe4e1f0e144a1e1ae3930f1c0:v1.9.3



# References

- [Ansible Playbook for Talos](https://github.com/JamesTurland/JimsGarage/tree/main/Ansible/Playbooks/Talos)
    - [Video to go with it](https://www.youtube.com/watch?v=TP8hVq1lCxM)
