#!/usr/bin/env bash
# Seed / rotate the single 1Password "workstation" item (Kubernetes vault) that
# the NixOS workstation dev VM self-pulls at bootstrap.
#
# RUN THIS ON YOUR LAPTOP (an authenticated machine), NOT on Proxmox or in the
# cluster. It reads context only your machine has -- `op` signed in, `talosctl`
# config, the docker osxkeychain, the Tailscale OAuth creds -- and WRITES the
# values into 1Password. Proxmox/the cluster never run this; they only READ the
# result (ESO pulls the item into the cluster; the VM's cloud-init consumes it).
#
# Consumed by (crossplane-proxmox chart):
#   templates/workstation-seed-secret.yaml   -> OP_SERVICE_ACCOUNT_TOKEN (the seed)
#   files/workstation-bootstrap.sh           -> TAILSCALE_AUTH_KEY, GITHUB_DEPLOY_KEY,
#                                               KUBECONFIG, REGISTRY_DOCKERCONFIGJSON
#
# WHEN to run:
#   * once, before the first provision, to replace the placeholder field values
#   * again to ROTATE: Tailscale key expiry, Talos cert refresh, deploy-key roll
#   Idempotent — each section overwrites its own field, safe to re-run.
#
# HOW to run:
#   task proxmox:workstation-seed                 # every runnable section
#   task proxmox:workstation-seed -- github       # or a single section
#   (invoked from the crossplane-proxmox chart; run via the task above)
#
# Sources (nothing here mutates infra; it reads local creds / mints a TS key and
# writes them into 1Password):
#   KUBECONFIG                -> talosctl kubeconfig (admin, Talos cluster)
#   REGISTRY_DOCKERCONFIGJSON -> local docker creds in osxkeychain (ghcr.io, docker.io)
#   TAILSCALE_AUTH_KEY        -> tailnet API, minted with tag:workstation
#   GITHUB_DEPLOY_KEY         -> fresh ed25519 keypair (public key printed to register)
#   OP_SERVICE_ACCOUNT_TOKEN  -> NOT auto-set; see note_op_token below
set -euo pipefail

VAULT="${OP_VAULT:-Kubernetes}"
ITEM="workstation"
TALOS_NODE="${TALOS_NODE:-192.168.68.100}"
GITHUB_REPO="${GITHUB_REPO:-yurifrl/nixos}"
DOCKER_REGISTRIES=("ghcr.io" "https://index.docker.io/v1/")

need()      { command -v "$1" >/dev/null || { echo "missing dependency: $1" >&2; exit 1; }; }
set_field() { op item edit "$ITEM" --vault "$VAULT" "$1[password]=$2" >/dev/null && echo "  ✓ $1"; }

seed_kubeconfig() {
  echo "[KUBECONFIG] talosctl kubeconfig (node $TALOS_NODE)"
  need talosctl
  local kcfg; kcfg=$(talosctl kubeconfig - --merge=false --nodes "$TALOS_NODE")
  [ -n "$kcfg" ] || { echo "  empty kubeconfig" >&2; return 1; }
  set_field KUBECONFIG "$kcfg"
}

seed_docker() {
  echo "[REGISTRY_DOCKERCONFIGJSON] osxkeychain -> dockerconfigjson"
  need docker-credential-osxkeychain; need jq
  local auths='{}' reg cred user secret auth
  for reg in "${DOCKER_REGISTRIES[@]}"; do
    if cred=$(printf '%s' "$reg" | docker-credential-osxkeychain get 2>/dev/null); then
      user=$(jq -r '.Username' <<<"$cred")
      secret=$(jq -r '.Secret'  <<<"$cred")
      auth=$(printf '%s:%s' "$user" "$secret" | base64 | tr -d '\n')
      auths=$(jq --arg reg "$reg" --arg auth "$auth" '.[$reg] = {auth: $auth}' <<<"$auths")
      echo "  + $reg ($user)"
    else
      echo "  - $reg (no keychain entry, skipped)"
    fi
  done
  set_field REGISTRY_DOCKERCONFIGJSON "$(jq -cn --argjson a "$auths" '{auths: $a}')"
}

