
## Using Ansible
```bash
# There's a ready to use docker image with ansible installed.
docker compose run --rm sh

# Join nodes using token from master
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/k3s-join.yml -e K3S_TOKEN="$K3S_TOKEN" --limit tp1,tp4

# To only run the uninstall task:
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/k3s-join.yml --tags uninstall

# Prepare nodes for longhorn
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/longhorn-prep.yml

# Setup SSH keys
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/ssh-key-setup.yml
```
