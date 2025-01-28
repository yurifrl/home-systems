# Using Ansible
```bash
# There's a ready to use docker image with ansible installed.
docker compose run --rm sh

# Setup nixos machine
ansible-playbook -i ansible/inventory.yml ansible/new-machine-setup.yml

# Setup nodes
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/setup.yml

# Join nodes using token from master
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/k3s-join.yml
```

## Uninstall
```bash
# To only run the uninstall task:
ansible-playbook --ask-pass -i ansible/inventory.yml ansible/k3s-join.yml --tags uninstall
```