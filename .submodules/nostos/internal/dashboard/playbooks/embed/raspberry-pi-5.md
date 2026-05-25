# Raspberry Pi 5 — Talos playbook

The Pi 5 needs a small ritual before Talos can boot it. Do this **once**
per board, then it joins the cluster like any other node.

## Bootloader EEPROM update

Use the latest stable EEPROM firmware. From a Raspberry Pi OS image:

```
sudo apt update && sudo apt full-upgrade
sudo rpi-eeprom-update -a
sudo reboot
```

Verify with `vcgencmd bootloader_version` after reboot.

## Boot order — enable USB / NVMe boot

```
sudo raspi-config
# → Advanced Options → Bootloader Version → Latest
# → Advanced Options → Boot Order → USB Boot
```

Equivalent CLI:

```
sudo rpi-eeprom-config --edit
# set:
BOOT_ORDER=0xf416   # try NVMe, then USB, then SD
```

## `config.txt` notes

The Pi 5 boots Talos via the standard arm64 image; nostos's schematic
already includes the `siderolabs/sbc-raspberrypi` overlay. Do **not**
hand-edit `config.txt` — Talos manages it. If you must add HAT-specific
DT overlays, do it through the Talos machine config patch, not on the
SD card directly.

## Storage choice

| Medium | When to use                                         | install_disk        |
| ------ | --------------------------------------------------- | ------------------- |
| SD     | Dev / disposable nodes only; high wear              | `/dev/mmcblk0`      |
| USB 3  | Decent option for low-write workloads               | `/dev/sda`          |
| NVMe   | Recommended for clusters; needs HAT or CM5 carrier  | `/dev/nvme0n1`      |

The Pi 5 root filesystem must be **ext4** (Talos default). Do not pre-format.

## Network

- Use the onboard 1 GbE; the USB-Ethernet path is unreliable for PXE.
- DHCP must serve the Pi's MAC; nostos pxe-server handles the rest.

## Re-flash

```
nostos node install <name> --reinstall --yes
```

If the bootloader doesn't pick PXE, hold the BOOTSEL flow per the Pi
imager docs and re-image the SD with the latest EEPROM, then retry.
