# Install k3s on ubuntu

## Manual Installation
```bash
TOKEN=(cat /data/k3s-token)
MASTER_IP=192.168.68.100
NODE_NAME=tp1

curl -sfL https://get.k3s.io | K3S_TOKEN="$TOKEN" K3S_URL="$MASTER_IP" K3S_NODE_NAME="$NODE_NAME" sh -
```

## Using Ansible
```bash
# Join nodes using token from master
ansible-playbook -i ansible/inventory.yml ansible/k3s-join.yml -e K3S_TOKEN="$K3S_TOKEN" --ask-pass

# To only run the uninstall task:
ansible-playbook -i ansible/inventory.yml ansible/k3s-join.yml --tags uninstall --ask-pass

# Prepare nodes for longhorn
ansible-playbook -i ansible/inventory.yml ansible/longhorn-prep.yml --ask-pass
```