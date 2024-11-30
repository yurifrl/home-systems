#!/usr/bin/env bash

# Stop K3s service
sudo systemctl stop k3s


# Remove K3s data directory
sudo rm -rf /var/lib/rancher/k3s

# Remove K3s configuration
sudo rm -rf /etc/rancher/k3s

# Remove K3s logs
sudo rm -rf /var/log/k3s.log

echo "K3s has been stopped and purged of all state."