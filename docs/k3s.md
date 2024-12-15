# K3s


## Change k3s token

Put token in `/data/k3s-token`

```bash
# Stop k3s
sudo systemctl stop k3s

# Clean up k3s data
sudo rm -rf /var/lib/rancher/k3s/server/db/
sudo rm -rf /var/lib/rancher/k3s/server/tls/
sudo rm -rf /var/lib/rancher/k3s/server/cred/
sudo rm -rf /var/lib/rancher/k3s/server/token

# Start k3s again
sudo systemctl start k3s
```