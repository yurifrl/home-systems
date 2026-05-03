#!/usr/bin/env bash
# Render a node's machineconfig from its template, injecting 1Password secrets.
# Usage:
#   scripts/pxe/2-render-config.sh dell01
# Writes to pxe/assets/configs/<MAC>.yaml (keyed by MAC without separators,
# matching iPXE's ${mac:hexhyp} format used in boot.ipxe).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

NODE="${1:-}"
if [ -z "$NODE" ]; then
  echo "usage: $0 <node-name>" >&2
  exit 2
fi

PXE_DIR="${PXE_DIR:-${REPO_ROOT}/pxe}"
TEMPLATE="${REPO_ROOT}/talos/templates/${NODE}.yaml"
if [ ! -s "${TEMPLATE}" ]; then
  echo "ERROR: template not found: ${TEMPLATE}" >&2
  echo "Create it in talos/templates/${NODE}.yaml first." >&2
  exit 1
fi

# Look up node MAC in nodes.yaml (simple grep, no yq dependency)
MAC=$(awk -v n="${NODE}:" '
  $0 == "  " n { found=1; next }
  found && /mac:/ { gsub(/[",]/,"",$2); print $2; exit }
' "${PXE_DIR}/nodes.yaml")

if [ -z "${MAC}" ]; then
  echo "ERROR: no mac entry for node '${NODE}' in pxe/nodes.yaml" >&2
  exit 1
fi

# iPXE ${mac:hexhyp} renders as d0-94-66-d9-eb-a5
MAC_HYPHEN=$(echo "${MAC}" | tr ':' '-' | tr '[:upper:]' '[:lower:]')

CONFIGS_DIR="${PXE_DIR}/assets/configs"
OUT="${CONFIGS_DIR}/${MAC_HYPHEN}.yaml"
mkdir -p "${CONFIGS_DIR}"

# 1Password injection - uses OP_ACCOUNT from root Taskfile.yml env (my.1password.com)
export OP_ACCOUNT="${OP_ACCOUNT:-my.1password.com}"

echo "[info] Rendering ${NODE} (${MAC}) -> ${OUT}"
op inject -f -i "${TEMPLATE}" -o "${OUT}"

# Validate
if command -v talosctl >/dev/null; then
  talosctl validate --config "${OUT}" --mode metal
fi

echo "[ok] ${OUT}"
