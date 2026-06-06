package pxe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
)

const (
	IpxeRepo         = "https://github.com/ipxe/ipxe.git"
	IpxeMaxSizeBytes = 300 * 1024 // Dell OptiPlex UEFI TFTP can accept ~327 KiB; 300 KiB = safety margin

	// RPi firmware files needed for net boot fallback / EEPROM recovery.
	rpiFirmwareBase = "https://raw.githubusercontent.com/raspberrypi/firmware/master/boot"
)

// AssetSpec is one (schematic, arch) pair to fetch.
type AssetSpec struct {
	Schematic string // 64-char hex schematic id
	Arch      string // amd64 | arm64
	Version   string // talos version e.g. v1.13.3
	// IsRPi is true when the corresponding node uses the rpi_generic overlay,
	// which means we also fetch start4.elf / fixup4.dat for net-boot.
	IsRPi bool
}

// CollectAssetSpecs returns the unique (schematic, arch) pairs across all
// nodes in cfg, marking RPi pairs that need extra firmware.
func CollectAssetSpecs(cfg *config.Config) []AssetSpec {
	if cfg == nil {
		return nil
	}
	type key struct {
		schematic, arch string
	}
	seen := map[key]*AssetSpec{}
	for _, node := range cfg.Nodes {
		k := key{
			schematic: node.EffectiveSchematic(cfg.Cluster),
			arch:      node.Arch,
		}
		if k.schematic == "" || k.arch == "" {
			continue
		}
		s, ok := seen[k]
		if !ok {
			s = &AssetSpec{
				Schematic: k.schematic,
				Arch:      k.arch,
				Version:   cfg.Cluster.TalosVersion,
			}
			seen[k] = s
		}
		if node.Overlay == "rpi_generic" {
			s.IsRPi = true
		}
	}
	// Always include cluster default if no nodes (preserves old behavior)
	if len(seen) == 0 && cfg.Cluster.SchematicID != "" {
		seen[key{cfg.Cluster.SchematicID, "amd64"}] = &AssetSpec{
			Schematic: cfg.Cluster.SchematicID,
			Arch:      "amd64",
			Version:   cfg.Cluster.TalosVersion,
		}
	}
	out := make([]AssetSpec, 0, len(seen))
	for _, s := range seen {
		out = append(out, *s)
	}
	return out
}

// BuildAll fetches Talos assets, builds iPXE, renders boot.ipxe.
//
// Backwards-compatible single-arch path. New code should prefer BuildAllNodes
// which iterates every node in config to handle multi-arch fleets.
func BuildAll(ctx context.Context, cfg *config.Config, p paths.Paths, arch string) error {
	if arch == "" {
		arch = "amd64"
	}
	if err := p.EnsureState(); err != nil {
		return err
	}
	if err := DownloadTalosAssets(ctx, cfg, p, arch); err != nil {
		return err
	}
	if err := BuildIpxe(ctx, p); err != nil {
		return err
	}
	if _, err := RenderBootIpxe(cfg, p, arch, ""); err != nil {
		return err
	}
	return nil
}

// BuildAllNodes is the multi-arch successor to BuildAll. It iterates every
// node in config, collects unique (schematic, arch) pairs, and downloads
// kernel + initramfs for each. RPi nodes also get start4.elf / fixup4.dat.
//
// iPXE is still built (amd64-only for now since it's the x86 PXE chain).
// boot.ipxe is rendered for the cluster default arch only.
func BuildAllNodes(ctx context.Context, cfg *config.Config, p paths.Paths) error {
	if err := p.EnsureState(); err != nil {
		return err
	}
	specs := CollectAssetSpecs(cfg)
	if len(specs) == 0 {
		return errors.New("no nodes in config and no cluster default schematic; nothing to build")
	}
	for _, s := range specs {
		if err := DownloadAssetsForSpec(ctx, p, s); err != nil {
			return fmt.Errorf("download assets for schematic=%s arch=%s: %w", s.Schematic, s.Arch, err)
		}
	}
	// Best-effort iPXE build (only relevant for amd64 PXE clients).
	if err := BuildIpxe(ctx, p); err != nil {
		// iPXE failures shouldn't block builds for arm64-only fleets.
		slog.Warn("iPXE build failed; PXE chain unavailable", "err", err)
	}
	// Render boot.ipxe for the first amd64 spec (legacy x86 path).
	defaultArch := "amd64"
	for _, s := range specs {
		if s.Arch == "amd64" {
			defaultArch = "amd64"
			break
		}
	}
	if _, err := RenderBootIpxe(cfg, p, defaultArch, ""); err != nil {
		return err
	}
	return nil
}

