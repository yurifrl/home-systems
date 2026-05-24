// Package clockx provides a small time seam so production code can call
// Now/Sleep/NewTimer through an interface that tests can drive deterministically.
package clockx

import "time"

// Timer is a minimal subset of *time.Timer the codebase uses.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(d time.Duration) bool
}

// Clock is the production-and-test time source.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
	NewTimer(d time.Duration) Timer
}

// Real is the wall-clock implementation.
type Real struct{}

func (Real) Now() time.Time        { return time.Now() }
func (Real) Sleep(d time.Duration) { time.Sleep(d) }
func (Real) NewTimer(d time.Duration) Timer {
	return &realTimer{t: time.NewTimer(d)}
}

type realTimer struct{ t *time.Timer }

func (r *realTimer) C() <-chan time.Time     { return r.t.C }
func (r *realTimer) Stop() bool              { return r.t.Stop() }
func (r *realTimer) Reset(d time.Duration) bool { return r.t.Reset(d) }
