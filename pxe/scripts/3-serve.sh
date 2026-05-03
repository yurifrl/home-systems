#!/usr/bin/env bash
# Start PXE server: HTTP for assets + dnsmasq for DHCP/TFTP.
# Runs in foreground, logs to stdout. Ctrl+C to stop.
#
# Requires sudo (DHCP needs port 67). Re-run on dnsmasq config change.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PXE_DIR="${PXE_DIR:-${REPO_ROOT}/pxe}"
ASSETS="${PXE_DIR}/assets"
HTTP_PORT="${HTTP_PORT:-9080}"

if [ ! -s "${ASSETS}/ipxe.efi" ] || [ ! -s "${ASSETS}/boot.ipxe" ]; then
  echo "ERROR: missing assets. Run 'task pxe:setup' first." >&2
  exit 1
fi

read -r IFACE MAC_IP < <(bash "${SCRIPT_DIR}/detect-mac-ip.sh")
echo "[info] PXE server: ${IFACE} ${MAC_IP}"

# No per-MAC pinning: any PXE client in the range gets an IP.
# Filter to PXE clients only so we don't fight the Deco router for non-PXE devices.
# --- HTTP server (background, killed on exit) ---
trap 'jobs -p | xargs -r kill 2>/dev/null || true' EXIT INT TERM

# Kill any stale HTTP server on our port from a prior run
stale_http_pid=$(lsof -ti :"${HTTP_PORT}" 2>/dev/null || true)
if [ -n "${stale_http_pid}" ]; then
  echo "[info] killing stale process on :${HTTP_PORT} (pid ${stale_http_pid})"
  kill "${stale_http_pid}" 2>/dev/null || true
  sleep 1
fi

(cd "${ASSETS}" && python3 -m http.server "${HTTP_PORT}") &
HTTP_PID=$!
echo "[info] HTTP server PID ${HTTP_PID} on :${HTTP_PORT}"

# Stage TFTP assets under /tmp so dnsmasq (drops to 'nobody' on macOS) can read them.
# /Users/yuri is 0750 and blocks 'nobody' traversal, so we can't serve directly from the repo.
TFTP_ROOT="/tmp/pxe-tftp"
mkdir -p "${TFTP_ROOT}"
cp -f "${ASSETS}/ipxe.efi" "${TFTP_ROOT}/ipxe.efi"
chmod 755 "${TFTP_ROOT}"
chmod 644 "${TFTP_ROOT}/ipxe.efi"
echo "[info] TFTP root staged at ${TFTP_ROOT}"

# --- dnsmasq foreground ---
DNSMASQ=/opt/homebrew/sbin/dnsmasq
if [ ! -x "${DNSMASQ}" ]; then
  echo "ERROR: ${DNSMASQ} not found. brew install dnsmasq" >&2
  exit 1
fi

# Allocation range covers the static IPs we pinned.
# Authoritative so we win the race against the Deco router.
# --port=0 disables DNS (we only want DHCP + TFTP).
# Fast-fail if sudo will prompt (non-interactive check)
if ! sudo -n "${DNSMASQ}" --version >/dev/null 2>&1; then
  echo "ERROR: sudo requires a password." >&2
  echo "  Either run 'sudo -v' first, or enable passwordless sudo for dnsmasq:" >&2
  echo "    echo \"\$USER ALL=(ALL) NOPASSWD: ${DNSMASQ} *\" | sudo tee /etc/sudoers.d/pxe-dnsmasq" >&2
  echo "    sudo chmod 440 /etc/sudoers.d/pxe-dnsmasq" >&2
  exit 1
fi

echo "[info] Starting dnsmasq (needs sudo)"
exec sudo "${DNSMASQ}" \
  --no-daemon \
  --port=0 \
  --interface="${IFACE}" \
  --dhcp-range=192.168.68.200,192.168.68.210,255.255.255.0,5m \
  --dhcp-option=3,192.168.68.1 \
  --dhcp-option=6,192.168.68.1 \
  --dhcp-authoritative \
  --dhcp-match=set:pxe,60,PXEClient \
  --dhcp-ignore=tag:!pxe \
  --enable-tftp \
  --tftp-root="${TFTP_ROOT}" \
  --dhcp-userclass=set:ipxe,iPXE \
  --dhcp-boot=tag:!ipxe,ipxe.efi,,"${MAC_IP}" \
  --dhcp-boot=tag:ipxe,http://"${MAC_IP}":"${HTTP_PORT}"/boot.ipxe \
  --log-queries \
  --log-dhcp
