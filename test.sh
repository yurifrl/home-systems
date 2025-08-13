#!/usr/bin/env bash

SECRET='66aff574ed07fbbd7f75b3d6341e3f36568801cfe3326c2f1cad3f47b701c618'
PAYLOAD='{"zen":"ping"}'
SIG="sha256=$(printf '%s' "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" -hex | awk '{print $2}')"

curl -sS -i -X POST https://home-systems-repo-webhook.syscd.live/webhook \
  -H "User-Agent: GitHub-Hookshot" \
  -H "X-GitHub-Event: ping" \
  -H "X-Hub-Signature-256: $SIG" \
  -H "Content-Type: application/json" \
  --data "$PAYLOAD"

