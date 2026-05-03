#!/usr/bin/env bash
# Print the Mac's ethernet IP on the LAN (192.168.68.0/24).
# Used by the PXE setup scripts to pin server addresses.
set -euo pipefail

LAN_PREFIX="${LAN_PREFIX:-192.168.68}"

for iface in en0 en1 en2 en3 en4 en5 en6 en7; do
  ip=$(ifconfig "$iface" 2>/dev/null | awk -v pfx="$LAN_PREFIX" '/inet / && $2 ~ pfx {print $2; exit}')
  if [ -n "$ip" ]; then
    # Skip Wi-Fi - we want ethernet for PXE speed race
    port=$(networksetup -listallhardwareports 2>/dev/null | awk -v dev="$iface" '
      /Hardware Port/ { name=$0 }
      $0 ~ "Device: "dev { print name; exit }
    ')
    case "$port" in
      *Wi-Fi*|*AirPort*) continue ;;
    esac
    echo "$iface $ip"
    exit 0
  fi
done

echo "ERROR: no ethernet interface found on ${LAN_PREFIX}.0/24" >&2
echo "Plug your Mac into the switch via ethernet and retry." >&2
exit 1
