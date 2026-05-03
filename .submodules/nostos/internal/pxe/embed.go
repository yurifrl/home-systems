// Package pxe builds Talos boot assets + serves them over HTTP/TFTP/DHCP.
package pxe

// EmbedIpxe is the boot script we compile into the iPXE binary.
//
// The retry loop tolerates consumer-router DHCP races: many home routers
// (notably TP-Link Deco) reply to PXE DHCPDISCOVER but don't set option 67
// (bootfile-name / ${filename}). We retry until we get an offer with it set,
// which means OUR dnsmasq won.
const EmbedIpxe = `#!ipxe
:retry_dhcp
dhcp || goto retry_dhcp
isset ${filename} || goto retry_dhcp
chain ${filename}
`

// BootIpxeTemplate is the second-stage script, rendered per-build.
//
// Uses iPXE runtime variables so the script is IP-portable — the Mac's
// ethernet IP can change without rebuilding anything.
//
// URLs use the /assets/ prefix because the HTTP server's document root is
// the state/ dir (not state/assets/), allowing /configs/<mac>.yaml to
// resolve as a sibling of /assets/.
//
// %s placeholders (Sprintf order):
//  1. Talos version (e.g. v1.10.3)
//  2. Architecture  (amd64 | arm64)
//  3. Extra kernel args (e.g. "talos.experimental.wipe=system" or empty)
//  4. Architecture again for initramfs filename
const BootIpxeTemplate = `#!ipxe
# Rendered by nostos. Per-MAC config fetched via ${mac:hexhyp}.
# ${next-server} resolves at runtime from the DHCP reply, so this script
# is portable across operator IP changes.
set cfg_url http://${next-server}:9080/configs/${mac:hexhyp}.yaml
echo Booting Talos %s, config at ${cfg_url}
kernel http://${next-server}:9080/assets/vmlinuz-%s talos.platform=metal talos.config=${cfg_url} %s slab_nomerge pti=on console=tty0
initrd http://${next-server}:9080/assets/initramfs-%s.xz
boot
`
