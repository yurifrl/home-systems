#!/usr/bin/env bash
# workstation-bootstrap.sh -- box-pulls-own-secrets bootstrap for the NixOS
# workstation dev VM (design.md §4).
#
# Contract: the ONLY secret handed to the box at provision time is a 1Password
# Service-Account token, delivered as $OP_SERVICE_ACCOUNT_TOKEN (injected by the
# workstation VM's cloud-init from the `workstation-op-sa-token` Secret; see
# ../templates/workstation-seed-secret.yaml). Using just that token + `op`, this
# script self-pulls every downstream secret and writes it to its on-box home.
#
# Seed-token scope (keep MINIMAL): a read-only 1Password Service Account with
# access to ONLY the vault holding these items -- the "kubernetes" vault that
# the cluster's onepassword ClusterSecretStore already uses. Do NOT grant it
# tenant-wide vault access. One vault, read-only, nothing else.
#
# Idempotent: safe to re-run (every boot / nixos-rebuild). Every fetch is an
# overwrite installed 0600. Kubeconfig targets api.k8s.lan:6443 (design.md §3).

set -euo pipefail

# --- 1Password references the box pulls (vault "kubernetes"). --------------
# op:// refs -- fill in the real item names when the vault items are created.
OP_VAULT="${OP_VAULT:-kubernetes}"
TS_AUTHKEY_REF="op://${OP_VAULT}/workstation-tailscale/authkey"
GH_DEPLOY_KEY_REF="op://${OP_VAULT}/workstation-github-deploy-key/private-key"
KUBECONFIG_REF="op://${OP_VAULT}/workstation-kubeconfig/kubeconfig"
REGISTRY_CREDS_REF="op://${OP_VAULT}/workstation-registry-creds/dockerconfigjson"

# --- on-box destinations ---------------------------------------------------
TS_AUTHKEY_FILE="/etc/tailscale/authkey"
GH_DEPLOY_KEY_FILE="/root/.ssh/id_workstation_deploy"
KUBECONFIG_FILE="/root/.kube/config"
REGISTRY_CREDS_FILE="/root/.docker/config.json"

# write_secret <op-ref> <dest> <mode> -- op read the ref, install atomically.
write_secret() {
  local ref="$1" dest="$2" mode="$3" tmp
  install -d -m 700 "$(dirname "$dest")"
  tmp="$(mktemp)"
  op read "$ref" >"$tmp"
  install -m "$mode" "$tmp" "$dest"
  rm -f "$tmp"
  echo "bootstrap: wrote $dest"
}

# --- self-check: stub op, assert write_secret installs a 0600 file. --------
if [[ "${1:-}" == "--selftest" ]]; then
  op() { printf 'SECRET-FOR-%s' "$2"; }
  d="$(mktemp -d)"
  write_secret "op://v/i/f" "$d/sub/secret" 0600 >/dev/null
  [[ -f "$d/sub/secret" ]] || { echo "selftest FAIL: file not written" >&2; exit 1; }
  [[ "$(stat -c '%a' "$d/sub/secret" 2>/dev/null || stat -f '%Lp' "$d/sub/secret")" == "600" ]] \
    || { echo "selftest FAIL: perms not 0600" >&2; exit 1; }
  [[ "$(cat "$d/sub/secret")" == "SECRET-FOR-op://v/i/f" ]] || { echo "selftest FAIL: content" >&2; exit 1; }
  rm -rf "$d"
  echo "selftest OK"
  exit 0
fi

command -v op >/dev/null 2>&1 || { echo "bootstrap: missing op (1Password CLI)" >&2; exit 1; }
: "${OP_SERVICE_ACCOUNT_TOKEN:?bootstrap: OP_SERVICE_ACCOUNT_TOKEN not set (seed secret missing)}"
export OP_SERVICE_ACCOUNT_TOKEN

write_secret "$TS_AUTHKEY_REF"     "$TS_AUTHKEY_FILE"     0600
write_secret "$GH_DEPLOY_KEY_REF"  "$GH_DEPLOY_KEY_FILE"  0600
write_secret "$KUBECONFIG_REF"     "$KUBECONFIG_FILE"     0600
write_secret "$REGISTRY_CREDS_REF" "$REGISTRY_CREDS_FILE" 0600

# join the tailnet with the freshly-pulled key (tagged node, see design.md §5).
if command -v tailscale >/dev/null 2>&1; then
  tailscale up --auth-key "$(cat "$TS_AUTHKEY_FILE")" --ssh --hostname workstation
fi

echo "bootstrap: done -- box self-provisioned from the seed token."
