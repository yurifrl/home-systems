#!/usr/bin/env bash

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