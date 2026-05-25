package tpi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
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
	pinned      string       // optional; "" => TOFU
	digestStore string       // path to digests.json (TOFU mode only)
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
//
// TOFU two-phase commit (A2):
//   - Stream-hash during download into <xz>.tmp via io.MultiWriter.
//   - Fsync .tmp, atomic rename .tmp → final, fsync directory.
//   - Only then write digests.json (atomic rename + fsync).
//   - On entry: if <xz>.tmp exists with no <xz>, delete the tmp.
//     We never recover from a partial — there is no way to know whether
//     the bytes on disk are a complete download or a torn write.
func (c imageCache) Ensure(ctx context.Context) (string, error) {
	dir, err := cacheDirFn(c.schematic, c.version)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	xzPath := filepath.Join(dir, "metal-"+c.arch+".raw.xz")
	tmpPath := xzPath + ".tmp"
	rawPath := filepath.Join(dir, "metal-"+c.arch+".raw")
	key := imageDigestKey(c.schematic, c.version, c.arch)

	// Crash-recovery: orphan .tmp without final → DELETE. Never trust partials.
	if _, err := os.Stat(tmpPath); err == nil {
		if _, ferr := os.Stat(xzPath); os.IsNotExist(ferr) {
			_ = os.Remove(tmpPath)
		} else if ferr == nil {
			// final exists; tmp is leftover from a crash after rename — drop it.
			_ = os.Remove(tmpPath)
		}
	}

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

	// Stream download into <xz>.tmp while hashing in-stream.
	url := fmt.Sprintf("%s/image/%s/%s/metal-%s.raw.xz", imageURLBase, c.schematic, c.version, c.arch)
	got, err := streamDownload(ctx, url, tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	if expected != "" && !digestMatches(got, expected) {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("tpi: image digest mismatch: expected %s got sha256:%s", expected, got)
	}

	// Phase commit: atomic rename .tmp → final, fsync directory.
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, xzPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := fsyncDir(dir); err != nil {
		// Non-fatal but note: a power loss right here can lose the rename.
		// We accept this — next run will re-download (no orphan tmp).
		slog.Warn("tpi: fsync image dir", "err", err)
	}

	// Only AFTER the image is durably renamed do we record the digest.
	if expected == "" && c.digestStore != "" {
		if err := writeRecordedDigest(c.digestStore, key, "sha256:"+got); err != nil {
			return "", fmt.Errorf("tpi: record digest: %w", err)
		}
	}

	if err := decompressXZ(xzPath, rawPath); err != nil {
		return "", err
	}
	return rawPath, nil
}

// streamDownload writes resp.Body to dst while hashing in-stream and
// fsyncs dst before returning. Returns the lower-case hex sha256.
func streamDownload(ctx context.Context, url, dst string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	mw := io.MultiWriter(out, h)
	if _, err := io.Copy(mw, resp.Body); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// hashSink is unused outside Ensure but exposed to allow other call sites
// to share the same hashing primitive without copy-paste.
var _ hash.Hash = sha256.New()

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
// Writes to .tmp, fsyncs, renames, fsyncs the parent dir.
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
	if err := tmp.Sync(); err != nil {
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
	_ = fsyncDir(filepath.Dir(path))
	return nil
}
