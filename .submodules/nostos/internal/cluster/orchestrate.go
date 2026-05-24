package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/yurifrl/nostos/internal/clockx"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/execx"
	"github.com/yurifrl/nostos/internal/paths"
	"github.com/yurifrl/nostos/internal/provisioner"
	"github.com/yurifrl/nostos/internal/provisioner/contention"
	"github.com/yurifrl/nostos/internal/provisioner/flock"
	"github.com/yurifrl/nostos/internal/provisioner/redact"
	"github.com/yurifrl/nostos/internal/registry"
)

// EventKind is the category of an install-progress Event.
type EventKind string

const (
	KindInfo          EventKind = "info"
	KindProgress      EventKind = "progress"
	KindDownload      EventKind = "download"
	KindConfigFetched EventKind = "config-fetched"
	KindNodeUp        EventKind = "node-up"
	KindApidUp        EventKind = "apid-up"
	KindBootstrapping EventKind = "bootstrapping"
	KindReady         EventKind = "ready"
	KindError         EventKind = "error"
)

// Event is a progress event from Install.
type Event struct {
	Kind    EventKind
	Message string
	Node    string
	At      time.Time
}

func newEvent(kind EventKind, msg, node string) Event {
	return Event{Kind: kind, Message: msg, Node: node, At: time.Now()}
}

// InstallOpts tunes timeouts + feature flags.
type InstallOpts struct {
	ServeTimeout            time.Duration
	BootTimeout             time.Duration
	BootstrapTimeout        time.Duration
	WaitMaintenanceDeadline time.Duration
	SkipWipe                bool
	Reinstall               bool
}

func (o *InstallOpts) withDefaults() {
	if o.ServeTimeout == 0 {
		o.ServeTimeout = 10 * time.Minute
	}
	if o.BootTimeout == 0 {
		o.BootTimeout = 10 * time.Minute
	}
	if o.BootstrapTimeout == 0 {
		o.BootstrapTimeout = 5 * time.Minute
	}
	if o.WaitMaintenanceDeadline == 0 {
		o.WaitMaintenanceDeadline = 20 * time.Minute
	}
}

