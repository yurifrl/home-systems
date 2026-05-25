// Package upstream fetches and caches latest-version metadata for Talos
// (and, in v0.4, helm charts and container images).
//
// The cache lives at ${XDG_CACHE_HOME:-~/.cache}/nostos/upstream-versions.json
// with a 24h TTL. Offline laptops fall back to whatever is on disk.
package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CacheTTL is the duration we trust the on-disk snapshot before re-fetching.
const CacheTTL = 24 * time.Hour

// Versions is the cached payload.
type Versions struct {
	TalosLatest string    `json:"talos_latest"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// CachePath returns the full path to the on-disk cache.
func CachePath() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, ".cache")
		} else {
			base = filepath.Join(os.TempDir(), "nostos-cache")
		}
	}
	return filepath.Join(base, "nostos", "upstream-versions.json")
}

// LoadCache reads the cached Versions or returns zero value + ok=false.
func LoadCache() (Versions, bool) {
	path := CachePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return Versions{}, false
	}
	var v Versions
	if err := json.Unmarshal(data, &v); err != nil {
		return Versions{}, false
	}
	return v, true
}

// SaveCache writes v atomically.
func SaveCache(v Versions) error {
	path := CachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Fetch performs the HTTP probe to factory.talos.dev/versions and returns the
// latest version string (e.g. "v1.12.0"). Falls back to cache if HTTP fails.
func Fetch(ctx context.Context) (Versions, error) {
	if cached, ok := LoadCache(); ok && time.Since(cached.FetchedAt) < CacheTTL {
		return cached, nil
	}
	v, err := fetchTalosLatest(ctx)
	if err != nil {
		// offline fall-through
		if cached, ok := LoadCache(); ok {
			return cached, nil
		}
		return Versions{}, err
	}
	out := Versions{TalosLatest: v, FetchedAt: time.Now()}
	_ = SaveCache(out)
	return out, nil
}

func fetchTalosLatest(ctx context.Context) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", "https://factory.talos.dev/versions", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var versions []string
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", nil
	}
	// The list is roughly chronological; pick the lexicographically highest stable v1.x.
	best := versions[0]
	for _, v := range versions {
		if !strings.HasPrefix(v, "v") {
			continue
		}
		if compareSemver(v, best) > 0 {
			best = v
		}
	}
	return best, nil
}

// CountMinorBehind returns how many minor releases current is behind latest.
// Returns 0 if either is unparseable or current >= latest.
func CountMinorBehind(current, latest string) int {
	cM, cm := parseSemver(current)
	lM, lm := parseSemver(latest)
	if cM == 0 && cm == 0 {
		return 0
	}
	if cM != lM {
		return (lM - cM) * 100 // major bump dominates
	}
	if lm > cm {
		return lm - cm
	}
	return 0
}

func parseSemver(v string) (int, int) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0
	}
	maj, _ := strconv.Atoi(parts[0])
	min, _ := strconv.Atoi(parts[1])
	return maj, min
}

func compareSemver(a, b string) int {
	aM, am := parseSemver(a)
	bM, bm := parseSemver(b)
	if aM != bM {
		return aM - bM
	}
	if am != bm {
		return am - bm
	}
	return strings.Compare(a, b)
}
