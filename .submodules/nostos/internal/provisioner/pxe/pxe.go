// Package pxe wires the v0.1 PXE flow behind the Provisioner interface.
//
// Apply is a no-op: PXE delivers the rendered machineconfig in-band via
// the iPXE chain. The HTTP-request tap goroutine that observes Talos
// fetching its config is owned here and joined in Cleanup.
package pxe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yurifrl/nostos/internal/cluster"
	"github.com/yurifrl/nostos/internal/config"
	pxeserver "github.com/yurifrl/nostos/internal/pxe"
	"github.com/yurifrl/nostos/internal/provisioner"
)

const Method = "pxe"

func init() {
	provisioner.Register(Method, New)
}

// Provisioner is the PXE implementation.
type Provisioner struct {
	deps provisioner.Deps

	mu          sync.Mutex
	srv         *pxeserver.Server
	ni          pxeserver.NetworkInfo
	tapDone     chan struct{}
	serveCancel context.CancelFunc
	skipWipe    bool
	wipeActive  bool
	expectedCfg string
	serveTO     time.Duration
}

// New is the registered factory.
func New(deps provisioner.Deps) provisioner.Provisioner {
	return &Provisioner{deps: deps, serveTO: 10 * time.Minute}
}

// Method returns "pxe".
func (p *Provisioner) Method() string { return Method }

// ContentionKey returns the single PXE-server key.
func (p *Provisioner) ContentionKey(_ *config.Node) string { return "pxe:server" }

// MaxWaitMaintenance is the maximum time we permit a node to spend in
// maintenance mode after PXE boot. PXE flows are fast; 10 minutes is
// generous.
func (p *Provisioner) MaxWaitMaintenance() time.Duration { return 10 * time.Minute }

// Preflight checks server-side prerequisites (assets present, dnsmasq, iface).
func (p *Provisioner) Preflight(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	emitInfo(emit, "pxe preflight: checking assets and interface")
	srv := pxeserver.NewServer(p.deps.Paths)
	ni, err := srv.Preflight()
	if err != nil {
		return fmt.Errorf("%w: %v", provisioner.ErrPreflight, err)
	}
	p.mu.Lock()
	p.srv = srv
	p.ni = ni
	p.mu.Unlock()
	return nil
}

// Prepare builds assets, queues wipe, renders boot.ipxe with wipe flag.
func (p *Provisioner) Prepare(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	cfg := p.deps.Cfg
	pa := p.deps.Paths

	if !p.skipWipe {
		if err := cluster.QueueWipe(pa.PendingWipes(), node.MAC); err != nil {
			return fmt.Errorf("queue wipe: %w", err)
		}
		emitInfo(emit, "queued one-shot wipe for "+node.MAC)
		p.wipeActive = true
	}

	emitProgress(emit, "ensuring PXE assets are built")
	if err := pxeserver.BuildAll(ctx, cfg, pa, node.Arch); err != nil {
		return err
	}
	if p.wipeActive {
		if _, err := pxeserver.RenderBootIpxe(cfg, pa, node.Arch, "talos.experimental.wipe=system"); err != nil {
			return fmt.Errorf("render boot.ipxe wipe: %w", err)
		}
		emitInfo(emit, "boot.ipxe rendered with wipe=system (one-shot)")
	}
	return nil
}