// Install drives node from power-on through Ready. The flow is method
// agnostic; per-method work lives in registered Provisioners.
func Install(
	ctx context.Context,
	cfg *config.Config,
	p paths.Paths,
	node config.Node,
	nodeName string,
	opts InstallOpts,
	events chan<- Event,
) error {
	opts.withDefaults()

	clusterEmit := func(e Event) {
		select {
		case events <- e:
		case <-ctx.Done():
		}
	}
	clusterEmit(newEvent(KindInfo, "starting install of "+nodeName, nodeName))

	// Bridge from provisioner.Event into cluster.Event for the channel.
	bridge := func(ev provisioner.Event) {
		clusterEmit(Event{Kind: EventKind(ev.Kind), Message: ev.Message, Node: nodeName, At: ev.At})
	}
	scrub := redact.NewScrubber()
	emit := redact.WrapEmitter(bridge, scrub)

	// Per-node flock: cross-process safety. Held for the duration.
	unlock, err := flock.AcquireNodeAt(p.Configs(), nodeName)
	if err != nil {
		clusterEmit(newEvent(KindError, err.Error(), nodeName))
		return err
	}
	defer unlock()

	method := node.Boot.Method
	if method == "" {
		method = "pxe"
	}

	deps := provisioner.Deps{
		Cfg:   cfg,
		Paths: p,
		Cmd:   execx.OSCommander{},
		Clock: clockx.Real{},
	}
	prov, err := provisioner.New(method, deps)
	if err != nil {
		clusterEmit(newEvent(KindError, err.Error(), nodeName))
		return err
	}

	// PXE-specific knob plumb-through. Optional setters are best-effort.
	if s, ok := prov.(interface{ SetSkipWipe(bool) }); ok {
		s.SetSkipWipe(opts.SkipWipe)
	}
	if s, ok := prov.(interface{ SetServeTimeout(time.Duration) }); ok {
		s.SetServeTimeout(opts.ServeTimeout)
	}

	// Phase: preflight.
	if err := prov.Preflight(ctx, &node, emit); err != nil {
		clusterEmit(newEvent(KindError, err.Error(), nodeName))
		_ = runCleanup(prov, &node, emit)
		return err
	}

	// Live-node reinstall guard.
	if !opts.Reinstall {
		if alive := probeReady(node); alive {
			err := provisioner.ErrNodeAlreadyReady
			clusterEmit(newEvent(KindError, fmt.Sprintf("%v: %s already healthy; pass --reinstall to force", err, nodeName), nodeName))
			_ = runCleanup(prov, &node, emit)
			return err
		}
	}

	// Render machineconfig to a 0600 temp file under a 0700 per-run dir.
	configPath, cleanupTmp, err := renderToTemp(cfg, p, nodeName)
	if err != nil {
		clusterEmit(newEvent(KindError, err.Error(), nodeName))
		_ = runCleanup(prov, &node, emit)
		return err
	}
	defer cleanupTmp()

	// Cleanup (always).
	defer func() {
		_ = runCleanup(prov, &node, emit)
	}()

	// ContentionKey acquire spans Boot..Apply.
	releaseKey := contention.Acquire(prov.ContentionKey(&node))
	keyReleased := false
	releaseKeyOnce := func() {
		if !keyReleased {
			keyReleased = true
			releaseKey()
		}
	}
	defer releaseKeyOnce()

	if err := prov.Prepare(ctx, &node, emit); err != nil {
		clusterEmit(newEvent(KindError, err.Error(), nodeName))
		return err
	}
	if err := prov.Boot(ctx, &node, emit); err != nil {
		clusterEmit(newEvent(KindError, err.Error(), nodeName))
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, opts.WaitMaintenanceDeadline)
	werr := prov.WaitMaintenance(waitCtx, &node, emit)
	cancel()
	if werr != nil {
		clusterEmit(newEvent(KindError, werr.Error(), nodeName))
		return werr
	}

	if err := prov.Apply(ctx, &node, configPath, emit); err != nil {
		clusterEmit(newEvent(KindError, err.Error(), nodeName))
		return err
	}
	releaseKeyOnce()

	// Wait for apid using the existing probe.
	clusterEmit(newEvent(KindProgress, "waiting for apid", nodeName))
	apidDeadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(apidDeadline) {
		if probeApid(node) {
			clusterEmit(newEvent(KindApidUp, "apid up on "+nodeName, nodeName))
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}

	if node.Role == "controlplane" {
		clusterEmit(newEvent(KindBootstrapping, "running talosctl bootstrap on "+nodeName, nodeName))
		if err := Bootstrap(ctx, cfg, p, node, opts.BootstrapTimeout); err != nil {
			clusterEmit(newEvent(KindError, err.Error(), nodeName))
			return err
		}
		if err := FetchKubeconfig(ctx, p, node); err != nil {
			slog.Warn("kubeconfig fetch failed", "err", err)
		}
	}

	clusterEmit(newEvent(KindReady, fmt.Sprintf("%s is Ready. kubeconfig: %s", nodeName, p.Kubeconfig()), nodeName))
	return nil
}

// runCleanup invokes prov.Cleanup with a fresh 60s context derived from
// context.Background so it survives ctx cancel.
func runCleanup(prov provisioner.Provisioner, node *config.Node, emit provisioner.EventEmitter) error {
	cctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return prov.Cleanup(cctx, node, emit)
}

// renderToTemp renders the machineconfig into a 0600 file inside a 0700
// per-run secrets directory and returns its path plus a cleanup func.
func renderToTemp(cfg *config.Config, p paths.Paths, nodeName string) (string, func(), error) {
	// First render into the canonical configs/ location (existing behavior).
	canonical, err := registry.Render(cfg, p, nodeName, true)
	if err != nil {
		return "", func() {}, err
	}
	body, err := os.ReadFile(canonical)
	if err != nil {
		return "", func() {}, err
	}

	dir, err := os.MkdirTemp("", "nostos-secrets-")
	if err != nil {
		return "", func() {}, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, err
	}
	tmp := filepath.Join(dir, "machineconfig.yaml")
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", func() {}, err
	}
	cleanup := func() {
		_ = os.Remove(tmp)
		_ = os.RemoveAll(dir)
	}
	return tmp, cleanup, nil
}

// probeReady reports whether the node is currently healthy at its IP.
// Uses the existing Probe (talosctl version short).
func probeReady(node config.Node) bool {
	s := registry.Probe(node, 1500*time.Millisecond)
	return s.Apid == registry.Up && s.Version != ""
}

// probeApid is a cheap reachability check for apid.
func probeApid(node config.Node) bool {
	s := registry.Probe(node, 2*time.Second)
	return s.Apid == registry.Up
}

// errLockTimeout is unused but reserved for future refinements.
var _ = errors.New
