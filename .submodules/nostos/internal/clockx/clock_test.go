package clockx_test

import (
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/clockx"
)

func TestFakeClockAdvancesDeterministically(t *testing.T) {
	c := clockx.NewFakeClock(time.Time{})
	start := c.Now()
	c.Advance(5 * time.Second)
	if got := c.Now().Sub(start); got != 5*time.Second {
		t.Fatalf("delta=%v", got)
	}
}

func TestFakeClockTimerFires(t *testing.T) {
	c := clockx.NewFakeClock(time.Time{})
	tm := c.NewTimer(2 * time.Second)
	select {
	case <-tm.C():
		t.Fatal("timer fired prematurely")
	default:
	}
	c.Advance(2 * time.Second)
	select {
	case <-tm.C():
	case <-time.After(time.Second):
		t.Fatal("timer did not fire after Advance")
	}
}

func TestFakeClockSleepBlocksUntilAdvance(t *testing.T) {
	c := clockx.NewFakeClock(time.Time{})
	done := make(chan struct{})
	go func() {
		c.Sleep(time.Second)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Sleep returned before Advance")
	case <-time.After(50 * time.Millisecond):
	}

	c.Advance(time.Second)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Sleep did not return after Advance")
	}
}
