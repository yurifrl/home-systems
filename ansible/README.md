
# Using Ansible
```bash
# There's a ready to use docker image with ansible installed.
docker compose run --rm sh

# Join nodes using token from master
ansible-playbook --ask-pass -i ansible/inventory.yaml ansible/k3s-join.yaml -e K3S_TOKEN="$K3S_TOKEN" --limit tp1,tp4

# Setup storage
ansible-playbook --ask-pass -i ansible/inventory.yaml ansible/storage.yaml ansible/timezone.yaml ansible/longhorn-prep.yaml

# Setup SSH keys
ansible-playbook --ask-pass -i ansible/inventory.yaml ansible/ssh-key-setup.yaml
```

## Uninstall
```bash
ansible-playbook --ask-pass -i ansible/inventory.yaml ansible/k3s-join.yaml --tags uninstall
```