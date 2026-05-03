#!/usr/bin/env bash
# One-time setup: download Talos assets, build iPXE chainloader.
# Safe to re-run (idempotent).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

PXE_DIR="${PXE_DIR:-${REPO_ROOT}/pxe}"
ASSETS="${PXE_DIR}/assets"
TALOS_VERSION="$(awk '/^talos_version:/ {print $2}' "${PXE_DIR}/nodes.yaml")"
SCHEMATIC_AMD64="$(awk '/^schematic_amd64:/ {print $2}' "${PXE_DIR}/nodes.yaml")"

mkdir -p "${ASSETS}"
cd "${ASSETS}"

# --- Mac's ethernet IP for the embedded iPXE script ---
read -r IFACE MAC_IP < <(bash "${SCRIPT_DIR}/detect-mac-ip.sh")
echo "[info] Mac ethernet: ${IFACE} ${MAC_IP}"

# --- Talos kernel + initramfs (amd64) ---
if [ ! -s vmlinuz-amd64 ]; then
  echo "[info] Downloading Talos ${TALOS_VERSION} kernel (amd64)"
  curl -fL -o vmlinuz-amd64 \
    "https://factory.talos.dev/image/${SCHEMATIC_AMD64}/${TALOS_VERSION}/kernel-amd64"
fi
if [ ! -s initramfs-amd64.xz ]; then
  echo "[info] Downloading Talos ${TALOS_VERSION} initramfs (amd64)"
  curl -fL -o initramfs-amd64.xz \
    "https://factory.talos.dev/image/${SCHEMATIC_AMD64}/${TALOS_VERSION}/initramfs-amd64.xz"
fi

# --- Build custom iPXE EFI binary with embedded boot script ---
# The Dell's UEFI PXE has a tiny TFTP buffer (<256KB). Vanilla ipxe.efi from
# boot.ipxe.org is too large (>300KB) AND the URL returns HTML (broken).
# We build a minimal snponly.efi (~267KB) via Docker cross-compile.
if [ ! -s ipxe.efi ]; then
  echo "[info] Building iPXE EFI binary"
  IPXE_SRC="${PXE_DIR}/ipxe-src"
  if [ ! -d "${IPXE_SRC}" ]; then
    git clone --depth 1 https://github.com/ipxe/ipxe.git "${IPXE_SRC}"
  fi
  cat > "${IPXE_SRC}/src/embed.ipxe" <<'EOF'
#!ipxe
# Keep DHCP'ing until we get a response from OUR dnsmasq. The home router (Deco)
# also replies to DHCP but doesn't set option 67 (bootfile-name / ${filename}),
# so if its offer wins the race we loop and retry.
:retry_dhcp
dhcp || goto retry_dhcp
isset ${filename} || goto retry_dhcp
chain ${filename}
EOF
  docker run --rm --platform linux/amd64 \
    -v "${IPXE_SRC}":/ipxe -w /ipxe/src \
    alpine:latest sh -c "
      apk add --no-cache --quiet build-base perl xz-dev mtools gnu-efi-dev >/dev/null &&
      make -j4 bin-x86_64-efi/snponly.efi EMBED=embed.ipxe NO_WERROR=1
    "
  cp "${IPXE_SRC}/src/bin-x86_64-efi/snponly.efi" "${ASSETS}/ipxe.efi"
fi

# --- Render the top-level iPXE boot script ---
# This is what iPXE loads after DHCP. It pulls kernel/initramfs over HTTP and
# tells Talos to fetch its machineconfig from the HTTP server too.
cat > "${ASSETS}/boot.ipxe" <<EOF
#!ipxe
# Identify node by MAC, fetch matching config
# \${next-server} is resolved by iPXE at runtime from the DHCP next-server field,
# so we don't have to rebuild when the Mac's IP changes.
#
# NOTE: If you need to wipe an existing Talos install from a node, uncomment the
# 'talos.experimental.wipe=system' arg below for ONE PXE boot, then re-run
# 'task pxe:setup' to remove it. Leaving it in causes an infinite wipe loop
# when the BIOS prefers PXE over disk.
set cfg_url http://\${next-server}:9080/configs/\${mac:hexhyp}.yaml
echo Booting Talos ${TALOS_VERSION}, config at \${cfg_url}
kernel http://\${next-server}:9080/vmlinuz-amd64 talos.platform=metal talos.config=\${cfg_url} slab_nomerge pti=on console=tty0
initrd http://\${next-server}:9080/initramfs-amd64.xz
boot
EOF

echo ""
echo "[ok] Assets ready in ${ASSETS}"
ls -lh "${ASSETS}"
