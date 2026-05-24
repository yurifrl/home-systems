package tpi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ulikunitz/xz"
)

// makeXZ returns the .xz-encoded form of payload.
func makeXZ(t *testing.T, payload []byte) []byte {
	t.Helper()
	tmp, err := os.CreateTemp(t.TempDir(), "buf.xz")
	if err != nil {
		t.Fatal(err)
	}
	defer tmp.Close()
	w, err := xz.NewWriter(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Sync(); err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func newImageServer(t *testing.T, body []byte, hits *int64) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(hits, 1)
		w.WriteHeader(200)
		_, _ = w.Write(body)
	})
	return httptest.NewServer(mux)
}

// withImageEnv sets up imageURLBase + isolated cacheDir.
func withImageEnv(t *testing.T, base string) string {
	t.Helper()
	prevBase := imageURLBase
	prevDirFn := cacheDirFn
	cache := t.TempDir()
	imageURLBase = base
	cacheDirFn = func(schematic, version string) (string, error) {
		return filepath.Join(cache, schematic, version), nil
	}
	t.Cleanup(func() {
		imageURLBase = prevBase
		cacheDirFn = prevDirFn
	})
	return cache
}

func TestImageCachePinnedCorrectDigest(t *testing.T) {
	payload := []byte("talos-image-bytes")
	xzBytes := makeXZ(t, payload)
	want := sha256Hex(xzBytes)

	var hits int64
	srv := newImageServer(t, xzBytes, &hits)
	defer srv.Close()
	withImageEnv(t, srv.URL)

	c := imageCache{schematic: "sch", version: "v1", arch: "arm64", pinned: "sha256:" + want}
	raw, err := c.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	got, err := os.ReadFile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("decoded payload mismatch")
	}
	if hits != 1 {
		t.Fatalf("expected 1 download, got %d", hits)
	}
}

func TestImageCachePinnedWrongDigestErrors(t *testing.T) {
	payload := []byte("talos-image-bytes")
	xzBytes := makeXZ(t, payload)

	var hits int64
	srv := newImageServer(t, xzBytes, &hits)
	defer srv.Close()
	withImageEnv(t, srv.URL)

	c := imageCache{schematic: "sch", version: "v1", arch: "arm64", pinned: "sha256:deadbeef"}
	if _, err := c.Ensure(context.Background()); err == nil {
		t.Fatal("expected digest mismatch error")
	}
	// .part should have been removed; xz should NOT be persisted.
	dir, _ := cacheDirFn("sch", "v1")
	if _, err := os.Stat(filepath.Join(dir, "metal-arm64.raw.xz")); !os.IsNotExist(err) {
		t.Fatalf("xz persisted on mismatch: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "metal-arm64.raw.xz.part")); !os.IsNotExist(err) {
		t.Fatalf(".part persisted on mismatch: err=%v", err)
	}
}

func TestImageCacheTOFURecordsDigest(t *testing.T) {
	payload := []byte("hello world")
	xzBytes := makeXZ(t, payload)
	want := sha256Hex(xzBytes)

	var hits int64
	srv := newImageServer(t, xzBytes, &hits)
	defer srv.Close()
	withImageEnv(t, srv.URL)

	store := filepath.Join(t.TempDir(), "cache", "digests.json")
	c := imageCache{schematic: "sch", version: "v1", arch: "arm64", pinned: "", digestStore: store}
	if _, err := c.Ensure(context.Background()); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 download, got %d", hits)
	}
	b, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if got := m["sch/v1/arm64"]; got != "sha256:"+want {
		t.Fatalf("recorded digest = %q want sha256:%s", got, want)
	}
	// Permission 0600.
	fi, _ := os.Stat(store)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("digests.json mode = %o", fi.Mode().Perm())
	}
}

func TestImageCacheTOFUSecondCallNoRedownload(t *testing.T) {
	payload := []byte("repeat me")
	xzBytes := makeXZ(t, payload)

	var hits int64
	srv := newImageServer(t, xzBytes, &hits)
	defer srv.Close()
	withImageEnv(t, srv.URL)

	store := filepath.Join(t.TempDir(), "cache", "digests.json")
	c := imageCache{schematic: "sch", version: "v1", arch: "arm64", digestStore: store}
	if _, err := c.Ensure(context.Background()); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	if hits != 1 {
		t.Fatalf("after first call hits=%d want 1", hits)
	}
	atomic.StoreInt64(&hits, 0)
	if _, err := c.Ensure(context.Background()); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if hits != 0 {
		t.Fatalf("second call hit server %d times, want 0", hits)
	}
}

func TestImageCacheTOFUDriftRedownloads(t *testing.T) {
	payload := []byte("first-payload")
	xzBytes := makeXZ(t, payload)

	var hits int64
	srv := newImageServer(t, xzBytes, &hits)
	defer srv.Close()
	withImageEnv(t, srv.URL)

	store := filepath.Join(t.TempDir(), "cache", "digests.json")
	var warned []string
	c := imageCache{schematic: "sch", version: "v1", arch: "arm64", digestStore: store, warn: func(s string) { warned = append(warned, s) }}
	if _, err := c.Ensure(context.Background()); err != nil {
		t.Fatalf("first Ensure: %v", err)
	}

	// Tamper: corrupt the cached xz so its digest no longer matches recorded.
	dir, _ := cacheDirFn("sch", "v1")
	xzPath := filepath.Join(dir, "metal-arm64.raw.xz")
	if err := os.WriteFile(xzPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Server now serves a DIFFERENT payload, simulating drift; recorded digest in
	// store still references the *original* digest, so detection fires.
	atomic.StoreInt64(&hits, 0)

	if _, err := c.Ensure(context.Background()); err != nil {
		// On drift we redownload original-server bytes; their digest matches the recorded one.
		t.Fatalf("Ensure after drift: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 redownload, got %d", hits)
	}
	if len(warned) == 0 {
		t.Fatalf("expected drift warning")
	}
}
