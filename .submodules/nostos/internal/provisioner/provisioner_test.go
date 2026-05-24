package provisioner

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRegisterAndNew(t *testing.T) {
	t.Cleanup(func() { unregister("fake") })

	called := false
	Register("fake", func(Deps) Provisioner { called = true; return nil })

	p, err := New("fake", Deps{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !called {
		t.Fatal("factory not called")
	}
	_ = p
}

func TestRegisterDuplicatePanicsWithMethodName(t *testing.T) {
	t.Cleanup(func() { unregister("dupe") })
	Register("dupe", func(Deps) Provisioner { return nil })

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, "dupe") {
			t.Fatalf("panic message %q does not mention method", msg)
		}
	}()
	Register("dupe", func(Deps) Provisioner { return nil })
}

func TestNewUnknownReturnsErrNotRegistered(t *testing.T) {
	_, err := New("nope-not-here", Deps{})
	if !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("err=%v want ErrNotRegistered", err)
	}
}

func TestErrorSentinelsRoundTrip(t *testing.T) {
	for _, sentinel := range []error{
		ErrPreflight, ErrBoot, ErrTimeout, ErrLocked,
		ErrNodeAlreadyReady, ErrNotRegistered,
	} {
		wrapped := fmt.Errorf("wrap: %w", sentinel)
		if !errors.Is(wrapped, sentinel) {
			t.Errorf("errors.Is failed for %v", sentinel)
		}
	}
}
