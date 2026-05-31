package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Downloader is the minimal fetch surface EnsureTalosctl needs to retrieve a
// talosctl binary. It mirrors the injectable Doer pattern from catalog.go so
// tests can serve canned bytes without real network access. The default impl
// (httpDownloader) uses net/http; pass nil to EnsureTalosctl to use it.
type Downloader interface {
	Get(ctx context.Context, url string) (io.ReadCloser, error)
}

// httpDownloader is the default net/http-backed Downloader.
type httpDownloader struct {
	client *http.Client
}

func (d httpDownloader) Get(ctx context.Context, url string) (io.ReadCloser, error) {
	c := d.client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("download %s returned %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

// talosctlURL builds the GitHub release asset URL for a given talosctl version
// and target platform. Pure helper for testability.
func talosctlURL(v Version, goos, goarch string) string {
	return fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/talosctl-%s-%s",
		v.String(), goos, goarch)
}

// parseClientVersion extracts the CLIENT Talos version from `talosctl version
// --client` output (handles both the "--short" single-line "Client: v1.10.3"
// form and the multi-line "Client:\n\tTag: v1.10.3" form). Pure helper.
func parseClientVersion(out string) (Version, error) {
	lines := strings.Split(out, "\n")
	inClient := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Client:"):
			inClient = true
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "Client:"))
			if rest != "" {
				if v, err := ParseVersion(rest); err == nil {
					return v, nil
				}
			}
		case strings.HasPrefix(trimmed, "Server:"):
			inClient = false
		case inClient && strings.HasPrefix(trimmed, "Tag:"):
			tag := strings.TrimSpace(strings.TrimPrefix(trimmed, "Tag:"))
			return ParseVersion(tag)
		case inClient && strings.HasPrefix(trimmed, "Talos "):
			// `--short` form prints a "Talos vX.Y.Z" line under "Client:".
			tag := strings.TrimSpace(strings.TrimPrefix(trimmed, "Talos "))
			if v, err := ParseVersion(tag); err == nil {
				return v, nil
			}
		}
	}
	return Version{}, fmt.Errorf("could not find client version in talosctl output")
}

// ClientVersion runs `<path> version --client` and parses the reported client
// version. It bounds the call with a short context timeout.
func ClientVersion(path string) (Version, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version", "--client", "--short").CombinedOutput()
	if err != nil {
		return Version{}, fmt.Errorf("%s version --client: %s", path, strings.TrimSpace(string(out)))
	}
	return parseClientVersion(string(out))
}

// verifyClientVersion is the indirection point used by EnsureTalosctl to check
// that a binary at path reports version v. It defaults to a real exec via
// ClientVersion but is a package var so tests can stub it without executing a
// real binary.
var verifyClientVersion = func(path string, v Version) error {
	got, err := ClientVersion(path)
	if err != nil {
		return err
	}
	if got != v {
		return fmt.Errorf("talosctl at %s reports %s, want %s", path, got, v)
	}
	return nil
}

// isExecutable reports whether the file at path exists and has any exec bit.
func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode().Perm()&0o111 != 0
}

// EnsureTalosctl returns the path to a talosctl binary matching version v,
// downloading and caching it under cacheDir if needed. The cache path is
// filepath.Join(cacheDir, "talosctl-"+v.String()).
//
// If a cached binary exists, is executable, and reports v, it is returned as a
// cache hit without touching the Downloader. Otherwise the matching release
// asset is downloaded (via dl, or a default net/http downloader when nil),
// written to a temp file, chmod 0o755, atomically renamed into place, and then
// verified to report v.
func EnsureTalosctl(ctx context.Context, v Version, cacheDir string, dl Downloader) (string, error) {
	dst := filepath.Join(cacheDir, "talosctl-"+v.String())

	// Cache hit: existing executable that reports the right version.
	if isExecutable(dst) {
		if err := verifyClientVersion(dst, v); err == nil {
			return dst, nil
		}
		// Stale/corrupt cache entry — fall through to re-download.
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	if dl == nil {
		dl = httpDownloader{}
	}
	url := talosctlURL(v, runtime.GOOS, runtime.GOARCH)
	rc, err := dl.Get(ctx, url)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	tmp, err := os.CreateTemp(cacheDir, "talosctl-*.download")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename

	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write talosctl: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close talosctl: %w", err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", fmt.Errorf("chmod talosctl: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return "", fmt.Errorf("install talosctl: %w", err)
	}

	if err := verifyClientVersion(dst, v); err != nil {
		return "", fmt.Errorf("verify downloaded talosctl: %w", err)
	}
	return dst, nil
}
