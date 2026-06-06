package image

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// EEPROMConfigDefault is the bootconf written into <eeprom>/boot.conf. It
// matches the rpi-imager "Network Boot" recovery preset: BOOT_ORDER=0xf21
// (network -> SD -> restart) plus quality-of-life flags.
const EEPROMConfigDefault = `[all]
BOOT_UART=1
WAKE_ON_GPIO=1
POWER_OFF_ON_HALT=0
BOOT_ORDER=0xf21
`

// EEPROMFiles is the canonical list of files emitted into the EEPROM
// recovery directory. Operators copy these to a FAT32-formatted SD card,
// boot the Pi once to flash the EEPROM, then plug the SD card with the
// real Talos image (or remove it for net-boot).
var EEPROMFiles = []string{
	"start4.elf",
	"fixup4.dat",
	"recovery.bin",
	"pieeprom.bin",
	"boot.conf",
}

// writeEEPROMImage assembles a directory at outPath with the EEPROM recovery
// files copied from firmwareDir. The caller flashes the contents to a FAT32
// SD card to set BOOT_ORDER on a fresh Pi 4.
//
// We deliberately avoid producing a single raw FAT32 image because that
// requires platform-specific tools (mkfs.fat / hdiutil) which are not
// universally available. A directory is trivially flashable cross-platform:
// `cp -R <dir>/* /Volumes/RECOVERY/`.
func writeEEPROMImage(outPath, firmwareDir string) error {
	if err := os.MkdirAll(outPath, 0o755); err != nil {
		return fmt.Errorf("mkdir eeprom dir: %w", err)
	}
	// boot.conf is generated, the rest is copied from the firmware cache.
	if err := os.WriteFile(filepath.Join(outPath, "boot.conf"),
		[]byte(EEPROMConfigDefault), 0o644); err != nil {
		return err
	}
	for _, name := range EEPROMFiles {
		if name == "boot.conf" {
			continue // generated above
		}
		src := filepath.Join(firmwareDir, name)
		dst := filepath.Join(outPath, name)
		if err := copyFileImage(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", name, err)
		}
	}
	return nil
}

func copyFileImage(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
