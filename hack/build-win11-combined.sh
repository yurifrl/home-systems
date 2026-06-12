#!/usr/bin/env bash
# Build a single combined Win11 24H2 install ISO for Proxmox VM 101 (windows),
# so the one-cdrom limit of provider-proxmox-bpg is respected while still:
#   - booting the Windows 11 installer,
#   - carrying the virtio (viostor/NetKVM) drivers under \virtio, and
#   - auto-running proxmox/autounattend.xml for a hands-free install.
#
# Run on the Proxmox host (Debian). Produces:
#   /var/lib/vz/template/iso/Win11_24H2_combined.iso
#
# Prereqs (apt): aria2 cabextract wimtools chntpw genisoimage unzip xorriso
# Inputs already present on the host:
#   - a base Win11 24H2 ISO (built via UUP dump; see UUID below)
#   - virtio-win.iso  (downloaded as a Crossplane EnvironmentDownloadFile)
#   - autounattend.xml (this repo: proxmox/autounattend.xml, scp'd to /root)
set -euo pipefail

ISO_DIR=/var/lib/vz/template/iso
WORK=/root/win11-combine
BASE_ISO="${1:?usage: build-win11-combined.sh <base-win11.iso>}"
VIRTIO_ISO="$ISO_DIR/virtio-win.iso"
AUTOUNATTEND=/root/autounattend.xml
OUT="$ISO_DIR/Win11_24H2_combined.iso"

rm -rf "$WORK"; mkdir -p "$WORK/iso" "$WORK/mnt" "$WORK/virtio"

# 1) extract base Win11 ISO
mount -o loop,ro "$BASE_ISO" "$WORK/mnt"
cp -aT "$WORK/mnt" "$WORK/iso"
umount "$WORK/mnt"
chmod -R u+w "$WORK/iso"

# 2) stage virtio drivers under \virtio
mount -o loop,ro "$VIRTIO_ISO" "$WORK/mnt"
mkdir -p "$WORK/iso/virtio/viostor/w11/amd64" "$WORK/iso/virtio/NetKVM/w11/amd64"
cp -a "$WORK/mnt/viostor/w11/amd64/." "$WORK/iso/virtio/viostor/w11/amd64/"
cp -a "$WORK/mnt/NetKVM/w11/amd64/." "$WORK/iso/virtio/NetKVM/w11/amd64/"
umount "$WORK/mnt"

# 3) autounattend.xml at ISO root
cp "$AUTOUNATTEND" "$WORK/iso/autounattend.xml"

# 4) repack as a BIOS+UEFI bootable ISO (Windows boot files)
xorriso -as mkisofs \
  -iso-level 3 -full-iso9660-filenames -volid "WIN11_24H2" \
  -b boot/etfsboot.com -no-emul-boot -boot-load-size 8 -boot-info-table \
  -eltorito-alt-boot -e efi/microsoft/boot/efisys.bin -no-emul-boot \
  -o "$OUT" "$WORK/iso"

echo "built: $OUT"
ls -la "$OUT"
