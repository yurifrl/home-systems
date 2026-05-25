package upstream

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	if err := SaveCache(Versions{TalosLatest: "v1.12.0", FetchedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nostos", "upstream-versions.json")); err != nil {
		t.Fatal(err)
	}
	v, ok := LoadCache()
	if !ok || v.TalosLatest != "v1.12.0" {
		t.Fatalf("load mismatch: %+v ok=%v", v, ok)
	}
}

func TestCountMinorBehind(t *testing.T) {
	cases := []struct {
		cur, lat string
		want     int
	}{
		{"v1.10.3", "v1.12.0", 2},
		{"v1.12.0", "v1.12.0", 0},
		{"v1.12.0", "v1.10.0", 0},
		{"v1.10.0", "v2.0.0", 100}, // major bump
	}
	for _, c := range cases {
		got := CountMinorBehind(c.cur, c.lat)
		if got != c.want {
			t.Fatalf("%s vs %s: got %d want %d", c.cur, c.lat, got, c.want)
		}
	}
}
