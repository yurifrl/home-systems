package clockx

import (
	"sort"
	"sync"
	"time"
)

// FakeClock is a deterministic Clock for tests. Time only advances when
// Advance or Set is called. Sleep blocks until the fake's clock has
// advanced by at least the requested duration.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

// NewFakeClock returns a FakeClock initialised at start. If start is
// the zero value, an arbitrary fixed reference time is chosen.
func NewFakeClock(start time.Time) *FakeClock {
	if start.IsZero() {
		start = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return &FakeClock{now: start}
}

func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Sleep blocks until the fake clock has advanced past now+d.
func (f *FakeClock) Sleep(d time.Duration) {
	t := f.NewTimer(d)
	<-t.C()
}

func (f *FakeClock) NewTimer(d time.Duration) Timer {
	f.mu.Lock()
	defer f.mu.Unlock()
	ft := &fakeTimer{
		c:        make(chan time.Time, 1),
		deadline: f.now.Add(d),
		clock:    f,
	}
	if d <= 0 {
		ft.fire(f.now)
	} else {
		f.timers = append(f.timers, ft)
	}
	return ft
}

// Advance moves the fake clock forward by d, firing any timers whose
// deadlines have been reached.
func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	now := f.now
	due := f.timers[:0]
	keep := []*fakeTimer{}
	for _, t := range f.timers {
		if !t.deadline.After(now) {
			due = append(due, t)
		} else {
			keep = append(keep, t)
		}
	}
	f.timers = keep
	f.mu.Unlock()

	sort.Slice(due, func(i, j int) bool { return due[i].deadline.Before(due[j].deadline) })
	for _, t := range due {
		t.fire(now)
	}
}

// Set jumps the clock to t (firing pending timers).
func (f *FakeClock) Set(t time.Time) {
	f.mu.Lock()
	d := t.Sub(f.now)
	f.mu.Unlock()
	if d > 0 {
		f.Advance(d)
	} else {
		f.mu.Lock()
		f.now = t
		f.mu.Unlock()
	}
}

type fakeTimer struct {
	c        chan time.Time
	deadline time.Time
	clock    *FakeClock
	mu       sync.Mutex
	fired    bool
	stopped  bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.c }

func (t *fakeTimer) fire(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fired || t.stopped {
		return
	}
	t.fired = true
	select {
	case t.c <- now:
	default:
	}
}

func (t *fakeTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fired || t.stopped {
		return false
	}
	t.stopped = true
	return true
}

func (t *fakeTimer) Reset(d time.Duration) bool {
	t.mu.Lock()
	wasActive := !t.fired && !t.stopped
	t.fired = false
	t.stopped = false
	t.deadline = t.clock.Now().Add(d)
	t.mu.Unlock()

	t.clock.mu.Lock()
	t.clock.timers = append(t.clock.timers, t)
	t.clock.mu.Unlock()
	return wasActive
}