// DownloadAssetsForSpec downloads kernel + initramfs (and RPi firmware /
// raw image when applicable) for a single asset spec into the per-spec cache
// directory.
func DownloadAssetsForSpec(ctx context.Context, p paths.Paths, s AssetSpec) error {
	dir := AssetDir(p, s.Schematic, s.Arch)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	base := fmt.Sprintf("https://factory.talos.dev/image/%s/%s", s.Schematic, s.Version)
	if err := download(ctx, fmt.Sprintf("%s/kernel-%s", base, s.Arch),
		filepath.Join(dir, "vmlinuz-"+s.Arch)); err != nil {
		return err
	}
	if err := download(ctx, fmt.Sprintf("%s/initramfs-%s.xz", base, s.Arch),
		filepath.Join(dir, "initramfs-"+s.Arch+".xz")); err != nil {
		return err
	}
	// Also keep top-level shortcut copies so legacy callers keep working.
	if err := mirrorIfMissing(filepath.Join(dir, "vmlinuz-"+s.Arch),
		filepath.Join(p.Assets(), "vmlinuz-"+s.Arch)); err != nil {
		return err
	}
	if err := mirrorIfMissing(filepath.Join(dir, "initramfs-"+s.Arch+".xz"),
		filepath.Join(p.Assets(), "initramfs-"+s.Arch+".xz")); err != nil {
		return err
	}
	if s.IsRPi {
		if err := DownloadRPiFirmware(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// DownloadRPiFirmware fetches start4.elf and fixup4.dat from the official
// raspberrypi/firmware repo. Cached under <assets>/rpi-firmware/.
func DownloadRPiFirmware(ctx context.Context, p paths.Paths) error {
	dir := filepath.Join(p.Assets(), "rpi-firmware")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"start4.elf", "fixup4.dat"} {
		if err := download(ctx, rpiFirmwareBase+"/"+name, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

// DownloadTalosRawImage fetches the full metal raw image (.xz compressed)
// for a schematic + arch. Cached under <schematic>/<arch>/metal-<arch>.raw.xz.
// This is the base used by `nostos ship` for image assembly.
func DownloadTalosRawImage(ctx context.Context, p paths.Paths, s AssetSpec) (string, error) {
	dir := AssetDir(p, s.Schematic, s.Arch)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, fmt.Sprintf("metal-%s.raw.xz", s.Arch))
	url := fmt.Sprintf("https://factory.talos.dev/image/%s/%s/metal-%s.raw.xz",
		s.Schematic, s.Version, s.Arch)
	if err := download(ctx, url, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// AssetDir returns the per-(schematic, arch) cache directory.
func AssetDir(p paths.Paths, schematic, arch string) string {
	return filepath.Join(p.Assets(), schematic, arch)
}

// DownloadTalosAssets fetches kernel + initramfs from factory.talos.dev.
//
// Legacy single-arch entry point. Prefer DownloadAssetsForSpec.
func DownloadTalosAssets(ctx context.Context, cfg *config.Config, p paths.Paths, arch string) error {
	base := fmt.Sprintf("https://factory.talos.dev/image/%s/%s",
		cfg.Cluster.SchematicID, cfg.Cluster.TalosVersion)
	kernelURL := fmt.Sprintf("%s/kernel-%s", base, arch)
	initramfsURL := fmt.Sprintf("%s/initramfs-%s.xz", base, arch)

	kernel := filepath.Join(p.Assets(), "vmlinuz-"+arch)
	initramfs := filepath.Join(p.Assets(), "initramfs-"+arch+".xz")

	if err := download(ctx, kernelURL, kernel); err != nil {
		return err
	}
	return download(ctx, initramfsURL, initramfs)
}

// BuildIpxe cross-compiles snponly.efi via Docker with our embedded script.
func BuildIpxe(ctx context.Context, p paths.Paths) error {
	ipxeEfi := filepath.Join(p.Assets(), "ipxe.efi")
	embedPath := filepath.Join(p.IpxeSrc(), "src", "embed.ipxe")

	// Skip rebuild when output exists + embed script matches.
	if exists(ipxeEfi) && exists(embedPath) {
		if b, err := os.ReadFile(embedPath); err == nil && string(b) == EmbedIpxe {
			slog.Info("iPXE up to date", "path", ipxeEfi)
			return nil
		}
	}

	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("Docker is required to build iPXE in v0.1; install Docker Desktop or OrbStack, or wait for v0.2 pre-built binaries")
	}

	if err := cloneIpxe(ctx, p); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(embedPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(embedPath, []byte(EmbedIpxe), 0o644); err != nil {
		return err
	}

	slog.Info("building iPXE snponly.efi via Docker")
	buildCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(buildCtx,
		"docker", "run", "--rm", "--platform", "linux/amd64",
		"-v", p.IpxeSrc()+":/ipxe",
		"-w", "/ipxe/src",
		"alpine:latest", "sh", "-c",
		"apk add --no-cache --quiet build-base perl xz-dev mtools gnu-efi-dev >/dev/null && "+
			"make -j4 bin-x86_64-efi/snponly.efi EMBED=embed.ipxe NO_WERROR=1",
	).CombinedOutput()
	if err != nil {
		tail := string(out)
		if len(tail) > 1024 {
			tail = "..." + tail[len(tail)-1024:]
		}
		return fmt.Errorf("iPXE build failed: %v\n%s", err, tail)
	}

	built := filepath.Join(p.IpxeSrc(), "src", "bin-x86_64-efi", "snponly.efi")
	if !exists(built) {
		return fmt.Errorf("iPXE build did not produce %s", built)
	}
	if err := copyFile(built, ipxeEfi); err != nil {
		return err
	}
	info, err := os.Stat(ipxeEfi)
	if err != nil {
		return err
	}
	if info.Size() > IpxeMaxSizeBytes {
		return fmt.Errorf("iPXE binary is %d bytes, exceeds Dell UEFI TFTP limit %d",
			info.Size(), IpxeMaxSizeBytes)
	}
	slog.Info("iPXE built", "path", ipxeEfi, "bytes", info.Size())
	return nil
}

// RenderBootIpxe writes state/assets/boot.ipxe. extraKernelArgs is appended
// after talos.config= (used for talos.experimental.wipe=system during wipes).
func RenderBootIpxe(cfg *config.Config, p paths.Paths, arch, extraKernelArgs string) (string, error) {
	if arch == "" {
		arch = "amd64"
	}
	if err := os.MkdirAll(p.Assets(), 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(p.Assets(), "boot.ipxe")
	body := fmt.Sprintf(BootIpxeTemplate, cfg.Cluster.TalosVersion, arch, extraKernelArgs, arch)
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		return "", err
	}
	return out, nil
}

// --- internals ---

func download(ctx context.Context, url, dest string) error {
	if exists(dest) {
		slog.Info("asset up to date", "path", dest)
		return nil
	}
	slog.Info("downloading", "url", url)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	tmp := dest + ".partial"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, dest)
}

// mirrorIfMissing copies src to dst when dst is absent. Used to keep
// top-level <assets>/vmlinuz-<arch> shortcuts in sync with the per-schematic
// cache so legacy callers keep working.
func mirrorIfMissing(src, dst string) error {
	if exists(dst) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return copyFile(src, dst)
}

func cloneIpxe(ctx context.Context, p paths.Paths) error {
	if exists(filepath.Join(p.IpxeSrc(), ".git")) {
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git is required to clone iPXE source")
	}
	slog.Info("cloning iPXE", "path", p.IpxeSrc())
	c, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(c, "git", "clone", "--depth", "1", IpxeRepo, p.IpxeSrc()).CombinedOutput(); err != nil {
		return fmt.Errorf("iPXE clone failed: %s", string(out))
	}
	return nil
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func copyFile(src, dst string) error {
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
