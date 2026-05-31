// Package flock provides a per-node cross-process exclusive lock backed
// by flock(2) (LOCK_EX|LOCK_NB) on a file under the nostos configs cache.
package flock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/yurifrl/nostos/internal/provisioner"
)

// DefaultDir is used when AcquireNode is called without an override.
// Production callers SHOULD use AcquireNodeAt to supply paths.Configs().
var DefaultDir = filepath.Join(defaultStateDir(), "configs")

func defaultStateDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "nostos")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", "nostos")
	}
	return filepath.Join("nostos", "state")
}

// AcquireNode takes an exclusive non-blocking flock on
// <DefaultDir>/<name>.lock (mode 0600). On contention it returns an
// error wrapping provisioner.ErrLocked whose message contains the
// lockfile path.
func AcquireNode(name string) (release func(), err error) {
	return AcquireNodeAt(DefaultDir, name)
}

// AcquireNodeAt is the dependency-injectable variant.
func AcquireNodeAt(dir, name string) (release func(), err error) {
	if name == "" {
		return nil, errors.New("flock: empty node name")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("flock: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, name+".lock")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("flock: open %s: %w", path, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: lockfile %s held by another process", provisioner.ErrLocked, path)
		}
		return nil, fmt.Errorf("flock: %s: %w", path, err)
	}

	released := false
	return func() {
		if released {
			return
		}
		released = true
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}
