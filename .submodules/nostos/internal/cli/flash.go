package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/dryrun"
	"github.com/yurifrl/nostos/internal/cli/errs"
	"github.com/yurifrl/nostos/internal/cli/inputx"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/image"
	"github.com/yurifrl/nostos/internal/paths"
	"github.com/yurifrl/nostos/internal/pxe"
	"github.com/yurifrl/nostos/internal/registry"
)

func newFlashCmd() *cobra.Command {
	var (
		outPath  string
		device   string
		compress bool
		dryRun   bool
		yes      bool
	)
	cmd := &cobra.Command{
		Use:   "flash NODE",
		Short: "Build a flashable Talos disk image for NODE (mints Tailscale key, renders config)",
		Long: "Produce a flashable disk image for the named node:\n" +
			"\n" +
			"  - Downloads (or reuses) the Talos raw image for the node's\n" +
			"    schematic + arch.\n" +
			"  - Renders the machineconfig (resolving secrets, minting a fresh\n" +
			"    Tailscale auth key embedded in the Tailscale extension).\n" +
			"  - Writes the image to --out FILE (optionally xz-compressed)\n" +
			"    or directly to --device /dev/diskN.\n" +
			"  - For RPi nodes, also emits an EEPROM recovery directory the\n" +
			"    operator can copy to a FAT32 SD card to enable network boot\n" +
			"    on a fresh Pi 4.\n" +
			"\n" +
			"After flashing, boot the node and apply the sidecar config with:\n" +
			"  talosctl apply-config --insecure --nodes <ip> --file <node>-config.yaml",
		Args: cobra.ExactArgs(1),
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := inputx.ValidateNodeName(name); err != nil {
				return err
			}
			if outPath == "" && device == "" {
				return errs.Validation("E_FLASH_OUTPUT_REQUIRED",
					"either --out FILE or --device /dev/diskN is required")
			}
			if outPath != "" && device != "" {
				return errs.Validation("E_FLASH_OUTPUT_CONFLICT",
					"--out and --device are mutually exclusive")
			}

			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			node, err := registry.Get(cfg, name)
			if err != nil {
				return errs.NotFound("E_NODE_NOT_FOUND", err.Error()).
					WithDetails(map[string]any{"name": name}).
					WithHint("nostos node list")
			}

			if dryRun {
				return emitFlashDryRun(cfg, node, name, outPath, device, compress)
			}

			// Confirmation gate when writing to a device.
			if device != "" && !yes {
				return errs.Conflict("E_CONFIRM_REQUIRED",
					fmt.Sprintf("writing to %s will overwrite the device; refusing without --yes", device)).
					WithDetails(map[string]any{"device": device, "node": name}).
					WithHint("re-run with --yes once you've confirmed the device path")
			}

			return runFlash(cmd.Context(), cfg, p, node, name, outPath, device, compress)
		}),
	}
	cmd.Flags().StringVar(&outPath, "out", "", "write image to FILE (.raw or .raw.xz). Sidecar <FILE>-config.yaml is also written.")
	cmd.Flags().StringVar(&device, "device", "", "write image directly to block device (e.g. /dev/disk10). Mutually exclusive with --out.")
	cmd.Flags().BoolVar(&compress, "compress", false, "xz-compress --output (no-op for --device)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview planned actions as JSON; no subprocesses, no key minting")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation when writing to a device")
	return cmd
}

func emitFlashDryRun(cfg *config.Config, node config.Node, name, outPath, device string, compress bool) error {
	plan := dryrun.New("flash")
	schematic := node.EffectiveSchematic(cfg.Cluster)
	plan.Add("preflight", fmt.Sprintf("validate node %s, arch=%s, overlay=%q", name, node.Arch, node.Overlay))
	plan.Add("download.image",
		fmt.Sprintf("download metal-%s.raw.xz for schematic %s@%s",
			node.Arch, shorten(schematic), cfg.Cluster.TalosVersion))
	if node.Overlay == "rpi_generic" {
		plan.Add("download.rpi-firmware", "download start4.elf + fixup4.dat from raspberrypi/firmware")
	}
	plan.Add("render", fmt.Sprintf("render machineconfig for %s (mints Tailscale key)", name))
	if outPath != "" {
		extra := ""
		if compress {
			extra = " (xz-compressed)"
		}
		plan.Add("assemble.file", fmt.Sprintf("write image to %s%s + sidecar config", outPath, extra))
	}
	if device != "" {
		plan.AddArgv("assemble.device", "write image to "+device,
			[]string{"dd", "if=<raw-image>", "of=" + device, "bs=4M", "status=progress"}, nil)
	}
	if node.Overlay == "rpi_generic" {
		plan.Add("eeprom", "emit EEPROM recovery directory (boot.conf with BOOT_ORDER=0xf21)")
	}
	plan.Add("instructions", "print next-step talosctl apply-config command")
	return emitDryRun(plan)
}

