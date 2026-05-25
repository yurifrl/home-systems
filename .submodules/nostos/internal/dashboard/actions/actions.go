// Package actions defines the mutating-action dispatch seam used by the
// dashboard TUI to implement i/r/d/G keybindings.
//
// The TUI never shells out directly: it always goes through Dispatcher.
// Tests inject FakeDispatcher; the cmux smoke test uses NoopDispatcher
// (or sets NOSTOS_DASHBOARD_DISPATCH_DRY_RUN=1) to avoid touching a real
// cluster.
package actions

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
	"github.com/yurifrl/nostos/internal/execx"
)

// Kind is the high-level action type bound to a TUI keypress.
type Kind string

const (
	KindIdentify  Kind = "identify"
	KindReinstall Kind = "reinstall"
	KindDelete    Kind = "delete"
	KindGoFix     Kind = "gofix"
)

// Target describes the row the action applies to. Optional fields are zero
// when not relevant. Filled by the TUI from the cursor row + config.
type Target struct {
	Name      string
	IP        string
	Bucket    string // known | orphan | unknown
	BootMethod string // pxe | tpi | ""
	TPIHost   string
	TPISlot   int
	IsOrphan  bool   // configured-but-unreachable or stale TS device
	Severity  snapshot.Severity
}

// Result is the outcome of a dispatch.
type Result struct {
	// Argv is the command + args that were (or would have been) executed.
	Argv     []string
	Stdout   string
	Stderr   string
	ExitCode int
	// DryRun is true when the dispatcher elected not to actually execute
	// (NOSTOS_DASHBOARD_DISPATCH_DRY_RUN=1 or NoopDispatcher).
	DryRun bool
}

// Dispatcher is the mutating-action seam.
type Dispatcher interface {
	Identify(ctx context.Context, t Target) (Result, error)
	Reinstall(ctx context.Context, t Target) (Result, error)
	Delete(ctx context.Context, t Target) (Result, error)
	GoFix(ctx context.Context, snap snapshot.Snapshot) (Result, error)
}

// ----------------------------------------------------------------------------
// Real dispatcher
// ----------------------------------------------------------------------------

// ExecDispatcher shells out via an execx.Commander.
type ExecDispatcher struct {
	Cmd    execx.Commander
	DryRun bool // when true, return Argv without running
}

// New returns an ExecDispatcher honoring NOSTOS_DASHBOARD_DISPATCH_DRY_RUN.
func New(cmd execx.Commander) *ExecDispatcher {
	dry := os.Getenv("NOSTOS_DASHBOARD_DISPATCH_DRY_RUN") == "1"
	return &ExecDispatcher{Cmd: cmd, DryRun: dry}
}

func (d *ExecDispatcher) run(ctx context.Context, argv []string) (Result, error) {
	r := Result{Argv: argv, DryRun: d.DryRun}
	if d.DryRun || d.Cmd == nil {
		r.DryRun = true
		return r, nil
	}
	var out, errb bytes.Buffer
	err := d.Cmd.Run(ctx, argv[0], argv[1:], nil, nil, &out, &errb)
	r.Stdout = out.String()
	r.Stderr = errb.String()
	if err != nil {
		r.ExitCode = 1
	}
	return r, err
}

// Identify maps to: tpi → `tpi power reset -n <slot>`; pxe → `talosctl reboot`;
// unknown bucket → error written to stderr.
func (d *ExecDispatcher) Identify(ctx context.Context, t Target) (Result, error) {
	switch t.BootMethod {
	case "tpi":
		if t.TPISlot < 1 || t.TPISlot > 4 {
			return Result{}, fmt.Errorf("identify: invalid tpi slot %d", t.TPISlot)
		}
		argv := []string{"tpi", "power", "reset", "-n", strconv.Itoa(t.TPISlot)}
		if t.TPIHost != "" {
			argv = append(argv, "--host", t.TPIHost)
		}
		return d.run(ctx, argv)
	case "pxe":
		if t.IP == "" {
			return Result{}, fmt.Errorf("identify: pxe target missing IP")
		}
		argv := []string{"talosctl", "-n", t.IP, "reboot"}
		return d.run(ctx, argv)
	default:
		return Result{}, fmt.Errorf("identify: no boot method known for %q (pick a configured node)", t.Name)
	}
}

// Reinstall spawns `nostos node install <name> --reinstall --yes`.
func (d *ExecDispatcher) Reinstall(ctx context.Context, t Target) (Result, error) {
	if t.Name == "" {
		return Result{}, fmt.Errorf("reinstall: missing node name")
	}
	argv := []string{"nostos", "node", "install", t.Name, "--reinstall", "--yes"}
	return d.run(ctx, argv)
}

// Delete dispatches `nostos cluster cleanup --target <name|ip>`.
// Allowed only for orphan k8s nodes / Tailscale stale devices.
func (d *ExecDispatcher) Delete(ctx context.Context, t Target) (Result, error) {
	if !t.IsOrphan && t.Bucket != "orphan" {
		return Result{}, fmt.Errorf("delete: only allowed on orphan rows")
	}
	tgt := t.Name
	if tgt == "" {
		tgt = t.IP
	}
	if tgt == "" {
		return Result{}, fmt.Errorf("delete: target missing name and ip")
	}
	argv := []string{"nostos", "cluster", "cleanup", "--target", tgt, "--yes"}
	return d.run(ctx, argv)
}

