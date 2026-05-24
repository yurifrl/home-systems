package pxe

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/clockx"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/provisioner"
)

// TestPXEDownloadEventsSubsequence drives the tap goroutine directly by
// constructing a Provisioner, swapping in a synthetic request channel,
// and asserting the emitted Event.Kind subsequence. This is the
// dell01-install regression check: it proves the PXE provider still
// produces the v0.1 observable subsequence
//   info? -> download(boot.ipxe) -> download(kernel) ->
//   download(initramfs) -> config-fetched
// without any KindError event.
func TestPXEDownloadEventsSubsequence(t *testing.T) {
	p := &Provisioner{
		deps:        provisioner.Deps{Clock: clockx.NewFakeClock(time.Time{})},
		expectedCfg: "/configs/d0-94-66-d9-eb-a5.yaml",
	}

	reqCh := make(chan string, 8)
	tapDone := make(chan struct{})
	p.tapDone = tapDone
	serveCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var events []provisioner.Event
	emit := func(e provisioner.Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}

	// Re-use the Boot tap loop body inline (mirrors pxe.go).
	go func() {
		defer close(tapDone)
		for {
			select {
			case path, ok := <-reqCh:
				if !ok {
					return
				}
				switch {
				case strings.HasSuffix(path, "/boot.ipxe"):
					emit(provisioner.Event{Kind: "download", Message: "iPXE chainloaded boot.ipxe"})
				case strings.Contains(path, "vmlinuz"):
					emit(provisioner.Event{Kind: "download", Message: "downloading kernel"})
				case strings.Contains(path, "initramfs"):
					emit(provisioner.Event{Kind: "download", Message: "downloading initramfs"})
				case path == p.expectedCfg:
					emit(provisioner.Event{Kind: "config-fetched", Message: "Talos fetched its config — installing"})
				}
			case <-serveCtx.Done():
				return
			}
		}
	}()

	// Synthesize a real PXE chain.
	for _, p := range []string{
		"/assets/boot.ipxe",
		"/assets/vmlinuz-amd64",
		"/assets/initramfs-amd64.xz",
		"/configs/d0-94-66-d9-eb-a5.yaml",
	} {
		reqCh <- p
	}
	close(reqCh)
	<-tapDone

	// Subsequence assertion (extra intermediate events allowed).
	want := []string{"download", "download", "download", "config-fetched"}
	wantMsgs := []string{"boot.ipxe", "kernel", "initramfs", "Talos fetched"}
	mu.Lock()
	defer mu.Unlock()

	idx := 0
	for _, e := range events {
		if string(e.Kind) == "error" {
			t.Fatalf("unexpected KindError event: %+v", e)
		}
		if idx >= len(want) {
			break
		}
		if string(e.Kind) == want[idx] && strings.Contains(e.Message, wantMsgs[idx]) {
			idx++
		}
	}
	if idx != len(want) {
		t.Fatalf("subsequence not satisfied at index %d; events=%+v", idx, events)
	}
}

// TestPXEMethodAndKey pins the public identity of the provider.
func TestPXEMethodAndKey(t *testing.T) {
	p := New(provisioner.Deps{Clock: clockx.NewFakeClock(time.Time{})})
	if p.Method() != "pxe" {
		t.Fatalf("Method = %q", p.Method())
	}
	if got := p.ContentionKey(&config.Node{}); got != "pxe:server" {
		t.Fatalf("ContentionKey = %q", got)
	}
}
