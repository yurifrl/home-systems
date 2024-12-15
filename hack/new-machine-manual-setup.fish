#!/usr/bin/env fish

# Colors for output
set RED '\033[0;31m'
set GREEN '\033[0;32m'
set YELLOW '\033[1;33m'
set NC '\033[0m' # No Color

# Target machine IP
set TARGET_IP "192.168.68.100"

function run_step
    set -l step_name $argv[1]
    set -l step_commands $argv[2]
    
    echo -e "$YELLOW$step_name"$NC"
Commands to be executed:
$GREEN"
    echo $step_commands | tr ';' '\n' | sed 's/^[[:space:]]*//' | grep -v '^$'
    echo -e "$NC
Press ENTER to run, 's' to skip, or Ctrl+C to exit"
    
    read -l response
    if test "$response" = "s"
        echo -e "$YELLOW"Skipping..."$NC"
        return
    end
    
    eval $step_commands
end

echo -e "$GREEN"Starting new machine setup..."$NC"

# =====================================
# Step 1: Get private key from 1Password
# =====================================
set step1_commands "
    op item get 'GithubAutomation' --fields 'private key' --reveal | tr -d '\"' | awk '/BEGIN/,/END/' > /tmp/id_ed25519;
    chmod 600 /tmp/id_ed25519;
"
run_step "Getting private key from 1Password..." $step1_commands

# =====================================
# Step 2: Copy keys to machine
# =====================================
set step2_commands "
    ssh root@$TARGET_IP \"
        mkdir -p /home/nixos/.ssh;
        chown -R nixos:users /home/nixos/.ssh;
        chmod 700 /home/nixos/.ssh;
    \";
    scp -O /tmp/id_ed25519 root@$TARGET_IP:/home/nixos/.ssh/id_ed25519;
    ssh root@$TARGET_IP \"
        chmod 600 /home/nixos/.ssh/id_ed25519;
        chown nixos:users /home/nixos/.ssh/id_ed25519;
    \";
"
run_step "Copying SSH keys to machine..." $step2_commands

# =====================================
# Step 3: Update nix channels
# =====================================
set step3_commands "
    ssh nixos@$TARGET_IP \"
        sudo nix-channel --add https://nixos.org/channels/nixpkgs-unstable nixpkgs-unstable && sudo nix-channel --update;
    \";
"
run_step "Updating nix channels..." $step3_commands

# =====================================
# Step 4: Clone repository
# =====================================
set step4_commands "
    ssh nixos@$TARGET_IP \"
        cd /home/nixos;
        git clone git@github.com:yurifrl/home-systems.git;
        cd home-systems/nixos;
    \";
    scp ~/.gitconfig root@$TARGET_IP:/home/nixos/.gitconfig
"
run_step "Cloning repository..." $step4_commands

# =====================================
# Step 5: Copy secrets to machine
# =====================================
set step5_commands "
    ssh root@$TARGET_IP \"
        mkdir -p /data;
        chown nixos:users /data;
    \";
    scp hack/secrets.sh root@$TARGET_IP:/data/secrets.sh;
    ssh root@$TARGET_IP \"
        chmod 644 /data/secrets.sh;
        chown nixos:users /data/secrets.sh;
    \";
"
run_step "Copying secrets to machine..." $step5_commands

# =====================================
# Step 6: Set up Kubernetes configuration
# =====================================
set step6_commands "
    mkdir -p ~/.kube;
    scp root@$TARGET_IP:/etc/rancher/k3s/k3s.yaml ~/.kube/k3s.yaml;
    sudo chmod 644 ~/.kube/k3s.yaml;
    sed -i \"s|server: https://0.0.0.0:6443|server: https://$TARGET_IP:6443|\" ~/.kube/k3s.yaml;
    set -x KUBECONFIG ~/.kube/k3s.yaml;
"
run_step "Setting up Kubernetes configuration..." $step6_commands

# =====================================
# Step 7: Wait for node to be ready
# =====================================
set step7_commands "kubectl wait --for=condition=ready node nixos-1 --timeout=300s;"
run_step "Waiting for Kubernetes node to be ready..." $step7_commands

# =====================================
# Step 8: Verify setup
# =====================================
set step8_commands "kubectl get nodes;"
run_step "Verifying setup..." $step8_commands

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
sudo nixos-rebuild switch --flake /home/nixos/home-systems/nixos#rpi --impure --show-trace
"
