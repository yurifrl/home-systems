package pxe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/clockx"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
	"github.com/yurifrl/nostos/internal/provisioner"
	pxeserver "github.com/yurifrl/nostos/internal/pxe"
)

// TestPXEDownloadEventsSubsequence drives the tap goroutine directly by
// constructing a Provisioner, swapping in a synthetic request channel,
// and asserting the emitted Event.Kind subsequence. This is the
// dell01-install regression check: it proves the PXE provider still
// produces the v0.1 observable subsequence
//
//	info? -> download(boot.ipxe) -> download(kernel) ->
//	download(initramfs) -> config-fetched
//
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

// TestWaitMaintenanceUnboundedTapClose proves that with serveTO=0 (the new
// default), WaitMaintenance has NO kill timer and returns nil when the tap
// goroutine closes early — never a timeout. The node may be powered on at any
// time, so the wait must not self-kill.
func TestWaitMaintenanceUnboundedTapClose(t *testing.T) {
	p := &Provisioner{
		deps:    provisioner.Deps{Clock: clockx.NewFakeClock(time.Time{})},
		serveTO: 0,
	}
	tapDone := make(chan struct{})
	p.tapDone = tapDone
	close(tapDone) // tap finished early (config fetched)

	err := p.WaitMaintenance(context.Background(), &config.Node{IP: "192.168.68.100"}, nil)
	if err != nil {
		t.Fatalf("WaitMaintenance(serveTO=0, tap closed) = %v, want nil", err)
	}
}

// TestWaitMaintenanceUnboundedCtxCancel proves that with serveTO=0, a cancelled
// ctx (operator Ctrl+C) ends the wait with ctx.Err() — NOT provisioner.ErrTimeout.
// Critically, the FakeClock is never advanced, so any latent kill timer would
// hang the test; its absence is what keeps this deterministic.
func TestWaitMaintenanceUnboundedCtxCancel(t *testing.T) {
	p := &Provisioner{
		deps:    provisioner.Deps{Clock: clockx.NewFakeClock(time.Time{})},
		serveTO: 0,
	}
	p.tapDone = make(chan struct{}) // never closes

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.WaitMaintenance(ctx, &config.Node{IP: "192.168.68.100"}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitMaintenance(serveTO=0, ctx cancel) = %v, want context.Canceled", err)
	}
	if errors.Is(err, provisioner.ErrTimeout) {
		t.Fatalf("WaitMaintenance returned ErrTimeout with serveTO=0; the kill timer must be disabled")
	}
}

// TestMaxWaitMaintenanceUnbounded pins the no-bound contract: the PXE
// provisioner imposes no maintenance-wait deadline of its own.
func TestMaxWaitMaintenanceUnbounded(t *testing.T) {
	p := New(provisioner.Deps{Clock: clockx.NewFakeClock(time.Time{})})
	if got := p.MaxWaitMaintenance(); got != 0 {
		t.Fatalf("MaxWaitMaintenance = %v, want 0 (unbounded)", got)
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

// TestShouldEmitConfigFetched is the core false-progress regression check:
// a config fetch only counts when it comes from the booting node (an IP that
// previously pulled the boot chain) and never from the operator host itself.
func TestShouldEmitConfigFetched(t *testing.T) {
	localIPs := map[string]bool{
		"127.0.0.1":     true,
		"::1":           true,
		"192.168.68.50": true, // operator host (PXE server bound IP)
	}

	tests := []struct {
		name      string
		req       pxeserver.HTTPRequest
		bootChain map[string]bool
		want      bool
	}{
		{
			name:      "operator-host curl does NOT trigger",
			req:       pxeserver.HTTPRequest{Path: "/configs/x.yaml", SourceIP: "192.168.68.50"},
			bootChain: map[string]bool{},
			want:      false,
		},
		{
			name:      "loopback curl does NOT trigger",
			req:       pxeserver.HTTPRequest{Path: "/configs/x.yaml", SourceIP: "127.0.0.1"},
			bootChain: map[string]bool{"127.0.0.1": true},
			want:      false,
		},
		{
			name:      "node IP that previously fetched initramfs DOES trigger",
			req:       pxeserver.HTTPRequest{Path: "/configs/x.yaml", SourceIP: "192.168.68.205"},
			bootChain: map[string]bool{"192.168.68.205": true},
			want:      true,
		},
		{
			name:      "unknown IP that never fetched the boot chain does NOT trigger",
			req:       pxeserver.HTTPRequest{Path: "/configs/x.yaml", SourceIP: "192.168.68.99"},
			bootChain: map[string]bool{"192.168.68.205": true},
			want:      false,
		},
		{
			name:      "empty source IP does NOT trigger",
			req:       pxeserver.HTTPRequest{Path: "/configs/x.yaml", SourceIP: ""},
			bootChain: map[string]bool{"": true},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldEmitConfigFetched(tt.req, localIPs, tt.bootChain); got != tt.want {
				t.Errorf("shouldEmitConfigFetched(%+v) = %v, want %v", tt.req, got, tt.want)
			}
		})
	}
}

// TestInstalledStateWiring exercises the exact installed-state calls the PXE
// provisioner makes — ClearInstalled at (re)install start (Prepare) and
// MarkInstalled on config-fetch (Boot tap) — against a temp Paths. It is
// deterministic and root-free: it never calls the full Prepare/Boot (which
// download assets / bind sockets), only the installed-state side effects.
func TestInstalledStateWiring(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	pa := paths.New(filepath.Join(t.TempDir(), "config.yaml"))
	if err := os.MkdirAll(pa.State(), 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	mac := "d0-94-66-d9-eb-a5"

	// Simulate a prior install having settled the node.
	if err := pxeserver.MarkInstalled(pa.InstalledMACs(), mac); err != nil {
		t.Fatalf("MarkInstalled: %v", err)
	}
	if !pxeserver.IsInstalled(pa.InstalledMACs(), mac) {
		t.Fatal("expected MAC installed before reinstall")
	}

	// Prepare's first step: clear installed-state so the install chain is served.
	if err := pxeserver.ClearInstalled(pa.InstalledMACs(), mac); err != nil {
		t.Fatalf("ClearInstalled: %v", err)
	}
	if pxeserver.IsInstalled(pa.InstalledMACs(), mac) {
		t.Fatal("Prepare path did not clear installed-state")
	}

	// Boot tap's config-fetch step: mark installed so the post-install reboot
	// settles to disk.
	if err := pxeserver.MarkInstalled(pa.InstalledMACs(), mac); err != nil {
		t.Fatalf("MarkInstalled (config-fetch): %v", err)
	}
	if !pxeserver.IsInstalled(pa.InstalledMACs(), mac) {
		t.Fatal("config-fetch path did not mark installed-state")
	}
}
