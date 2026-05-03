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
)

// BuildAll fetches Talos assets, builds iPXE, renders boot.ipxe.
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

// DownloadTalosAssets fetches kernel + initramfs from factory.talos.dev.
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
