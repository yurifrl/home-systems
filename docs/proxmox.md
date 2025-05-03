
```bash

vim /etc/default/grub
# GRUB_CMDLINE_LINUX_DEFAULT="quiet amd_iommu=on"

update-grub

tee /etc/modules > /dev/null <<EOF
vfio
vfio_iommu_type1
vfio_pci
vfio_virqfd
EOF

```


## Nvidia

- https://www.reddit.com/r/Proxmox/comments/1enqeu3/pci_gpu_passthrough_on_talos_linux_vms/?share_id=jgI_gfdATvXkbTmJwVnjw&utm_content=2&utm_medium=ios_app&utm_name=ioscss&utm_source=share&utm_term=1