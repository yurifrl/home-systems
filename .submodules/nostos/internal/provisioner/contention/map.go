// Package contention provides an in-process keyed mutex used by the
// orchestrator to serialize installs that share a scarce resource (e.g.
// every slot on a Turing Pi shares one BMC).
package contention

import "sync"

// Map is a keyed mutex: callers Acquire(key) and receive a release
// closure. Same key serializes; distinct keys are independent.
//
// The zero value is ready to use.
type Map struct {
	mu    sync.Mutex
	locks map[string]*entry
}

type entry struct {
	mu  sync.Mutex
	ref int // refcount of pending+holding callers
}

// Acquire blocks until the lock for key is held by the caller. The
// returned function MUST be called exactly once to release. Empty keys
// return a no-op release function and never block.
func (m *Map) Acquire(key string) func() {
	if key == "" {
		return func() {}
	}
	m.mu.Lock()
	if m.locks == nil {
		m.locks = map[string]*entry{}
	}
	e, ok := m.locks[key]
	if !ok {
		e = &entry{}
		m.locks[key] = e
	}
	e.ref++
	m.mu.Unlock()

	e.mu.Lock()

	released := false
	return func() {
		if released {
			return
		}
		released = true
		e.mu.Unlock()
		m.mu.Lock()
		e.ref--
		if e.ref == 0 {
			delete(m.locks, key)
		}
		m.mu.Unlock()
	}
}

// Default is a process-wide convenience instance. Most code should use
// it; tests construct their own Map.
var Default Map

// Acquire forwards to Default.Acquire.
func Acquire(key string) func() { return Default.Acquire(key) }
