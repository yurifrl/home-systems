#!/usr/bin/env bash

CLOUDFLARED_DIR="/home/nixos/.cloudflared"

if [ $# -ne 1 ]; then
    echo "Usage: $0 <tunnel-id>"
    echo "Available tunnel IDs:"
    for file in "$CLOUDFLARED_DIR"/*.json; do
        basename "$file" .json
    done
    exit 1
fi

TUNNEL_ID="$1"
JSON_FILE="/home/nixos/.cloudflared/${TUNNEL_ID}.json"

# Check if file exists
if [ ! -f "$JSON_FILE" ]; then
    echo "Error: Tunnel credentials file not found at $JSON_FILE"
    exit 1
fi

# Delete existing secret if it exists
kubectl delete secret -n cloudflare-tunnel generic cloudflare-tunnel-secret --ignore-not-found

# Create the secret with stringData format
kubectl create secret -n cloudflare-tunnel generic cloudflare-tunnel-secret \
    --from-file=credentials.json=/dev/stdin <<EOF | kubectl apply -f -
{
  "stringData": {
    "credentials.json": {
      "AccountTag": "$(jq -r .AccountTag $JSON_FILE)",
      "TunnelID": "$(jq -r .TunnelID $JSON_FILE)",
      "TunnelSecret": "$(jq -r .TunnelSecret $JSON_FILE)"
    }
  }
}
EOF

echo "Secret created/updated successfully!" 