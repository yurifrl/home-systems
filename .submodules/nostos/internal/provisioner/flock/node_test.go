package flock_test

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/provisioner"
	"github.com/yurifrl/nostos/internal/provisioner/flock"
)

// TestHelperProcess runs as a subprocess (env NOSTOS_FLOCK_HELPER=1).
// It acquires the lock, prints "ACQUIRED\n", then blocks until the
// parent kills it.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("NOSTOS_FLOCK_HELPER") != "1" {
		return
	}
	dir := os.Getenv("NOSTOS_FLOCK_DIR")
	name := os.Getenv("NOSTOS_FLOCK_NAME")
	rel, err := flock.AcquireNodeAt(dir, name)
	if err != nil {
		os.Stderr.WriteString("HELPER_ERR: " + err.Error() + "\n")
		os.Exit(2)
	}
	defer rel()
	os.Stdout.WriteString("ACQUIRED\n")
	_ = os.Stdout.Sync()
	// Block forever; the parent SIGKILLs us.
	select {}
}

func TestAcquireAndRelease(t *testing.T) {
	dir := t.TempDir()
	rel, err := flock.AcquireNodeAt(dir, "tp1")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tp1.lock")); err != nil {
		t.Fatalf("lockfile missing: %v", err)
	}
	rel()

	rel2, err := flock.AcquireNodeAt(dir, "tp1")
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	rel2()
}

func TestAcquireBlocksWhileChildHolds(t *testing.T) {
	dir := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run", "TestHelperProcess", "-test.v")
	cmd.Env = append(os.Environ(),
		"NOSTOS_FLOCK_HELPER=1",
		"NOSTOS_FLOCK_DIR="+dir,
		"NOSTOS_FLOCK_NAME=tp1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// Wait for child to confirm it holds the lock.
	acquired := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "ACQUIRED") {
				close(acquired)
				return
			}
		}
	}()
	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		t.Fatal("child never reported ACQUIRED")
	}

	// Parent's AcquireNode must fail with ErrLocked within 100ms.
	type result struct{ err error }
	done := make(chan result, 1)
	go func() {
		_, err := flock.AcquireNodeAt(dir, "tp1")
		done <- result{err: err}
	}()
	select {
	case r := <-done:
		if !errors.Is(r.err, provisioner.ErrLocked) {
			t.Fatalf("err=%v want ErrLocked", r.err)
		}
		if !strings.Contains(r.err.Error(), filepath.Join(dir, "tp1.lock")) {
			t.Fatalf("err message %q does not cite lockfile path", r.err.Error())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("AcquireNode did not return within 100ms (LOCK_NB should fail fast)")
	}

	// Kill child; verify lock can be re-acquired afterwards.
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()

	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("never reacquired lock after child exit")
		case <-tick.C:
			rel, err := flock.AcquireNodeAt(dir, "tp1")
			if err == nil {
				rel()
				return
			}
		}
	}
}
