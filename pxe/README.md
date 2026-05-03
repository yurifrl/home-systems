# Assembly-Line PXE Boot for Talos Linux

Dead-simple Talos installer for any bare-metal machine on your LAN.

## How it works

1. You power on a machine with PXE-capable NIC (first in boot order)
2. Machine hits your Mac's PXE server (dnsmasq)
3. Mac serves iPXE + Talos kernel + machine-specific config
4. Talos installs to disk, applies config, reboots into cluster

## One-time setup

```bash
task pxe:setup
```

Downloads Talos kernel/initramfs, builds a custom iPXE EFI binary, and prepares the asset directory.

## Adding a new node

1. Create a template in `talos/templates/<hostname>.yaml`
   - Copy an existing one (`dell01.yaml`) as a starting point
   - Set hostname, MAC address, IP, install disk
2. Render the node config with secrets:
   ```bash
   task pxe:config NODE=dell01
   ```
3. Add the node's MAC to `pxe/nodes.yaml` (maps MAC → IP)
4. Start the PXE server:
   ```bash
   task pxe:up
   ```
5. Power on the target machine, F12 → network boot
6. When Talos is installed and running, `task pxe:down`

## Commands

| Command | Purpose |
|---------|---------|
| `task pxe:setup` | One-time: download assets, build iPXE |
| `task pxe:config NODE=name` | Render a node's config (1Password injects secrets) |
| `task pxe:up` | Start HTTP + dnsmasq (requires sudo, foreground) |
| `task pxe:down` | Kill background servers if any |
| `task pxe:status` | Show what's running |

## Requirements

- macOS with Homebrew
- `brew install dnsmasq` (used directly, not as a service)
- `op` (1Password CLI) for secret injection
- Docker (OrbStack) for building iPXE
- Your Mac connected via **ethernet to the same switch** as the target machine (WiFi won't win the DHCP race with your router)

## Troubleshooting

See `docs/pxe-boot.md` for deep troubleshooting history (Tailscale network extension, BIOS settings, TFTP buffer limits, etc.).
