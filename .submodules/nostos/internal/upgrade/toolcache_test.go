package upgrade

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTalosctlURL(t *testing.T) {
	cases := []struct {
		v            Version
		goos, goarch string
		want         string
	}{
		{Version{1, 10, 3}, "darwin", "arm64",
			"https://github.com/siderolabs/talos/releases/download/v1.10.3/talosctl-darwin-arm64"},
		{Version{1, 13, 3}, "linux", "amd64",
			"https://github.com/siderolabs/talos/releases/download/v1.13.3/talosctl-linux-amd64"},
		{Version{1, 11, 6}, "linux", "arm64",
			"https://github.com/siderolabs/talos/releases/download/v1.11.6/talosctl-linux-arm64"},
	}
	for _, c := range cases {
		if got := talosctlURL(c.v, c.goos, c.goarch); got != c.want {
			t.Errorf("talosctlURL(%v, %q, %q) = %q, want %q", c.v, c.goos, c.goarch, got, c.want)
		}
	}
}

func TestParseClientVersionShort(t *testing.T) {
	v, err := parseClientVersion("Client: v1.10.3\nServer: v1.12.0\n")
	if err != nil {
		t.Fatal(err)
	}
	if v != (Version{1, 10, 3}) {
		t.Errorf("got %v, want v1.10.3", v)
	}
}

// TestParseClientVersionShortReal pins the ACTUAL `talosctl version --client
// --short` output, which prints a "Talos vX.Y.Z" line under "Client:".
func TestParseClientVersionShortReal(t *testing.T) {
	v, err := parseClientVersion("Client:\nTalos v1.10.3\n")
	if err != nil {
		t.Fatal(err)
	}
	if v != (Version{1, 10, 3}) {
		t.Errorf("got %v, want v1.10.3", v)
	}
}

func TestParseClientVersionMultiLine(t *testing.T) {
	v, err := parseClientVersion("Client:\n\tTag: v1.13.3\nServer:\n\tTag: v1.10.3\n")
	if err != nil {
		t.Fatal(err)
	}
	if v != (Version{1, 13, 3}) {
		t.Errorf("got %v, want v1.13.3", v)
	}
}

func TestParseClientVersionMissing(t *testing.T) {
	if _, err := parseClientVersion("Server:\n\tTag: v1.10.3\n"); err == nil {
		t.Fatal("expected error when no client version present")
	}
}

// fakeDownloader serves bytes from a map and records the requested URL plus
// call count, so tests can assert whether a download happened.
type fakeDownloader struct {
	bodies map[string]string
	gotURL string
	calls  int
}

func (f *fakeDownloader) Get(ctx context.Context, url string) (io.ReadCloser, error) {
	f.calls++
	f.gotURL = url
	body, ok := f.bodies[url]
	if !ok {
		return nil, io.EOF
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

// withStubVerify swaps the package-level verifyClientVersion for the duration
// of a test so cache/download logic can be exercised without exec'ing a real
// talosctl binary.
func withStubVerify(t *testing.T, fn func(path string, v Version) error) {
	t.Helper()
	orig := verifyClientVersion
	verifyClientVersion = fn
	t.Cleanup(func() { verifyClientVersion = orig })
}

func TestEnsureTalosctlCacheHit(t *testing.T) {
	dir := t.TempDir()
	v := Version{1, 10, 3}
	dst := filepath.Join(dir, "talosctl-v1.10.3")
	// Pre-create an executable file at the cache path.
	if err := os.WriteFile(dst, []byte("#!/bin/sh\necho client\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	withStubVerify(t, func(path string, want Version) error {
		if path != dst {
			t.Errorf("verify path = %q, want %q", path, dst)
		}
		return nil // pretend it reports the right version
	})

	dl := &fakeDownloader{bodies: map[string]string{}}
	got, err := EnsureTalosctl(context.Background(), v, dir, dl)
	if err != nil {
		t.Fatal(err)
	}
	if got != dst {
		t.Errorf("path = %q, want %q", got, dst)
	}
	if dl.calls != 0 {
		t.Errorf("downloader called %d times on cache hit, want 0", dl.calls)
	}
}

func TestEnsureTalosctlDownload(t *testing.T) {
	dir := t.TempDir()
	v := Version{1, 11, 6}
	url := talosctlURL(v, runtime.GOOS, runtime.GOARCH)
	dl := &fakeDownloader{bodies: map[string]string{
		url: "#!/bin/sh\necho v1.11.6\n",
	}}
	withStubVerify(t, func(path string, want Version) error {
		// Verify the file was written, chmod'd, and is the expected dst.
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat downloaded binary: %v", err)
		}
		if fi.Mode().Perm()&0o111 == 0 {
			t.Errorf("downloaded binary not executable: %v", fi.Mode())
		}
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "v1.11.6") {
			t.Errorf("downloaded content = %q, missing version marker", string(data))
		}
		return nil
	})

	got, err := EnsureTalosctl(context.Background(), v, dir, dl)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "talosctl-v1.11.6")
	if got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	if dl.calls != 1 {
		t.Errorf("downloader called %d times, want 1", dl.calls)
	}
	if dl.gotURL != url {
		t.Errorf("downloaded URL = %q, want %q", dl.gotURL, url)
	}
}
