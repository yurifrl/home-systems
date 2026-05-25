# Turing RK1 — Talos on the Turing Pi 2 BMC

The RK1 is an ARM64 SoM with eMMC and an optional NVMe SSD; install through
the Turing Pi 2 BMC (`tpi`) CLI.

## BMC credentials

- Default user is `root`, password is set during BMC firmware first-boot.
- Rotate via the BMC web UI → **Settings → Users**.
- Persist the rotated password in your secrets backend; reference it from
  `nostos/config.yaml` via `boot.tpi.password_ref: op://...`.

## Slot mapping

The BMC numbers slots **1..4** (left-to-right looking at the front panel).
A node's `boot.tpi.slot` MUST match the physical slot, not its position in
your config.

## Boot order

- eMMC vs NVMe: the RK1 will prefer NVMe if present and bootable. To wipe
  and re-flash, use `tpi power off → tpi flash --image <talos> --node <slot>`.
- After install, set `install_disk: /dev/nvme0n1` (NVMe present) or
  `/dev/mmcblk0` (eMMC only).

## SPI flash recovery

If a node bricks during flash, hold the **MASKROM** button on the SoM while
power-cycling the BMC; the SoM will enumerate as a USB device on the BMC's
host port for `rkdeveloptool` recovery.
