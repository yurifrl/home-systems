package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
	"github.com/yurifrl/nostos/internal/pxe"
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
	ServeTimeout     time.Duration
	BootTimeout      time.Duration
	BootstrapTimeout time.Duration
	SkipWipe         bool
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
}

// Install drives node from power-on through Ready. Emits Events on `events`.
// Returns after emitting a terminal Event (Ready or Error) or ctx cancellation.
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

	emit := func(e Event) {
		select {
		case events <- e:
		case <-ctx.Done():
		}
	}

	emit(newEvent(KindInfo, fmt.Sprintf("starting install of %s", nodeName), nodeName))

	// 1. Queue wipe unless skipped.
	if !opts.SkipWipe {
		if err := QueueWipe(p.PendingWipes(), node.MAC); err != nil {
			emit(newEvent(KindError, fmt.Sprintf("queue wipe: %v", err), nodeName))
			return err
		}
		emit(newEvent(KindInfo, fmt.Sprintf("queued one-shot wipe for %s", node.MAC), nodeName))
	}

	// 2. Ensure assets + render config.
	emit(newEvent(KindProgress, "ensuring PXE assets are built", nodeName))
	if err := pxe.BuildAll(ctx, cfg, p, node.Arch); err != nil {
		emit(newEvent(KindError, fmt.Sprintf("build: %v", err), nodeName))
		return err
	}

	emit(newEvent(KindProgress, fmt.Sprintf("rendering machineconfig for %s", nodeName), nodeName))
	if _, err := registry.Render(cfg, p, nodeName, true); err != nil {
		emit(newEvent(KindError, fmt.Sprintf("render: %v", err), nodeName))
		return err
	}

	// 3. If wipe active, re-render boot.ipxe with wipe=system.
	wipeActive := !opts.SkipWipe
	if wipeActive {
		if _, err := pxe.RenderBootIpxe(cfg, p, node.Arch, "talos.experimental.wipe=system"); err != nil {
			emit(newEvent(KindError, fmt.Sprintf("render boot.ipxe with wipe: %v", err), nodeName))
			return err
		}
		emit(newEvent(KindInfo, "boot.ipxe rendered with wipe=system (one-shot)", nodeName))
	}

	// 4. Start serve.
	emit(newEvent(KindProgress, "starting PXE server (sudo for dnsmasq)", nodeName))
	srv := pxe.NewServer(p)
	ni, err := srv.Preflight()
	if err != nil {
		emit(newEvent(KindError, err.Error(), nodeName))
		return err
	}

	serveCtx, serveCancel := context.WithCancel(ctx)
	defer serveCancel()

	// Tap HTTP request channel BEFORE Start so we don't miss early GETs.
	reqCh := srv.HTTPRequests()
	if err := srv.Start(serveCtx, ni); err != nil {
		emit(newEvent(KindError, err.Error(), nodeName))
		return err
	}
	emit(newEvent(KindInfo, fmt.Sprintf("PXE server on %s (%s); power on %s", ni.Interface, ni.IP, nodeName), nodeName))

	// 5. Wait for GET /configs/<mac>.yaml.
	expectedCfg := fmt.Sprintf("/configs/%s.yaml", node.MACHyphen())
	sawConfig := false
	serveDeadline := time.After(opts.ServeTimeout)
waitLoop:
	for !sawConfig {
		select {
		case path, ok := <-reqCh:
			if !ok {
				break waitLoop
			}
			switch {
			case strings.HasSuffix(path, "/boot.ipxe"):
				emit(newEvent(KindDownload, "iPXE chainloaded boot.ipxe", nodeName))
			case strings.Contains(path, "vmlinuz"):
				emit(newEvent(KindDownload, "downloading kernel", nodeName))
			case strings.Contains(path, "initramfs"):
				emit(newEvent(KindDownload, "downloading initramfs", nodeName))
			case path == expectedCfg:
				emit(newEvent(KindConfigFetched, "Talos fetched its config — installing", nodeName))
				sawConfig = true
			}
		case <-serveDeadline:
			emit(newEvent(KindError, fmt.Sprintf("no config fetch in %s; check BIOS boot order", opts.ServeTimeout), nodeName))
			return errors.New("serve timeout before config fetched")
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// 6. Wait for node to come back at its static IP.
	emit(newEvent(KindProgress, fmt.Sprintf("waiting for %s to come up at %s", nodeName, node.IP), nodeName))
	bootDeadline := time.Now().Add(opts.BootTimeout)
	for time.Now().Before(bootDeadline) {
		s := registry.Probe(node, 2*time.Second)
		if s.Ping == registry.Up {
			emit(newEvent(KindNodeUp, fmt.Sprintf("%s is responding at %s", nodeName, node.IP), nodeName))
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}

	// 7. Wait for apid.
	emit(newEvent(KindProgress, "waiting for apid", nodeName))
	apidDeadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(apidDeadline) {
		s := registry.Probe(node, 2*time.Second)
		if s.Apid == registry.Up {
			emit(newEvent(KindApidUp, fmt.Sprintf("apid up on %s", nodeName), nodeName))
			break
		}
		time.Sleep(3 * time.Second)
	}

	// 8. Stop serve.
	srv.Stop()
	emit(newEvent(KindInfo, "stopped PXE server", nodeName))

	// 9. Consume wipe + restore boot.ipxe.
	if wipeActive {
		if _, err := ConsumeWipe(p.PendingWipes(), node.MAC); err != nil {
			slog.Warn("could not consume wipe flag", "err", err)
		}
		if _, err := pxe.RenderBootIpxe(cfg, p, node.Arch, ""); err != nil {
			slog.Warn("could not restore clean boot.ipxe", "err", err)
		} else {
			emit(newEvent(KindInfo, "cleared wipe flag; boot.ipxe back to normal", nodeName))
		}
	}

	// 10. Bootstrap if controlplane.
	if node.Role == "controlplane" {
		if _, err := exec.LookPath("talosctl"); err == nil {
			emit(newEvent(KindBootstrapping, fmt.Sprintf("running talosctl bootstrap on %s", nodeName), nodeName))
			if err := Bootstrap(ctx, cfg, p, node, opts.BootstrapTimeout); err != nil {
				emit(newEvent(KindError, err.Error(), nodeName))
				return err
			}
			if err := FetchKubeconfig(ctx, p, node); err != nil {
				slog.Warn("kubeconfig fetch failed", "err", err)
			}
		}
	}

	emit(newEvent(KindReady, fmt.Sprintf("%s is Ready. kubeconfig: %s", nodeName, p.Kubeconfig()), nodeName))
	return nil
}