// GoFix picks the highest-severity remediable check and dispatches the
// matching subcommand. When nothing is remediable, returns ErrNothingToFix.
func (d *ExecDispatcher) GoFix(ctx context.Context, snap snapshot.Snapshot) (Result, error) {
	pick, ok := PickRemediable(snap.Checks)
	if !ok {
		return Result{}, ErrNothingToFix
	}
	argv := remediationArgv(pick)
	if len(argv) == 0 {
		return Result{}, ErrNothingToFix
	}
	return d.run(ctx, argv)
}

// ErrNothingToFix is returned by GoFix when no check is remediable.
var ErrNothingToFix = fmt.Errorf("nothing to auto-fix; press s for setup-info on selected row")

// PickRemediable returns the worst-severity check that has a known
// remediation mapping. Exposed for tests + TUI hint rendering.
func PickRemediable(checks []snapshot.Check) (snapshot.Check, bool) {
	rank := func(s snapshot.Severity) int {
		switch s {
		case snapshot.SevFail:
			return 3
		case snapshot.SevWarn:
			return 2
		case snapshot.SevUnknown:
			return 1
		}
		return 0
	}
	var best snapshot.Check
	found := false
	for _, c := range checks {
		if len(remediationArgv(c)) == 0 {
			continue
		}
		if !found || rank(c.Severity) > rank(best.Severity) {
			best = c
			found = true
		}
	}
	return best, found
}

// remediationArgv maps a check.ID to its auto-fix argv. Empty slice = no fix.
func remediationArgv(c snapshot.Check) []string {
	switch c.ID {
	case "upstream.talos":
		return []string{"nostos", "cluster", "upgrade", "--dry-run"}
	case "cluster.argocd.apps":
		return []string{"nostos", "cluster", "sync", "--yes"}
	case "node.talos.apid":
		return []string{"nostos", "cluster", "status"}
	}
	return nil
}

// ----------------------------------------------------------------------------
// Fakes / mocks
// ----------------------------------------------------------------------------

// FakeDispatcher records intended invocations for tests.
type FakeDispatcher struct {
	mu    sync.Mutex
	Calls []FakeCall
	// Result/Err override per kind (zero values mean: synthesize default).
	Override map[Kind]struct {
		R   Result
		Err error
	}
}

// FakeCall is a single recorded dispatch.
type FakeCall struct {
	Kind   Kind
	Target Target
	Argv   []string
}

func (f *FakeDispatcher) record(k Kind, t Target, argv []string) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, FakeCall{Kind: k, Target: t, Argv: argv})
	if f.Override != nil {
		if o, ok := f.Override[k]; ok {
			if o.R.Argv == nil {
				o.R.Argv = argv
			}
			return o.R, o.Err
		}
	}
	return Result{Argv: argv, DryRun: true}, nil
}

// Identify implements Dispatcher.
func (f *FakeDispatcher) Identify(ctx context.Context, t Target) (Result, error) {
	d := &ExecDispatcher{DryRun: true}
	r, err := d.Identify(ctx, t)
	if err != nil {
		return f.recordWithErr(KindIdentify, t, r.Argv, err)
	}
	return f.record(KindIdentify, t, r.Argv)
}

// Reinstall implements Dispatcher.
func (f *FakeDispatcher) Reinstall(ctx context.Context, t Target) (Result, error) {
	d := &ExecDispatcher{DryRun: true}
	r, err := d.Reinstall(ctx, t)
	if err != nil {
		return f.recordWithErr(KindReinstall, t, r.Argv, err)
	}
	return f.record(KindReinstall, t, r.Argv)
}

// Delete implements Dispatcher.
func (f *FakeDispatcher) Delete(ctx context.Context, t Target) (Result, error) {
	d := &ExecDispatcher{DryRun: true}
	r, err := d.Delete(ctx, t)
	if err != nil {
		return f.recordWithErr(KindDelete, t, r.Argv, err)
	}
	return f.record(KindDelete, t, r.Argv)
}

// GoFix implements Dispatcher.
func (f *FakeDispatcher) GoFix(ctx context.Context, snap snapshot.Snapshot) (Result, error) {
	d := &ExecDispatcher{DryRun: true}
	r, err := d.GoFix(ctx, snap)
	if err != nil {
		return f.recordWithErr(KindGoFix, Target{}, r.Argv, err)
	}
	return f.record(KindGoFix, Target{}, r.Argv)
}

func (f *FakeDispatcher) recordWithErr(k Kind, t Target, argv []string, err error) (Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, FakeCall{Kind: k, Target: t, Argv: argv})
	return Result{Argv: argv, DryRun: true}, err
}

// NoopDispatcher records nothing and always returns dry-run results. Used by
// the cmux smoke test and `--dispatch=mock` flag.
type NoopDispatcher struct{}

// Identify implements Dispatcher.
func (NoopDispatcher) Identify(ctx context.Context, t Target) (Result, error) {
	d := &ExecDispatcher{DryRun: true}
	return d.Identify(ctx, t)
}

// Reinstall implements Dispatcher.
func (NoopDispatcher) Reinstall(ctx context.Context, t Target) (Result, error) {
	d := &ExecDispatcher{DryRun: true}
	return d.Reinstall(ctx, t)
}

// Delete implements Dispatcher.
func (NoopDispatcher) Delete(ctx context.Context, t Target) (Result, error) {
	d := &ExecDispatcher{DryRun: true}
	return d.Delete(ctx, t)
}

// GoFix implements Dispatcher.
func (NoopDispatcher) GoFix(ctx context.Context, s snapshot.Snapshot) (Result, error) {
	d := &ExecDispatcher{DryRun: true}
	return d.GoFix(ctx, s)
}
