# Install k3s on ubuntu

```bash
TOKEN=(sudo cat /var/lib/rancher/k3s/server/node-token)
MASTER_IP=192.168.68.100
NODE_NAME=tp1

curl -sfL https://get.k3s.io | K3S_TOKEN="$TOKEN" K3S_URL="$MASTER_IP" K3S_NODE_NAME="$NODE_NAME" sh -
```

## Adhoc

```bash

curl -sfL https://get.k3s.io | K3S_URL="https://192.168.68.100:6443" K3S_NODE_NAME="tp1" K3S_TOKEN="K10de522f9f5b2d22f6d0eeef77291616863bd9d88615ce9eaf3f07a2ad7f76fb13::server:e30094b5d622d30dbc71a9ae382eb38a" sh -

curl -sfL https://get.k3s.io | K3S_URL="https://192.168.68.100:6443"  K3S_NODE_NAME="tp2"  K3S_TOKEN="K10de522f9f5b2d22f6d0eeef77291616863bd9d88615ce9eaf3f07a2ad7f76fb13::server:e30094b5d622d30dbc71a9ae382eb38a" sh -
```