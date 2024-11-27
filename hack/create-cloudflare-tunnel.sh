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

# Create the secret with the exact format
kubectl create secret generic cloudflare-tunnel-credentials \
    --from-literal=credentials.json="{
      \"AccountTag\": \"$(jq -r .AccountTag $JSON_FILE)\",
      \"TunnelID\": \"$(jq -r .TunnelID $JSON_FILE)\",
      \"TunnelSecret\": \"$(jq -r .TunnelSecret $JSON_FILE)\"
    }" \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Secret created/updated successfully!" 