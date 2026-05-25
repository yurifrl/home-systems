package tpi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestImageCacheTOFUTorture simulates the SIGKILL-mid-download race: a
// partial <xz>.tmp from a prior run with no <xz>. Ensure() must observe
// only one of {nothing}, {<xz> + recorded digest}. Never {<xz>.tmp + no <xz>}
// and never partial bytes claimed as full.
func TestImageCacheTOFUTorture(t *testing.T) {
	payload := []byte("torture-payload-bytes-of-interest")
	xzBytes := makeXZ(t, payload)
	want := sha256Hex(xzBytes)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(xzBytes)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for i := 0; i < 100; i++ {
		cache := withImageEnv(t, srv.URL)
		store := filepath.Join(t.TempDir(), "digests.json")

		// Simulate a prior crash: write a random prefix of the expected bytes
		// to <xz>.tmp WITHOUT producing the final <xz> or recording a digest.
		dir := filepath.Join(cache, "sch", "v1")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		prefixLen := randInt(t, len(xzBytes))
		tmpPath := filepath.Join(dir, "metal-arm64.raw.xz.tmp")
		if err := os.WriteFile(tmpPath, xzBytes[:prefixLen], 0o600); err != nil {
			t.Fatal(err)
		}
		// Ensure no .xz or .raw exists.
		_ = os.Remove(filepath.Join(dir, "metal-arm64.raw.xz"))
		_ = os.Remove(filepath.Join(dir, "metal-arm64.raw"))

		c := imageCache{schematic: "sch", version: "v1", arch: "arm64", digestStore: store}
		if _, err := c.Ensure(context.Background()); err != nil {
			t.Fatalf("iter %d Ensure: %v", i, err)
		}

		// Invariant 1: no orphan .tmp.
		if _, err := os.Stat(tmpPath); err == nil {
			t.Fatalf("iter %d: orphan .tmp survived after Ensure", i)
		}

		// Invariant 2: final exists and matches expected digest.
		xzPath := filepath.Join(dir, "metal-arm64.raw.xz")
		got, err := sha256File(xzPath)
		if err != nil {
			t.Fatalf("iter %d: stat final: %v", i, err)
		}
		if got != want {
			t.Fatalf("iter %d: final xz digest %q != want %q", i, got, want)
		}

		// Invariant 3: digests.json records the same digest (TOFU recorded it,
		// not the partial we planted).
		b, err := os.ReadFile(store)
		if err != nil {
			t.Fatalf("iter %d: read store: %v", i, err)
		}
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("iter %d: parse store: %v", i, err)
		}
		if rec := m["sch/v1/arm64"]; rec != "sha256:"+want {
			t.Fatalf("iter %d: recorded digest %q, want sha256:%s", i, rec, want)
		}
	}
}

// TestImageCacheOrphanTmpCleanedOnEntry pins the explicit cleanup behaviour:
// if <xz>.tmp exists with no <xz>, the next Ensure must remove the tmp
// before doing anything else.
func TestImageCacheOrphanTmpCleanedOnEntry(t *testing.T) {
	// Server that 500s — Ensure() would normally fail. The fact that the
	// orphan tmp must be gone regardless is what we are pinning.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	cache := withImageEnv(t, srv.URL)

	dir := filepath.Join(cache, "sch", "v1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	tmpPath := filepath.Join(dir, "metal-arm64.raw.xz.tmp")
	if err := os.WriteFile(tmpPath, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := filepath.Join(t.TempDir(), "digests.json")
	c := imageCache{schematic: "sch", version: "v1", arch: "arm64", digestStore: store}
	_, _ = c.Ensure(context.Background()) // expected to fail, that's fine

	if _, err := os.Stat(tmpPath); err == nil {
		t.Fatalf("orphan .tmp not removed on entry")
	}
}

func randInt(t *testing.T, max int) int {
	t.Helper()
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		t.Fatal(err)
	}
	return int(n.Int64())
}
