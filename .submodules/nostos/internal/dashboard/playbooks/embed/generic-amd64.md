# Generic amd64 — BIOS / UEFI playbook

This playbook covers generic amd64 hosts (mini-PCs, NUCs, OptiPlexes that
aren't the 3080M variant) onboarded via Talos PXE.

## Required BIOS / UEFI settings

1. **Boot mode**: UEFI only (Legacy / CSM disabled).
2. **Secure Boot**: **OFF**. Talos kernels are not signed for vendor keys.
3. **TPM**: Either state works, but leave consistent across re-installs.
4. **Fast Boot**: OFF — full POST is required for reliable PXE.
5. **CPU virtualization**: VT-x / AMD-V **ON** (kube workloads expect it).

## Boot order

Set the first boot device to **IPv4 PXE / Network**, with the install disk
as second (so the box boots from disk after Talos lands the ISO):

```
1) IPv4 PXE / NIC (LAN1)
2) Internal NVMe / SATA install disk
3) USB / removable media
```

## NIC configuration

- Wake-on-LAN: ON if you intend to use `nostos node install --reinstall`.
- DHCP must be available on the PXE VLAN; nostos pxe-server handles the
  `next-server` / `filename` handoff.

## Install disk

- Recommended: NVMe (`/dev/nvme0n1`). Fast wipes, fast boots.
- SATA SSD (`/dev/sda`) works; spinning rust is unsupported.
- For dual-disk boxes, set `install_disk:` explicitly in `config.yaml` —
  do not rely on Talos auto-pick.

## Re-flash workflow

```
nostos node install <name> --reinstall --yes
```

The TUI's `r` keybind invokes the same path.
