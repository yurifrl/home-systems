# Dell OptiPlex 3080M — Talos PXE Setup

This playbook captures the BIOS knobs the Dell OptiPlex 3080M needs to PXE-boot
into Talos and the disk choices that have bitten me.

## BIOS settings

1. Reboot → tap **F2** to enter Setup.
2. **Boot Sequence** → set to **UEFI**.
3. **Secure Boot** → **Disabled** (Talos kernels are not signed for Dell's CA).
4. **Integrated NIC** → **Enabled w/ PXE Boot**.
5. **Boot Order** → put `IPv4 Ethernet` ahead of any local disk for the install,
   then re-order back to NVMe-first after the first successful install.

## Disks

- The 3080M has both an NVMe slot (M.2 2280) and a 2.5" SATA bay.
- Talos `install_disk` should target `/dev/nvme0n1` — confirm with
  `talosctl -n <ip> disks` after maintenance boot.

## Recovery

If PXE chainload hangs, hold the power button 10s, re-enter BIOS, and verify
the NIC's option ROM is still **enabled** — Dell's BIOS sometimes resets this
after a CMOS battery dip.
