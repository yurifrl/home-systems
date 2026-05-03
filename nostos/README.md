# home-systems / nostos data

This directory holds the bare-metal Talos provisioning data for **this** home
lab. The tool that consumes it (`nostos`) lives in `../.submodules/nostos/`.

## Layout

```
nostos/
├── config.yaml        ← cluster + node registry + secrets backend (committed)
├── templates/         ← per-node Talos machineconfig templates with op:// refs
│   └── dell01.yaml
└── state/             ← everything else: downloaded kernel/initramfs, built iPXE,
                        rendered secret-bearing configs, talosconfig, logs.
                        Gitignored; rebuildable from config.yaml + templates + 1Password.
```

## Nodes

- **dell01** — Dell OptiPlex 3080M, amd64, `192.168.68.100`, controlplane.
  Replaced the original Raspberry Pi controlplane on 2026-05-03.

Workers (tp1 on 192.168.68.107, tp4 on 192.168.68.114, vm-pc01 on
192.168.68.102) are not yet in `config.yaml` — their Tailscale authkeys
expired and they need a reinstall before they can rejoin. Add them here
when you're ready.

## Common commands

```bash
# Taskfile wrappers (recommended)
task nostos:build                   # download Talos + build iPXE
task nostos:render NODE=dell01      # render per-MAC machineconfig
task nostos:up                      # start PXE server (sudo for dnsmasq)
task nostos:status                  # per-node reachability + Talos version
task nostos:bootstrap NODE=dell01   # talosctl bootstrap + fetch kubeconfig

# Direct invocation
go run ./.submodules/nostos/cmd/nostos --config nostos/config.yaml <cmd>
```

## Recovery

`nostos/state/` is a cache. Any of the following is safe:

```bash
task nostos:nuke --yes              # wipe state/ entirely
rm -rf nostos/state/                # same thing, more direct
```

Rebuild via:

```bash
task nostos:build
task nostos:render NODE=dell01
```

1Password is the only source of primary state (CA, cluster tokens,
Tailscale authkey). As long as the `kubernetes` vault is intact, everything
under `nostos/state/` can be recreated.

## See also

- `../.submodules/nostos/README.md` — the tool itself (install, full CLI reference)
- `../docs/nostos-demo.md` — 10-minute guided demo (if present)
- `../CLAUDE.md` — agent guidance for this repo
