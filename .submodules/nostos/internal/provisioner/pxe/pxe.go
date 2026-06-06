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
	"github.com/yurifrl/nostos/internal/provisioner"
	pxeserver "github.com/yurifrl/nostos/internal/pxe"
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

// New is the registered factory. serveTO defaults to 0 (no timeout): the
// install flow must wait until the node fetches its config or the operator
// Ctrl+C's, because the node may be powered on at any time. An operator can
// opt into a bound via SetServeTimeout.
func New(deps provisioner.Deps) provisioner.Provisioner {
	return &Provisioner{deps: deps, serveTO: 0}
}

// Method returns "pxe".
func (p *Provisioner) Method() string { return Method }

// ContentionKey returns the single PXE-server key.
func (p *Provisioner) ContentionKey(_ *config.Node) string { return "pxe:server" }

// MaxWaitMaintenance returns 0: PXE imposes NO provisioner-side bound on the
// maintenance/config-fetch wait. The node can be powered on whenever, so the
// wait ends only on config-fetch (tap) or operator Ctrl+C (ctx cancel). An
// operator can still opt into a bound via the install ServeTimeout knob.
func (p *Provisioner) MaxWaitMaintenance() time.Duration { return 0 }

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

	// A (re)install must always serve the install chain, not the
	// boot-from-disk script: clear any prior installed-state for this MAC
	// regardless of skipWipe. Best-effort — never block the install on it.
	if err := pxeserver.ClearInstalled(pa.InstalledMACs(), node.MAC); err != nil {
		emitInfo(emit, "warning: clear installed-state: "+err.Error())
	}

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

	// Snapshot the operator-host/local IPs so the tap can reject a
	// config-fetch that originates from this machine (e.g. a hand-rolled
	// curl) instead of the booting node.
	localIPs := make(map[string]bool)
	for _, ip := range srv.LocalIPs() {
		localIPs[ip] = true
	}

	done := make(chan struct{})
	p.tapDone = done
	expected := p.expectedCfg
	go func() {
		defer close(done)
		// bootChainIPs tracks source IPs that fetched the boot chain
		// (boot.ipxe / vmlinuz / initramfs). A booting node always fetches
		// the kernel+initramfs before it can fetch its config, so a
		// config-fetch is only trusted from an IP seen here. The goroutine
		// is single-threaded over reqCh, so a plain map needs no locking.
		bootChainIPs := make(map[string]bool)
		for {
			select {
			case req, ok := <-reqCh:
				if !ok {
					return
				}
				path := req.Path
				switch {
				case strings.HasSuffix(path, "/boot.ipxe"):
					bootChainIPs[req.SourceIP] = true
					emit(provisioner.Event{Phase: provisioner.PhaseBoot, Kind: "download", Message: "iPXE chainloaded boot.ipxe", At: p.deps.Clock.Now()})
				case strings.Contains(path, "vmlinuz"):
					bootChainIPs[req.SourceIP] = true
					emit(provisioner.Event{Phase: provisioner.PhaseBoot, Kind: "download", Message: "downloading kernel", At: p.deps.Clock.Now()})
				case strings.Contains(path, "initramfs"):
					bootChainIPs[req.SourceIP] = true
					emit(provisioner.Event{Phase: provisioner.PhaseBoot, Kind: "download", Message: "downloading initramfs", At: p.deps.Clock.Now()})
				case path == expected:
					if shouldEmitConfigFetched(req, localIPs, bootChainIPs) {
						// The node fetched its config -> it is committing to
						// install to disk. Mark installed so the post-install
						// reboot is served the boot-from-disk script and settles.
						// Best-effort: log on error, never fail the install.
						if err := pxeserver.MarkInstalled(p.deps.Paths.InstalledMACs(), node.MAC); err != nil {
							emit(provisioner.Event{Phase: provisioner.PhaseApply, Kind: "info", Message: "warning: mark installed-state: " + err.Error(), At: p.deps.Clock.Now()})
						}
						emit(provisioner.Event{Phase: provisioner.PhaseApply, Kind: "config-fetched", Message: "Talos fetched its config — installing", At: p.deps.Clock.Now()})
					}
				}
			case <-serveCtx.Done():
				return
			}
		}
	}()
	return nil
}

// shouldEmitConfigFetched decides whether a request for the expected config
// path is a genuine node-side fetch worth reporting as "installing".
//
// The config-fetched signal is the core of the false-progress bug: any HTTP
// hit to the config path used to trip it, including a curl run from the
// operator host. Two conditions must hold to accept it:
//
//   - The source is NOT a local/operator-host IP (loopback or any IP the PXE
//     server is bound to). A curl from this machine is excluded.
//   - The source previously fetched the boot chain (boot.ipxe / vmlinuz /
//     initramfs). A booting node always pulls kernel+initramfs before it can
//     fetch its config, so this positively identifies the real node. The
//     node's DHCP-assigned (proxy-mode) IP is generally NOT node.IP, so we do
//     not match against the final static IP.
func shouldEmitConfigFetched(req pxeserver.HTTPRequest, localIPs, bootChainIPs map[string]bool) bool {
	if req.SourceIP == "" {
		return false
	}
	if localIPs[req.SourceIP] || req.SourceIP == "127.0.0.1" || req.SourceIP == "::1" {
		return false
	}
	return bootChainIPs[req.SourceIP]
}

// WaitMaintenance blocks until either the config-fetch is observed or
// ctx deadline expires. PXE delivers config in-band, so once the
// node has fetched the config we treat the node as having reached
// post-maintenance.
//
// When serveTO <= 0 (the default) there is NO kill timer: the wait is
// unbounded and ends only on ctx cancel (operator Ctrl+C / parent deadline)
// or the tap goroutine signaling an early finish. When serveTO > 0 the
// operator opted into a bound, so we arm the kill timer.
func (p *Provisioner) WaitMaintenance(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	emitProgress(emit, fmt.Sprintf("waiting for %s to fetch its machineconfig", node.IP))
	// Note: actual config-fetch is detected by the tap; here we just
	// wait either for ctx done or tap goroutine ending early.
	if p.serveTO <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.tapDoneEarly():
			return nil
		}
	}
	timer := p.deps.Clock.NewTimer(p.serveTO)
	defer timer.Stop()
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

// SetServeTimeout overrides the default (0 = no timeout) serve bound. Only a
// positive duration arms the kill timer in WaitMaintenance.
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
