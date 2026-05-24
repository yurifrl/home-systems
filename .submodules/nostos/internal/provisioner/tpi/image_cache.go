package tpi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"context"

	"github.com/ulikunitz/xz"
)

// imageURLBase is the factory.talos.dev base; var so tests can override.
var imageURLBase = "https://factory.talos.dev"

// cacheDirFn is a seam so tests can redirect the per-user image cache.
var cacheDirFn = defaultCacheDir

// imageCache resolves a Talos factory image to a local .raw path,
// optionally verifying against a pinned sha256. When pinned == "",
// the cache operates in TOFU mode and records the observed digest in
// digestStore (a JSON map keyed by "<schematic>/<version>/<arch>").
type imageCache struct {
	schematic   string
	version     string
	arch        string
	pinned      string      // optional; "" => TOFU
	digestStore string      // path to digests.json (TOFU mode only)
	warn        func(string) // optional sink for hard warnings
}

func defaultCacheDir(schematic, version string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "nostos", "images", schematic, version), nil
}

func (c imageCache) emitWarn(msg string) {
	if c.warn != nil {
		c.warn(msg)
		return
	}
	slog.Warn(msg)
}

// Ensure downloads the .raw.xz, verifies sha256, decompresses to .raw,
// and returns the .raw path. Idempotent: cache hit short-circuits.
func (c imageCache) Ensure(ctx context.Context) (string, error) {
	dir, err := cacheDirFn(c.schematic, c.version)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	xzPath := filepath.Join(dir, "metal-"+c.arch+".raw.xz")
	rawPath := filepath.Join(dir, "metal-"+c.arch+".raw")
	key := imageDigestKey(c.schematic, c.version, c.arch)

	// Resolve expected digest: pinned (strict) or recorded (TOFU).
	expected := strings.TrimSpace(c.pinned)
	tofu := expected == ""
	if tofu && c.digestStore != "" {
		if rec, ok, err := readRecordedDigest(c.digestStore, key); err != nil {
			return "", err
		} else if ok {
			expected = rec
		}
	}

	// Cache-hit path.
	if fi, err := os.Stat(xzPath); err == nil && fi.Size() > 0 {
		got, err := sha256File(xzPath)
		if err == nil {
			switch {
			case expected != "" && digestMatches(got, expected):
				if _, err := os.Stat(rawPath); err == nil {
					return rawPath, nil
				}
				if err := decompressXZ(xzPath, rawPath); err == nil {
					return rawPath, nil
				}
			case expected != "" && !digestMatches(got, expected):
				if tofu {
					c.emitWarn(fmt.Sprintf("tpi: cached image %s digest drift (recorded=%s got=sha256:%s); re-downloading", xzPath, expected, got))
				}
				_ = os.Remove(xzPath)
				_ = os.Remove(rawPath)
			case expected == "":
				// TOFU with no record yet: trust nothing on disk; redownload.
				_ = os.Remove(xzPath)
				_ = os.Remove(rawPath)
			}
		} else {
			_ = os.Remove(xzPath)
		}
	}

	// Download.
	url := fmt.Sprintf("%s/image/%s/%s/metal-%s.raw.xz", imageURLBase, c.schematic, c.version, c.arch)
	tmp := xzPath + ".part"
	if err := downloadTo(ctx, url, tmp); err != nil {
		return "", err
	}
	got, err := sha256File(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if expected != "" {
		if !digestMatches(got, expected) {
			_ = os.Remove(tmp)
			return "", fmt.Errorf("tpi: image digest mismatch: expected %s got sha256:%s", expected, got)
		}
	} else {
		// TOFU first observation: record it.
		if c.digestStore != "" {
			if err := writeRecordedDigest(c.digestStore, key, "sha256:"+got); err != nil {
				_ = os.Remove(tmp)
				return "", fmt.Errorf("tpi: record digest: %w", err)
			}
		}
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, xzPath); err != nil {
		return "", err
	}
	if err := decompressXZ(xzPath, rawPath); err != nil {
		return "", err
	}
	return rawPath, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func digestMatches(gotHex, expected string) bool {
	expected = strings.TrimPrefix(expected, "sha256:")
	return strings.EqualFold(gotHex, expected)
}

func downloadTo(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func decompressXZ(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	r, err := xz.NewReader(in)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

// readRecordedDigest reads digestStore (JSON map) and returns the value
// for key. ok=false when file does not exist or key is absent.
func readRecordedDigest(path, key string) (string, bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if len(b) == 0 {
		return "", false, nil
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return "", false, fmt.Errorf("parse %s: %w", path, err)
	}
	v, ok := m[key]
	return v, ok, nil
}

// writeRecordedDigest atomically merges {key: digest} into digestStore.
func writeRecordedDigest(path, key, digest string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	m := map[string]string{}
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &m) // tolerate corrupt; we'll overwrite.
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	m[key] = digest
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".digests.json.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = ""
	return nil
}