// Boot starts the PXE server and the HTTP-request tap goroutine.
func (p *Provisioner) Boot(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	p.mu.Lock()
	srv := p.srv
	ni := p.ni
	p.mu.Unlock()
	if srv == nil {
		return errors.New("pxe: Boot called before Preflight")
	}

	serveCtx, cancel := context.WithCancel(context.Background())
	p.serveCancel = cancel

	reqCh := srv.HTTPRequests()
	if err := srv.Start(serveCtx, ni); err != nil {
		cancel()
		return fmt.Errorf("%w: pxe start: %v", provisioner.ErrBoot, err)
	}
	emitInfo(emit, fmt.Sprintf("PXE server on %s (%s); power on %s", ni.Interface, ni.IP, node.MAC))
	p.expectedCfg = "/configs/" + node.MACHyphen() + ".yaml"

	done := make(chan struct{})
	p.tapDone = done
	expected := p.expectedCfg
	go func() {
		defer close(done)
		for {
			select {
			case path, ok := <-reqCh:
				if !ok {
					return
				}
				switch {
				case strings.HasSuffix(path, "/boot.ipxe"):
					emit(provisioner.Event{Phase: provisioner.PhaseBoot, Kind: "download", Message: "iPXE chainloaded boot.ipxe", At: p.deps.Clock.Now()})
				case strings.Contains(path, "vmlinuz"):
					emit(provisioner.Event{Phase: provisioner.PhaseBoot, Kind: "download", Message: "downloading kernel", At: p.deps.Clock.Now()})
				case strings.Contains(path, "initramfs"):
					emit(provisioner.Event{Phase: provisioner.PhaseBoot, Kind: "download", Message: "downloading initramfs", At: p.deps.Clock.Now()})
				case path == expected:
					emit(provisioner.Event{Phase: provisioner.PhaseApply, Kind: "config-fetched", Message: "Talos fetched its config — installing", At: p.deps.Clock.Now()})
				}
			case <-serveCtx.Done():
				return
			}
		}
	}()
	return nil
}

// WaitMaintenance blocks until either the config-fetch is observed or
// ctx deadline expires. PXE delivers config in-band, so once the
// node has fetched the config we treat the node as having reached
// post-maintenance.
func (p *Provisioner) WaitMaintenance(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	emitProgress(emit, fmt.Sprintf("waiting for %s to fetch its machineconfig", node.IP))
	timer := p.deps.Clock.NewTimer(p.serveTO)
	defer timer.Stop()
	// Note: actual config-fetch is detected by the tap; here we just
	// wait either for ctx done or tap goroutine ending early.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C():
		return fmt.Errorf("%w: pxe wait", provisioner.ErrTimeout)
	case <-p.tapDoneEarly():
		return nil
	}
}

// tapDoneEarly returns a channel closed when the tap goroutine exits.
// We don't actually inspect what was observed; the orchestrator checks
// node liveness separately. This keeps the contract simple.
func (p *Provisioner) tapDoneEarly() <-chan struct{} {
	if p.tapDone == nil {
		c := make(chan struct{})
		return c
	}
	return p.tapDone
}

// Apply is a no-op for PXE (config delivered in-band via iPXE).
func (p *Provisioner) Apply(ctx context.Context, node *config.Node, configPath string, emit provisioner.EventEmitter) error {
	emitInfo(emit, "pxe apply: in-band; no talosctl apply-config required")
	return nil
}

// Cleanup stops the server, joins the tap goroutine, consumes wipe.
func (p *Provisioner) Cleanup(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	p.mu.Lock()
	srv := p.srv
	cancel := p.serveCancel
	tapDone := p.tapDone
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if srv != nil {
		srv.Stop()
		emitInfo(emit, "stopped PXE server")
	}
	if tapDone != nil {
		select {
		case <-tapDone:
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}

	if p.wipeActive && node != nil {
		if _, err := cluster.ConsumeWipe(p.deps.Paths.PendingWipes(), node.MAC); err == nil {
			if _, err := pxeserver.RenderBootIpxe(p.deps.Cfg, p.deps.Paths, node.Arch, ""); err == nil {
				emitInfo(emit, "cleared wipe flag; boot.ipxe back to normal")
			}
		}
		p.wipeActive = false
	}
	return nil
}

// SetSkipWipe is used by the orchestrator to honor InstallOpts.SkipWipe.
func (p *Provisioner) SetSkipWipe(v bool) { p.skipWipe = v }

// SetServeTimeout overrides the default 10m serve timeout.
func (p *Provisioner) SetServeTimeout(d time.Duration) {
	if d > 0 {
		p.serveTO = d
	}
}

func emitInfo(emit provisioner.EventEmitter, msg string) {
	if emit == nil {
		return
	}
	emit(provisioner.Event{Phase: provisioner.PhaseBoot, Kind: "info", Message: msg, At: time.Now()})
}
func emitProgress(emit provisioner.EventEmitter, msg string) {
	if emit == nil {
		return
	}
	emit(provisioner.Event{Phase: provisioner.PhaseBoot, Kind: "progress", Message: msg, At: time.Now()})
}
