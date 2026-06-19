#!/usr/bin/env bash
# build.sh — runs INSIDE a privileged debian:13 container. Builds the combined
# Win11 24H2 install ISO and writes it to /out/Win11_24H2_combined.iso.
#
# Combines, into one ISO (the bpg provider allows only ONE cdrom):
#   - Win11 24H2 retail amd64 base (built via UUP dump)
#   - virtio viostor + NetKVM drivers under \virtio  (so Setup sees virtio0)
#   - autounattend.xml at the ISO root (hands-free install)
#   - no-keypress UEFI boot (efisys_noprompt.bin) so no "press any key"
#
# Inputs (mounted): /ctx/autounattend.xml
# Output (mounted): /out/Win11_24H2_combined.iso
# See README.md for the `docker run` invocation.
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq aria2 cabextract wimtools chntpw genisoimage xorriso unzip curl ca-certificates >/dev/null
echo "[1/5] tools installed"

cd /work 2>/dev/null || { mkdir -p /work; cd /work; }
UUID=f7e8991e-4fd8-4bfd-a404-0de6dccd4191   # Win11 24H2 retail amd64 (uupdump)
mkdir -p uup && cd uup
curl -sL -o pkg.zip "https://uupdump.net/get.php?id=$UUID&pack=en-us&edition=professional&autodl=2"
unzip -o pkg.zip >/dev/null
chmod +x uup_download_linux.sh
echo "[2/5] UUP build (downloads ~4GB; git.uupdump.net 522s are transient -> retried)"
ok=0
for attempt in $(seq 1 8); do
  if ./uup_download_linux.sh >/work/uup.log 2>&1; then ok=1; break; fi
  echo "  attempt $attempt failed (likely transient); retrying..."; sleep 15
done
[ "$ok" = 1 ] || { echo "UUP build failed after retries"; tail -20 /work/uup.log; exit 1; }
BASE=$(ls /work/uup/*.ISO | head -1)

cd /work
echo "[3/5] virtio-win..."
curl -fL --retry 8 -o virtio-win.iso https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso

echo "[4/5] combine..."
mkdir -p iso mnt
mount -o loop,ro "$BASE" mnt; cp -aT mnt iso; umount mnt; chmod -R u+w iso
mount -o loop,ro virtio-win.iso mnt
mkdir -p iso/virtio/viostor/w11/amd64 iso/virtio/NetKVM/w11/amd64
cp -a mnt/viostor/w11/amd64/. iso/virtio/viostor/w11/amd64/
cp -a mnt/NetKVM/w11/amd64/. iso/virtio/NetKVM/w11/amd64/
umount mnt
cp /ctx/autounattend.xml iso/autounattend.xml
# Inject the Windows admin password at build time (never stored in the repo).
# Pass it in: docker run -e WIN_ADMIN_PASSWORD="$(op item get windows-pc01-admin --fields password --reveal)" ...
: "${WIN_ADMIN_PASSWORD:?set WIN_ADMIN_PASSWORD (source from 1Password item windows-pc01-admin)}"
sed -i "s|__WIN_ADMIN_PASSWORD__|${WIN_ADMIN_PASSWORD}|g" iso/autounattend.xml
EFISYS=efi/microsoft/boot/efisys.bin
[ -f iso/efi/microsoft/boot/efisys_noprompt.bin ] && EFISYS=efi/microsoft/boot/efisys_noprompt.bin
echo "[4/5] UEFI boot image: $EFISYS"

echo "[5/5] repack..."
xorriso -as mkisofs -iso-level 3 -full-iso9660-filenames -volid "WIN11_24H2" \
  -b boot/etfsboot.com -no-emul-boot -boot-load-size 8 -boot-info-table \
  -eltorito-alt-boot -e "$EFISYS" -no-emul-boot \
  -o /out/Win11_24H2_combined.iso iso
echo "DONE:"; ls -la /out/Win11_24H2_combined.iso
