package contention_test

import (
	"sync"
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/provisioner/contention"
)

// Same key MUST serialize: while one holder is inside, the second cannot
// enter. Verified via barrier channels (no wall-clock dependency).
func TestSameKeySerializes(t *testing.T) {
	var m contention.Map
	const key = "tpi:host"

	enter1 := make(chan struct{})
	leave1 := make(chan struct{})
	enter2 := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		rel := m.Acquire(key)
		defer rel()
		close(enter1)
		<-leave1
	}()

	// Wait for goroutine 1 to be inside the critical section.
	<-enter1

	go func() {
		defer wg.Done()
		rel := m.Acquire(key)
		defer rel()
		close(enter2)
	}()

	// Goroutine 2 MUST NOT enter while goroutine 1 holds the lock.
	select {
	case <-enter2:
		t.Fatal("second acquirer entered before first released")
	case <-time.After(50 * time.Millisecond):
	}

	close(leave1)

	select {
	case <-enter2:
	case <-time.After(time.Second):
		t.Fatal("second acquirer never entered after release")
	}
	wg.Wait()
}

func TestDistinctKeysOverlap(t *testing.T) {
	var m contention.Map

	enterA := make(chan struct{})
	enterB := make(chan struct{})
	exitAB := make(chan struct{})

	go func() {
		rel := m.Acquire("a")
		defer rel()
		close(enterA)
		<-exitAB
	}()
	go func() {
		rel := m.Acquire("b")
		defer rel()
		close(enterB)
		<-exitAB
	}()

	select {
	case <-enterA:
	case <-time.After(time.Second):
		t.Fatal("a did not enter")
	}
	select {
	case <-enterB:
	case <-time.After(time.Second):
		t.Fatal("b did not enter — distinct keys did not overlap")
	}
	close(exitAB)
}

func TestEmptyKeyIsNoop(t *testing.T) {
	var m contention.Map
	rel := m.Acquire("")
	rel()
	rel() // double-release of empty must be safe
}

func TestReleaseIsIdempotent(t *testing.T) {
	var m contention.Map
	rel := m.Acquire("k")
	rel()
	rel() // second call is a no-op
}