seed_tailscale() {
  echo "[TAILSCALE_AUTH_KEY] mint tag:workstation key via tailnet API"
  need curl; need jq
  local cid csec tok resp key
  # OAuth client that OWNS tag:workstation (Auth Keys: write). Stored in the
  # workstation item; falls back to the legacy operator client if unset.
  cid=$(op read "op://$VAULT/workstation/TS_OAUTH_CLIENT_ID" 2>/dev/null || op read "op://$VAULT/tailscale-operator-oauth/client_id")
  csec=$(op read "op://$VAULT/workstation/TS_OAUTH_CLIENT_SECRET" 2>/dev/null || op read "op://$VAULT/tailscale-operator-oauth/client_secret")
  tok=$(curl -sf -d "client_id=$cid" -d "client_secret=$csec" \
        https://api.tailscale.com/api/v2/oauth/token | jq -r '.access_token // empty')
  [ -n "$tok" ] || { echo "  OAuth token request failed (client scope?)" >&2; return 1; }
  resp=$(curl -s -H "Authorization: Bearer $tok" -H "Content-Type: application/json" \
         -X POST https://api.tailscale.com/api/v2/tailnet/-/keys \
         -d '{"capabilities":{"devices":{"create":{"reusable":false,"ephemeral":false,"preauthorized":true,"tags":["tag:workstation"]}}}}')
  key=$(jq -r '.key // empty' <<<"$resp")
  [ -n "$key" ] || {
    echo "  mint failed: $(jq -c . <<<"$resp")" >&2
    echo "  -> apply the tailnet ACL (tagOwners tag:workstation) and grant this OAuth" >&2
    echo "     client ownership of tag:workstation, then re-run." >&2
    return 1
  }
  set_field TAILSCALE_AUTH_KEY "$key"
}

seed_passwords() {
  echo "[PASSWORDS] root + yuri-workstation login passwords (idempotent, generate-if-absent)"
  need openssl
  local f pw
  for f in ROOT_PASSWORD YURI_WORKSTATION_PASSWORD; do
    if op read "op://$VAULT/$ITEM/$f" >/dev/null 2>&1; then
      echo "  = $f already set (kept; delete the field in 1Password to rotate)"
    else
      pw=$(openssl rand -base64 32 | LC_ALL=C tr -dc "A-Za-z0-9"); pw=${pw:0:20}
      [ ${#pw} -eq 20 ] || { echo "  password gen failed" >&2; return 1; }
      set_field "$f" "$pw"
    fi
  done
}

seed_github() {
  echo "[GITHUB_DEPLOY_KEY] generate ed25519 deploy keypair"
  need ssh-keygen
  local tmp; tmp=$(mktemp -d)
  ssh-keygen -q -t ed25519 -N "" -C "workstation-deploy@$GITHUB_REPO" -f "$tmp/id"
  set_field GITHUB_DEPLOY_KEY "$(cat "$tmp/id")"
  echo "  register this READ-ONLY deploy key at:"
  echo "    https://github.com/$GITHUB_REPO/settings/keys/new"
  echo "    $(cat "$tmp/id.pub")"
  rm -rf "$tmp"
}

note_op_token() {
  cat <<'EOF'
[OP_SERVICE_ACCOUNT_TOKEN] not auto-set.
  The Kubernetes vault is in a 1Password FAMILY account, which cannot mint
  Service Accounts (Business-only), and your ESO reads it via 1Password Connect
  (op-credentials / OP_CONNECT_TOKEN). Decide the box's seed model:
    (a) point the on-box `op` at Connect (OP_CONNECT_HOST + OP_CONNECT_TOKEN,
        reusing op-credentials) instead of a service-account token, or
    (b) mint a real SA in the nsx-team Business account and mirror the item there.
EOF
}

main() {
  need op
  local secs=("$@"); [ ${#secs[@]} -eq 0 ] && secs=(passwords kubeconfig docker tailscale github)
  local rc=0
  for s in "${secs[@]}"; do
    case "$s" in
      passwords)  seed_passwords  || rc=1;;
      kubeconfig) seed_kubeconfig || rc=1;;
      docker)     seed_docker     || rc=1;;
      tailscale)  seed_tailscale  || rc=1;;
      github)     seed_github     || rc=1;;
      *) echo "unknown section: $s (passwords|kubeconfig|docker|tailscale|github)" >&2; rc=1;;
    esac
  done
  note_op_token
  return $rc
}
main "$@"
