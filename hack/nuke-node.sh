#!/usr/bin/env bash

# Check if running on nixos-1
if [[ $(hostname) != "nixos-1" ]]; then
    echo "Error: This script must be run on nixos-1"
    exit 1
fi

# Prompt for confirmation
echo "WARNING: This will completely remove K3s and all associated data."
echo "Are you sure you want to continue? (y/n)"
read -r response
if [[ ! $response =~ ^[Yy]$ ]]; then
    echo "Operation cancelled."
    exit 0
fi

# Stop K3s service
sudo systemctl stop k3s
sudo systemctl stop secret-loader 
sudo systemctl stop argo-setup

# Remove K3s configuration
sudo rm -rf /etc/rancher/k3s
sudo rm -rf /tmp/rancher/k3s