#!/usr/bin/env fish

# Colors for output
set RED '\033[0;31m'
set GREEN '\033[0;32m'
set YELLOW '\033[1;33m'
set NC '\033[0m' # No Color

# Target machine IP
set TARGET_IP "192.168.68.100"

echo -e "$GREEN"Starting new machine setup..."$NC"

# =====================================
# Step 1: Get private key from 1Password
# =====================================
echo -e "$YELLOW"Getting private key from 1Password..."$NC"
op item get "GithubAutomation" --fields "private key" --reveal | tr -d '"' | awk '/BEGIN/,/END/' > /tmp/id_ed25519
chmod 600 /tmp/id_ed25519

# =====================================
# Step 2: Copy keys to machine
# =====================================
echo -e "$YELLOW"Copying SSH keys to machine..."$NC"
ssh root@$TARGET_IP "
    mkdir -p /home/nixos/.ssh
    chown -R nixos:users /home/nixos/.ssh
    chmod 700 /home/nixos/.ssh
"
scp -O /tmp/id_ed25519 root@$TARGET_IP:/home/nixos/.ssh/id_ed25519
ssh root@$TARGET_IP "
    chmod 600 /home/nixos/.ssh/id_ed25519
    chown nixos:users /home/nixos/.ssh/id_ed25519
"

# =====================================
# Step 4: Initial system configuration
# =====================================
echo -e "$YELLOW"Performing initial system configuration this may take a while and it will look like it\'s frozen ..."$NC"
ssh nixos@$TARGET_IP "
  # Add unstable channel
  sudo nix-channel --add https://nixos.org/channels/nixpkgs-unstable nixpkgs-unstable
  sudo nix-channel --update

  # Clone repository
  cd /home/nixos
  git clone git@github.com:yurifrl/home-systems.git
  cd home-systems/nixos
"

# =====================================
# Step 3: Copy secrets to machine
# =====================================
echo -e "$YELLOW"Copying secrets to machine..."$NC"
ssh root@$TARGET_IP "
    mkdir -p /data
    chown nixos:users /data
"
scp hack/secrets.sh root@$TARGET_IP:/data/secrets.sh
ssh root@$TARGET_IP "
    chmod 644 /data/secrets.sh
    chown nixos:users /data/secrets.sh
"

# =====================================
# Step 5: Set up Kubernetes configuration
# =====================================
echo -e "$YELLOW"Setting up Kubernetes configuration..."$NC"
mkdir -p ~/.kube
scp root@$TARGET_IP:/etc/rancher/k3s/k3s.yaml ~/.kube/k3s.yaml
sudo chmod 644 ~/.kube/k3s.yaml
# Replace default server IP with actual IP
sed -i "s|server: https://0.0.0.0:6443|server: https://$TARGET_IP:6443|" ~/.kube/k3s.yaml
set -x KUBECONFIG ~/.kube/k3s.yaml

# =====================================
# Step 6: Wait for node to be ready
# =====================================
echo -e "$YELLOW"Waiting for Kubernetes node to be ready..."$NC"
kubectl wait --for=condition=ready node nixos-1 --timeout=300s

# =====================================
# Step 7: Verify setup
# =====================================
echo -e "$YELLOW"Verifying setup..."$NC"
kubectl get nodes

# Final message
echo -e "$GREEN"Setup complete! Your new machine is ready."$NC"
echo -e "$YELLOW"Next steps:"$NC"
echo "1. Check ArgoCD setup: sudo systemctl status argo-setup"
echo "2. Check secret loader: sudo systemctl status secret-loader"
echo "3. Access the system: ssh nixos@$TARGET_IP"

echo -e "$YELLOW"Useful commands:"$NC"
echo "
# Status
sudo systemctl status tailscale-autoconnect
sudo systemctl status argo-setup
sudo systemctl status secret-loader

# Monitor logs
sudo journalctl -u tailscale-autoconnect -f
sudo journalctl -u argo-setup -f
sudo journalctl -u secret-loader -f

# Restart and monitor services
sudo systemctl restart argo-setup & sudo journalctl -u argo-setup -f
sudo systemctl restart secret-loader & sudo journalctl -u secret-loader -f
sudo systemctl restart tailscale-autoconnect & sudo journalctl -u tailscale-autoconnect -f

# System configuration
sudo nixos-rebuild switch --flake .#rpi --impure --show-trace

# End
"
