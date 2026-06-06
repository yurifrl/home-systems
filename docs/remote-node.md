# Remote node: zero-touch enrollment

This guide covers shipping a Talos node to a remote site (different LAN
than the cluster) and having it auto-join via Tailscale.

## TL;DR

```bash
# 1. Add the node to nostos/config.yaml
# 2. Ship a flashable image
go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml \
    flash <node> --out /tmp/<node>.raw.xz --compress
# 3. Flash the image to an SD or SSD
xzcat /tmp/<node>.raw.xz | sudo dd of=/dev/rdiskN bs=4M status=progress
# 4. (Pi 4 only, first power-on) Flash the EEPROM with the bundled recovery
#    files: format a microSD as FAT32, copy the contents of <node>-eeprom/,
#    boot the Pi until the green LED settles, then swap to the Talos disk.
# 5. Boot the node, watch for it on the LAN
# 6. Apply the sidecar config once Talos is up:
talosctl apply-config --insecure --nodes <ip> --file /tmp/<node>-config.yaml
```

That's it. Tailscale connects, accept-routes is on, etcd peers reach the
existing controlplane via the tailnet, the new node joins as a learner,
gets promoted, kubelet registers — and `kubectl get nodes` shows it.

## Prerequisites

- Tailscale OAuth client configured under `secrets.tailscale` in
  `nostos/config.yaml` (so `nostos flash` can mint a fresh auth key per
  image).
- The cluster's controlplane(s) advertise their LAN subnet via Tailscale
  (`TS_ROUTES=…,192.168.68.0/24`) and approve routes in the Tailscale
  admin console.
- All node templates include `TS_EXTRA_ARGS=--accept-routes` so the new
  node can use peer subnet routes (now the default in `nostos/templates/`).

## Adding the node to config

Append a `nodes.<name>` entry to `nostos/config.yaml`:

```yaml
nodes:
  rpi01:
    mac: "e4:5f:01:3c:68:fa"
    ip: 192.168.0.170          # local LAN IP at the remote site
    role: controlplane         # or: worker
    arch: arm64
    install_disk: /dev/sda
    template: rpi01.yaml
    overlay: rpi_generic       # only set for Raspberry Pi 4/5
    schematic_id: <schematic>  # arm64 schematic with rpi_generic overlay
```

Then create `nostos/templates/<name>.yaml` with hostname, network, install
disk, and Tailscale extension config (mirroring `dell01.yaml`/`rpi01.yaml`).

## Why each step matters

- **`overlay: rpi_generic`** tells `nostos build` and `nostos flash` to also
  fetch `start4.elf` / `fixup4.dat` (RPi GPU firmware) and to emit the
  EEPROM recovery bundle alongside the image.
- **The sidecar config (`<image>-config.yaml`)** is the rendered
  machineconfig with secrets resolved and a fresh Tailscale auth key
  embedded. The node boots Talos in maintenance mode; one
  `talosctl apply-config --insecure` writes it to disk and the rest of
  the join is automatic.
- **`--accept-routes` on every node** is the gotcha that breaks etcd peer
  communication when nodes live on different LANs. Without it, the Pi
  can't reach `192.168.68.100:2380` even though Tailscale sees the
  controlplane online.

## Lifecycle

After the node joins:

```bash
# Re-render + push config changes (e.g. node labels)
nostos apply rpi01 --mode no-reboot

# Roll Talos OS upgrade
nostos upgrade

# Health check
nostos status --output json | jq
```

Workloads scheduled to the new node use the cluster's pod CIDR
(`10.244.0.0/16`) as usual. Cross-subnet pod traffic flows over the
tailnet.

## Common failure modes

- **"online=no, lastseen=…"** for a peer in the new node's `ext-tailscale`
  logs → an old/stale Tailscale device is shadowing the route. Remove the
  stale entry from the Tailscale admin and let routing reconverge.
- **etcd member stays a learner** → confirm the node's etcd peer URL is
  reachable from existing controlplanes. The classic cause is the
  controlplane lacks `--accept-routes` and can't see the new subnet.
- **Pi sits with a solid red LED** → EEPROM recovery hasn't run. Format
  a microSD as FAT32, copy the `<node>-eeprom/` contents, boot once.

## Related

- [`AGENTS.md`](../.submodules/nostos/AGENTS.md) — operator invariants
  and the AI-friendly schema.
- [`docs/tailscale-authkey-refresh.md`](./tailscale-authkey-refresh.md) —
  the per-render auth-key minting contract.
