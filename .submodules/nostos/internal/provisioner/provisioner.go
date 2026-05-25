// Package provisioner defines the install-method abstraction used by the
// nostos orchestrator. Each boot method (pxe, tpi, ...) ships an
// implementation in its own subpackage that registers itself in init().
package provisioner

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yurifrl/nostos/internal/clockx"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/execx"
	"github.com/yurifrl/nostos/internal/paths"
)

// Phase identifies a lifecycle stage emitted on the event channel as the
// orchestrator transitions between provisioner hooks.
type Phase string

const (
	PhasePreflight Phase = "preflight"
	PhasePrepare   Phase = "prepare"
	PhaseBoot      Phase = "boot"
	PhaseWait      Phase = "wait"
	PhaseApply     Phase = "apply"
	PhaseBootstrap Phase = "bootstrap"
	PhaseReady     Phase = "ready"
	PhaseError     Phase = "error"
	PhaseCleanup   Phase = "cleanup"
)

// Event is a single observation emitted during an install run.
type Event struct {
	Phase   Phase
	Kind    string
	Message string
	At      time.Time
}

// EventEmitter is the sink used by providers to report progress. The
// orchestrator wraps the raw events channel with a Scrubber sink before
// passing it to providers.
type EventEmitter func(Event)

// SecretsResolver is a minimal seam for resolving `op://`/`sops://`/`file://`
// references. Wave 1 only needs the type to exist so Deps compiles; later
// waves replace this with a proper interface from internal/secrets.
type SecretsResolver interface{}

// Deps is the set of seams a provisioner needs.
type Deps struct {
	Cfg     *config.Config
	Paths   paths.Paths
	Secrets SecretsResolver
	Cmd     execx.Commander
	Clock   clockx.Clock
}

// Provisioner is the contract every install method implements.
type Provisioner interface {
	Method() string
	ContentionKey(node *config.Node) string

	// MaxWaitMaintenance returns the upper-bound the orchestrator should
	// allow for the WaitMaintenance phase. Returning 0 lets the
	// orchestrator fall back to its configured default.
	MaxWaitMaintenance() time.Duration

	Preflight(ctx context.Context, node *config.Node, emit EventEmitter) error
	Prepare(ctx context.Context, node *config.Node, emit EventEmitter) error
	Boot(ctx context.Context, node *config.Node, emit EventEmitter) error
	WaitMaintenance(ctx context.Context, node *config.Node, emit EventEmitter) error
	Apply(ctx context.Context, node *config.Node, configPath string, emit EventEmitter) error
	Cleanup(ctx context.Context, node *config.Node, emit EventEmitter) error
}

// Factory builds a Provisioner given orchestrator-supplied seams.
type Factory func(deps Deps) Provisioner

var (
	regMu    sync.RWMutex
	registry = map[string]Factory{}
)

// Register adds a Factory under method. It panics if method is empty or
// already registered; the panic message contains the method name.
func Register(method string, f Factory) {
	if method == "" {
		panic("provisioner.Register: empty method")
	}
	if f == nil {
		panic(fmt.Sprintf("provisioner.Register: nil factory for method %q", method))
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, ok := registry[method]; ok {
		panic(fmt.Sprintf("provisioner.Register: duplicate method %q", method))
	}
	registry[method] = f
}

// New constructs the provisioner registered under method. Returns
// ErrNotRegistered if no factory has been registered.
func New(method string, deps Deps) (Provisioner, error) {
	regMu.RLock()
	f, ok := registry[method]
	regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provisioner %q: %w", method, ErrNotRegistered)
	}
	return f(deps), nil
}

// Methods returns the sorted list of registered method names. Intended
// for test diagnostics and CLI help.
func Methods() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(registry))
	for m := range registry {
		out = append(out, m)
	}
	return out
}

// unregister is a test-only helper for cleaning up between sub-tests.
func unregister(method string) {
	regMu.Lock()
	defer regMu.Unlock()
	delete(registry, method)
}
