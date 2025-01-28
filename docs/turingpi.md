# Install k3s on ubuntu

## Using Ansible
```bash
# Join nodes using token from master
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/k3s-join.yml -e K3S_TOKEN="$K3S_TOKEN"

# Setup nodes
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/setup.yml
```

## Uninstall
```bash
# To only run the uninstall task:
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/k3s-join.yml --tags uninstall
```

## Flash image

```bash 
# Download image
https://firmware.turingpi.com/turing-rk1/ubuntu_22.04_rockchip_linux/

# Uncompress image
xz -d ubuntu-22.04.3-preinstalled-server-arm64-turing-rk1_v1.33.img.xz

# Flash image
tpi flash -i ~/Downloads/ubuntu-22.04.3-preinstalled-server-arm64-turing-rk1_v1.33.img -n 1
```