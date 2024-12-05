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

# Remove K3s data directory
sudo rm -rf /var/lib/rancher/k3s

# Remove K3s configuration
sudo rm -rf /etc/rancher/k3s

# Remove K3s logs
sudo rm -rf /var/log/k3s.log

# Remove Rook
# https://brettweir.com/blog/remove-rook-from-cluster/
# Remove rook directory
sudo rm -rvf /var/lib/rook
# Remove logical volumes
sudo lvs --noheadings -o lv_path | grep ceph- | xargs -r -I {} sudo lvremove -f {}

# Remove volume groups
sudo vgs --noheadings -o vg_name | grep ceph- | xargs -r -I {} sudo vgremove -f {}

# Remove physical volume labels
sudo pvs --noheadings -o pv_name | xargs -r -I {} sudo pvremove -f {} 
echo "K3s has been stopped and purged of all state."