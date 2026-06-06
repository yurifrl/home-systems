// Package image assembles a flashable Talos disk image for a configured node.
//
// In v0.1 the assembly is intentionally simple: the Talos raw disk image is
// streamed (decompressed when needed) to the output target, and the rendered
// machineconfig is written alongside as a sidecar file. RPi nodes also get
// an EEPROM recovery FAT32 image so a fresh Pi 4 can be brought online with
// a single SD card flash.
//
// Future iterations will inject the machineconfig into the Talos META
// partition (key 0x0a, UserData) so first-boot is truly zero-touch. The
// current sidecar approach is a faithful, cross-platform mirror of what
// `nostos node install` does today: PXE boot Talos, then `talosctl
// apply-config --insecure`.
package image

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ulikunitz/xz"
)

// OutputMode determines where the assembled image goes.
type OutputMode int

const (
	// ModeFile writes the (optionally compressed) raw image to a file path.
	ModeFile OutputMode = iota
	// ModeDevice writes the raw image directly to a block device. The caller
	// is responsible for confirming the device is writable / unmounted.
	ModeDevice
)

// Builder is the per-ship invocation. It is intentionally a value type so
// callers can populate it field-by-field and then call Assemble.
type Builder struct {
	// NodeName, NodeMAC, NodeArch, NodeOverlay describe the target node.
	NodeName    string
	NodeMAC     string
	NodeArch    string // "amd64" | "arm64"
	NodeOverlay string // "" | "rpi_generic" | "turing_rk1"

	// RawImagePath is the source Talos raw image. May be a .raw or .raw.xz
	// file. The decision to decompress is made on suffix.
	RawImagePath string

	// MachineConfig is the rendered + secret-injected Talos machineconfig YAML.
	// Empty means "no sidecar config file is written" — used by --dry-run and
	// other preview paths that don't actually mint Tailscale keys.
	MachineConfig []byte

	// RPiFirmwareDir is the directory containing start4.elf / fixup4.dat /
	// recovery.bin / pieeprom.bin used to assemble the EEPROM recovery FAT32
	// partition for rpi_generic nodes. Empty disables EEPROM emission.
	RPiFirmwareDir string

	// Out describes where the assembled image goes.
	Out OutputMode
	// OutPath is interpreted as a file path (ModeFile) or device path
	// (ModeDevice).
	OutPath string
	// Compress toggles xz compression for ModeFile (no-op for ModeDevice).
	Compress bool
}

// Result describes what Assemble produced. Paths are absolute when possible.
type Result struct {
	ImagePath  string `json:"image_path"`
	ConfigPath string `json:"config_path,omitempty"`
	EEPROMPath string `json:"eeprom_path,omitempty"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
}

// Assemble decompresses + writes the Talos image to the chosen output, then
// writes the sidecar config + (for RPi nodes) an EEPROM recovery image
// alongside.
func (b *Builder) Assemble(ctx context.Context) (*Result, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	res := &Result{}

	imageOut, cleanup, err := b.openImageOutput()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	src, err := os.Open(b.RawImagePath)
	if err != nil {
		return nil, fmt.Errorf("open raw image: %w", err)
	}
	defer src.Close()
	srcInfo, _ := src.Stat()
	if srcInfo != nil {
		res.BytesIn = srcInfo.Size()
	}

	var reader io.Reader = src
	if filepath.Ext(b.RawImagePath) == ".xz" {
		xr, err := xz.NewReader(src)
		if err != nil {
			return nil, fmt.Errorf("xz reader: %w", err)
		}
		reader = xr
	}

	n, err := io.Copy(imageOut, reader)
	if err != nil {
		return nil, fmt.Errorf("write image: %w", err)
	}
	res.BytesOut = n
	res.ImagePath = b.OutPath

	// Sidecar machineconfig.
	if len(b.MachineConfig) > 0 && b.Out == ModeFile {
		cfgPath := sidecarPath(b.OutPath, "-config.yaml")
		if err := os.WriteFile(cfgPath, b.MachineConfig, 0o600); err != nil {
			return nil, fmt.Errorf("write sidecar config: %w", err)
		}
		res.ConfigPath = cfgPath
	}

	// EEPROM partition (RPi only).
	if b.NodeOverlay == "rpi_generic" && b.RPiFirmwareDir != "" && b.Out == ModeFile {
		ePath := sidecarPath(b.OutPath, "-eeprom.img")
		if err := writeEEPROMImage(ePath, b.RPiFirmwareDir); err != nil {
			return nil, fmt.Errorf("write eeprom image: %w", err)
		}
		res.EEPROMPath = ePath
	}

	return res, nil
}

// validate sanity-checks the builder before any work.
func (b *Builder) validate() error {
	if b.NodeName == "" {
		return errors.New("image.Builder: NodeName is empty")
	}
	if b.NodeArch != "amd64" && b.NodeArch != "arm64" {
		return fmt.Errorf("image.Builder: unsupported arch %q", b.NodeArch)
	}
	if b.RawImagePath == "" {
		return errors.New("image.Builder: RawImagePath is empty")
	}
	if _, err := os.Stat(b.RawImagePath); err != nil {
		return fmt.Errorf("image.Builder: raw image not found: %w", err)
	}
	if b.OutPath == "" {
		return errors.New("image.Builder: OutPath is empty")
	}
	if b.Out == ModeDevice && b.Compress {
		return errors.New("image.Builder: --compress is incompatible with --device")
	}
	return nil
}

// openImageOutput returns the writer for the primary image stream + a cleanup
// fn (close + flush). For ModeFile + Compress, an xz encoder is layered on top
// of the file.
func (b *Builder) openImageOutput() (io.Writer, func(), error) {
	switch b.Out {
	case ModeFile:
		path := b.OutPath
		if b.Compress && filepath.Ext(path) != ".xz" {
			path += ".xz"
			b.OutPath = path
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, fmt.Errorf("create image output: %w", err)
		}
		if b.Compress {
			enc, err := xz.NewWriter(f)
			if err != nil {
				f.Close()
				return nil, nil, fmt.Errorf("xz writer: %w", err)
			}
			return enc, func() { enc.Close(); f.Close() }, nil
		}
		return f, func() { f.Close() }, nil
	case ModeDevice:
		// O_SYNC ensures data hits the device. Permissions left to the caller
		// (running as root or in admin group on macOS).
		f, err := os.OpenFile(b.OutPath, os.O_WRONLY|os.O_SYNC, 0o600)
		if err != nil {
			return nil, nil, fmt.Errorf("open device %s: %w", b.OutPath, err)
		}
		return f, func() { f.Close() }, nil
	default:
		return nil, nil, fmt.Errorf("image.Builder: unknown output mode %d", b.Out)
	}
}

// sidecarPath drops trailing .xz / .raw / .img and appends the suffix.
func sidecarPath(out, suffix string) string {
	base := out
	for {
		ext := filepath.Ext(base)
		if ext == ".xz" || ext == ".raw" || ext == ".img" {
			base = base[:len(base)-len(ext)]
			continue
		}
		break
	}
	return base + suffix
}