func runFlash(ctx context.Context, cfg *config.Config, p paths.Paths, node config.Node, name, outPath, device string, compress bool) error {
	if err := p.EnsureState(); err != nil {
		return err
	}

	// 1. Download Talos raw image for this (schematic, arch) pair.
	spec := pxe.AssetSpec{
		Schematic: node.EffectiveSchematic(cfg.Cluster),
		Arch:      node.Arch,
		Version:   cfg.Cluster.TalosVersion,
		IsRPi:     node.Overlay == "rpi_generic",
	}
	if outputMode != "json" {
		fmt.Fprintf(outWriter, "→ downloading Talos raw image (%s/%s)…\n", shorten(spec.Schematic), spec.Arch)
	}
	rawPath, err := pxe.DownloadTalosRawImage(ctx, p, spec)
	if err != nil {
		return errs.Network("E_TALOS_IMAGE_DOWNLOAD", err.Error())
	}

	// 2. Optionally fetch RPi firmware (start4.elf, fixup4.dat).
	rpiDir := ""
	if spec.IsRPi {
		if err := pxe.DownloadRPiFirmware(ctx, p); err != nil {
			return errs.Network("E_RPI_FIRMWARE_DOWNLOAD", err.Error())
		}
		rpiDir = filepath.Join(p.Assets(), "rpi-firmware")
	}

	// 3. Render machineconfig (mints Tailscale key as a side effect).
	if outputMode != "json" {
		fmt.Fprintf(outWriter, "→ rendering machineconfig for %s (mints Tailscale key)…\n", name)
	}
	cfgPath, err := registry.Render(cfg, p, name, true)
	if err != nil {
		return errs.FromGo(err)
	}
	configBytes, err := os.ReadFile(cfgPath)
	if err != nil {
		return errs.FromGo(err)
	}

	// 4. Assemble image.
	b := &image.Builder{
		NodeName:       name,
		NodeMAC:        node.MAC,
		NodeArch:       node.Arch,
		NodeOverlay:    node.Overlay,
		RawImagePath:   rawPath,
		MachineConfig:  configBytes,
		RPiFirmwareDir: rpiDir,
		Compress:       compress,
	}
	if device != "" {
		b.Out = image.ModeDevice
		b.OutPath = device
	} else {
		b.Out = image.ModeFile
		b.OutPath = outPath
	}
	if outputMode != "json" {
		fmt.Fprintf(outWriter, "→ assembling image at %s…\n", b.OutPath)
	}
	res, err := b.Assemble(ctx)
	if err != nil {
		return errs.FromGo(err)
	}

	// 5. Emit result.
	if outputMode == "json" {
		return outputJSON(map[string]any{
			"status": "flashped",
			"node":   name,
			"image":  res.ImagePath,
			"config": res.ConfigPath,
			"eeprom": res.EEPROMPath,
		})
	}

	fmt.Fprintf(outWriter, "✓ image:  %s (%s -> %s)\n",
		res.ImagePath, humanBytes(res.BytesIn), humanBytes(res.BytesOut))
	if res.ConfigPath != "" {
		fmt.Fprintf(outWriter, "✓ config: %s\n", res.ConfigPath)
	}
	if res.EEPROMPath != "" {
		fmt.Fprintf(outWriter, "✓ eeprom: %s/ (copy to FAT32 SD for first-boot EEPROM flash)\n", res.EEPROMPath)
	}
	fmt.Fprintln(outWriter)
	fmt.Fprintln(outWriter, "next steps:")
	if device != "" {
		fmt.Fprintf(outWriter, "  1. eject the device: %s\n", ejectHint(device))
		fmt.Fprintf(outWriter, "  2. plug it into the target node, power on\n")
	} else {
		fmt.Fprintf(outWriter, "  1. flash %s to the target disk:\n", filepath.Base(res.ImagePath))
		if compress {
			fmt.Fprintf(outWriter, "       xzcat %s | sudo dd of=/dev/rdiskN bs=4M status=progress\n", res.ImagePath)
		} else {
			fmt.Fprintf(outWriter, "       sudo dd if=%s of=/dev/rdiskN bs=4M status=progress\n", res.ImagePath)
		}
		fmt.Fprintf(outWriter, "  2. boot the node\n")
	}
	if res.ConfigPath != "" {
		fmt.Fprintf(outWriter, "  3. apply config once Talos is up (maintenance mode):\n")
		fmt.Fprintf(outWriter, "       talosctl apply-config --insecure --nodes %s --file %s\n",
			node.IP, res.ConfigPath)
	} else {
		fmt.Fprintf(outWriter, "  3. apply config from the rendered file under %s\n", p.Configs())
	}
	if res.EEPROMPath != "" {
		fmt.Fprintln(outWriter)
		fmt.Fprintln(outWriter, "  (Pi 4 first-time setup: format a separate microSD as FAT32, copy the eeprom dir contents,")
		fmt.Fprintln(outWriter, "   boot the Pi until the green LED settles, then swap to the Talos disk.)")
	}
	return nil
}

// shorten trims a 64-char schematic id for display.
func shorten(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:12] + "…"
}

// humanBytes formats a byte count in MiB/GiB.
func humanBytes(n int64) string {
	const (
		KiB = 1024
		MiB = 1024 * KiB
		GiB = 1024 * MiB
	)
	switch {
	case n >= GiB:
		return fmt.Sprintf("%.1f GiB", float64(n)/float64(GiB))
	case n >= MiB:
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(MiB))
	case n >= KiB:
		return fmt.Sprintf("%.1f KiB", float64(n)/float64(KiB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ejectHint returns a platform-appropriate eject command for the device path.
func ejectHint(device string) string {
	switch runtime.GOOS {
	case "darwin":
		return "diskutil eject " + device
	case "linux":
		return "udisksctl power-off -b " + device
	default:
		return "sync && eject " + device
	}
}

// Compile-time check that flash is wired through the standard error helpers.
var _ = errors.New
var _ = strings.TrimSpace
