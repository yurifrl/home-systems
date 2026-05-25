// Package cache persists the last full dashboard snapshot to disk so the TUI
// can render something useful at cold start before the first live refresh.
//
// The cache is at ${XDG_CACHE_HOME:-~/.cache}/nostos/dashboard-state.json,
// mode 0600, with a `cached_at` timestamp so the TUI can show staleness and
// mark cached rows with `~` until fresh data lands.
//
// Cache is NOT used in headless --once mode: that path always runs fresh.
package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
)

// State is the on-disk envelope.
type State struct {
	CachedAt time.Time         `json:"cached_at"`
	Snap     snapshot.Snapshot `json:"snapshot"`
}

// Path returns the cache file path.
func Path() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, ".cache")
		} else {
			base = filepath.Join(os.TempDir(), "nostos-cache")
		}
	}
	return filepath.Join(base, "nostos", "dashboard-state.json")
}

// Load reads the cached State; returns ok=false when missing or corrupt.
func Load() (State, bool) {
	return LoadFrom(Path())
}

// LoadFrom reads State from a custom path (tests).
func LoadFrom(path string) (State, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, false
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, false
	}
	return s, true
}

// Save writes the snapshot atomically with mode 0600.
func Save(snap snapshot.Snapshot) error {
	return SaveTo(Path(), snap)
}

// SaveTo writes to an explicit path (tests).
func SaveTo(path string, snap snapshot.Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	st := State{CachedAt: time.Now(), Snap: snap}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		// rename across mounts? fall back to direct write
		_ = os.WriteFile(path, b, 0o600)
	}
	return nil
}

// MarkCached annotates every row label with a leading `~` to indicate the
// data came from disk and isn't yet fresh. Idempotent.
func MarkCached(s *snapshot.Snapshot) {
	for i := range s.Nodes {
		if len(s.Nodes[i].Name) > 0 && s.Nodes[i].Name[0] != '~' {
			s.Nodes[i].Name = "~" + s.Nodes[i].Name
		}
	}
	for i := range s.Discoveries {
		if len(s.Discoveries[i].Hostname) > 0 && s.Discoveries[i].Hostname[0] != '~' {
			s.Discoveries[i].Hostname = "~" + s.Discoveries[i].Hostname
		}
	}
}

// ErrEmpty is returned when no cache could be loaded.
var ErrEmpty = errors.New("dashboard cache empty")
