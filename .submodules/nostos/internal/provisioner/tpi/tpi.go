// Package tpi implements the Turing Pi BMC install method.
package tpi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yurifrl/nostos/internal/clockx"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/execx"
	"github.com/yurifrl/nostos/internal/provisioner"
	"github.com/yurifrl/nostos/internal/secrets"
)

const (
	Method     = "tpi"
	MinVersion = "1.0.0"
)

func init() {
	provisioner.Register(Method, New)
}

// Provisioner implements the tpi method.
type Provisioner struct {
	deps    provisioner.Deps
	runID   string
	mu      sync.Mutex
	keyPath string
	secDir  string
	imgPath string
	user    string
	pass    string
	bootErr error
}

// New is the registered factory.
func New(deps provisioner.Deps) provisioner.Provisioner {
	return &Provisioner{
		deps:  deps,
		runID: fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid()),
	}
}

// Method returns "tpi".
func (p *Provisioner) Method() string { return Method }

// ContentionKey returns "tpi:<host>".
func (p *Provisioner) ContentionKey(node *config.Node) string {
	if node == nil || node.Boot.TPI == nil {
		return ""
	}
	return "tpi:" + node.Boot.TPI.Host
}

// dialer seam (var so tests can substitute).
var dialTimeout = func(network, addr string, d time.Duration) (net.Conn, error) {
	return net.DialTimeout(network, addr, d)
}

// Preflight: tpi --version >= MinVersion, TCP host:443, refs resolve,
// cache root has free space (best-effort).
func (p *Provisioner) Preflight(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	if node == nil || node.Boot.TPI == nil {
		return fmt.Errorf("%w: tpi: missing boot.tpi block", provisioner.ErrPreflight)
	}
	tpi := node.Boot.TPI

	// tpi --version
	var stdout bytes.Buffer
	if err := p.run(ctx, "tpi", []string{"--version"}, nil, nil, &stdout, io.Discard); err != nil {
		return fmt.Errorf("%w: tpi --version: %v", provisioner.ErrPreflight, err)
	}
	if v := parseVersion(stdout.String()); !versionAtLeast(v, MinVersion) {
		return fmt.Errorf("%w: tpi version %q < required %s", provisioner.ErrPreflight, v, MinVersion)
	}

	// TCP probe host:443
	conn, err := dialTimeout("tcp", net.JoinHostPort(tpi.Host, "443"), 2*time.Second)
	if err != nil {
		return fmt.Errorf("%w: tpi BMC %s:443: %v", provisioner.ErrPreflight, tpi.Host, err)
	}
	_ = conn.Close()

	// Resolve refs (read-only here; values cached on Boot).
	if err := p.resolveRefs(node); err != nil {
		return fmt.Errorf("%w: %v", provisioner.ErrPreflight, err)
	}

	hasCreds := tpi.IdentityFileRef != "" || tpi.UsernameRef != "" || tpi.PasswordRef != ""
	if !hasCreds {
		emit(provisioner.Event{
			Phase:   provisioner.PhasePreflight,
			Kind:    "info",
			Message: "tpi auth: cached token / interactive (no creds in config)",
			At:      p.deps.Clock.Now(),
		})
	}

	// Image digest pin is recommended; absence falls back to TOFU.
	schematic := node.EffectiveSchematic(p.deps.Cfg.Cluster)
	key := imageDigestKey(schematic, p.deps.Cfg.Cluster.TalosVersion, node.Arch)
	if _, ok := p.deps.Cfg.Cluster.ImageDigests[key]; !ok {
		emit(provisioner.Event{
			Phase:   provisioner.PhasePreflight,
			Kind:    "warn",
			Message: fmt.Sprintf("no image_digest pinned for %s; trusting TLS to factory.talos.dev (TOFU)", key),
			At:      p.deps.Clock.Now(),
		})
	}

	emit(provisioner.Event{Phase: provisioner.PhasePreflight, Kind: "info", Message: "tpi preflight ok", At: p.deps.Clock.Now()})
	return nil
}

// Prepare: download (if needed), verify sha256, decompress xz to .raw.
func (p *Provisioner) Prepare(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	if node == nil {
		return errors.New("tpi: nil node")
	}
	cfg := p.deps.Cfg
	schematic := node.EffectiveSchematic(cfg.Cluster)
	key := imageDigestKey(schematic, cfg.Cluster.TalosVersion, node.Arch)
	cache := imageCache{
		schematic:   schematic,
		version:     cfg.Cluster.TalosVersion,
		arch:        node.Arch,
		pinned:      cfg.Cluster.ImageDigests[key],
		digestStore: filepath.Join(p.deps.Paths.Cache(), "digests.json"),
		warn: func(s string) {
			emit(provisioner.Event{Phase: provisioner.PhasePrepare, Kind: "warn", Message: s, At: p.deps.Clock.Now()})
		},
	}
	emit(provisioner.Event{Phase: provisioner.PhasePrepare, Kind: "progress", Message: "fetching Talos image", At: p.deps.Clock.Now()})
	rawPath, err := cache.Ensure(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.imgPath = rawPath
	p.mu.Unlock()
	emit(provisioner.Event{Phase: provisioner.PhasePrepare, Kind: "info", Message: "image ready: " + rawPath, At: p.deps.Clock.Now()})
	return nil
}

// Boot: power off (non-fatal "already off"), flash, power on. Streams
// stdout to emit through a 200ms coalescing window.
func (p *Provisioner) Boot(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	tpi := node.Boot.TPI
	host := tpi.Host
	slot := strconv.Itoa(tpi.Slot)

	if err := p.materializeKey(node); err != nil {
		p.bootErr = err
		return fmt.Errorf("%w: %v", provisioner.ErrBoot, err)
	}

	env := os.Environ()
	if p.user != "" {
		env = append(env, "TPI_USERNAME="+p.user)
	}
	if p.pass != "" {
		env = append(env, "TPI_PASSWORD="+p.pass)
	}

	// power off (best-effort)
	var poStderr bytes.Buffer
	poArgs := []string{"--host", host, "power", "off", "-n", slot}
	if err := p.runStream(ctx, "tpi", poArgs, env, nil, emit, &poStderr); err != nil {
		if !strings.Contains(strings.ToLower(poStderr.String()), "already off") {
			p.bootErr = err
			return fmt.Errorf("%w: tpi power off: %v", provisioner.ErrBoot, err)
		}
	}

	// flash
	flashArgs := []string{"--host", host, "flash", "-i", p.imgPath, "-n", slot}
	if err := p.runStream(ctx, "tpi", flashArgs, env, nil, emit, io.Discard); err != nil {
		p.bootErr = err
		return fmt.Errorf("%w: tpi flash: %v", provisioner.ErrBoot, err)
	}

	// power on
	onArgs := []string{"--host", host, "power", "on", "-n", slot}
	if err := p.runStream(ctx, "tpi", onArgs, env, nil, emit, io.Discard); err != nil {
		p.bootErr = err
		return fmt.Errorf("%w: tpi power on: %v", provisioner.ErrBoot, err)
	}
	return nil
}

// WaitMaintenance polls talosctl --insecure -n <ip> version every 5s.
func (p *Provisioner) WaitMaintenance(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	const interval = 5 * time.Second
	for {
		var stdout bytes.Buffer
		err := p.run(ctx, "talosctl", []string{"--insecure", "-n", node.IP, "version"}, nil, nil, &stdout, io.Discard)
		if err == nil && parseVersion(stdout.String()) != "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: tpi wait maintenance: %v", provisioner.ErrTimeout, ctx.Err())
		case <-p.deps.Clock.NewTimer(interval).C():
		}
	}
}

// Apply runs talosctl apply-config -i with the rendered machineconfig.
func (p *Provisioner) Apply(ctx context.Context, node *config.Node, configPath string, emit provisioner.EventEmitter) error {
	args := []string{"apply-config", "-i", "-n", node.IP, "--file", configPath}
	if err := p.run(ctx, "talosctl", args, nil, nil, io.Discard, io.Discard); err != nil {
		return fmt.Errorf("talosctl apply-config: %w", err)
	}
	return nil
}

// Cleanup: best-effort power off on prior error, always unlink secrets.
func (p *Provisioner) Cleanup(ctx context.Context, node *config.Node, emit provisioner.EventEmitter) error {
	p.mu.Lock()
	keyPath := p.keyPath
	secDir := p.secDir
	bootErr := p.bootErr
	p.mu.Unlock()

	if bootErr != nil && node != nil && node.Boot.TPI != nil {
		host := node.Boot.TPI.Host
		slot := strconv.Itoa(node.Boot.TPI.Slot)
		env := os.Environ()
		if p.user != "" {
			env = append(env, "TPI_USERNAME="+p.user)
		}
		if p.pass != "" {
			env = append(env, "TPI_PASSWORD="+p.pass)
		}
		_ = p.run(ctx, "tpi", []string{"--host", host, "power", "off", "-n", slot}, env, nil, io.Discard, io.Discard)
	}
	if keyPath != "" {
		_ = os.Remove(keyPath)
	}
	if secDir != "" {
		_ = os.RemoveAll(secDir)
	}
	p.mu.Lock()
	p.keyPath = ""
	p.secDir = ""
	p.bootErr = nil
	p.mu.Unlock()
	return nil
}

// --- helpers ---

func (p *Provisioner) run(ctx context.Context, name string, args, env []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if p.deps.Cmd == nil {
		return errors.New("tpi: nil Commander")
	}
	return p.deps.Cmd.Run(ctx, name, args, env, stdin, stdout, stderr)
}

// runStream pipes the child's stdout through a 200ms coalescing window
// to emit. The Commander double in tests writes scripted bytes synchronously
// so this still produces deterministic output.
func (p *Provisioner) runStream(ctx context.Context, name string, args, env []string, stdin io.Reader, emit provisioner.EventEmitter, stderr io.Writer) error {
	pr, pw := io.Pipe()
	var werr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		coalesce(ctx, pr, p.deps.Clock, 200*time.Millisecond, func(s string) {
			emit(provisioner.Event{Phase: provisioner.PhaseBoot, Kind: "info", Message: s, At: p.deps.Clock.Now()})
		})
	}()
	werr = p.run(ctx, name, args, env, stdin, pw, stderr)
	_ = pw.Close()
	wg.Wait()
	return werr
}

func coalesce(ctx context.Context, r io.Reader, clk clockx.Clock, window time.Duration, sink func(string)) {
	buf := make([]byte, 0, 4096)
	read := make([]byte, 1024)
	timer := clk.NewTimer(window)
	defer timer.Stop()
	timerActive := false
	flush := func() {
		if len(buf) > 0 {
			sink(string(buf))
			buf = buf[:0]
		}
	}
	for {
		// blocking read; on EOF flush and return
		n, err := r.Read(read)
		if n > 0 {
			buf = append(buf, read[:n]...)
			if !timerActive {
				timer.Reset(window)
				timerActive = true
			}
			// drain timer if window elapsed
			select {
			case <-timer.C():
				flush()
				timerActive = false
			case <-ctx.Done():
				flush()
				return
			default:
			}
		}
		if err != nil {
			flush()
			return
		}
	}
}

func (p *Provisioner) resolveRefs(node *config.Node) error {
	tpi := node.Boot.TPI
	backends, err := secrets.BuildBackends(p.deps.Cfg)
	if err != nil {
		return err
	}
	resolve := func(r config.Ref) (string, error) {
		if r == "" {
			return "", nil
		}
		out, err := secrets.ResolveTemplate(string(r), backends)
		if err != nil {
			return "", err
		}
		return out, nil
	}
	if v, err := resolve(tpi.UsernameRef); err != nil {
		return err
	} else {
		p.user = v
	}
	if v, err := resolve(tpi.PasswordRef); err != nil {
		return err
	} else {
		p.pass = v
	}
	return nil
}

func (p *Provisioner) materializeKey(node *config.Node) error {
	tpi := node.Boot.TPI
	if tpi.IdentityFileRef == "" {
		return nil
	}
	backends, err := secrets.BuildBackends(p.deps.Cfg)
	if err != nil {
		return err
	}
	val, err := secrets.ResolveTemplate(string(tpi.IdentityFileRef), backends)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".cache", "nostos", "secrets", p.runID)
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	keyPath := filepath.Join(dir, "tpi-key")
	// refuse symlink at target
	if fi, err := os.Lstat(keyPath); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("tpi: refuse to write key to symlink %s", keyPath)
		}
		_ = os.Remove(keyPath)
	}
	f, err := os.OpenFile(keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write([]byte(val)); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	p.mu.Lock()
	p.keyPath = keyPath
	p.secDir = dir
	p.mu.Unlock()
	return nil
}

var versionRE = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

func parseVersion(s string) string {
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[0]
}

func versionAtLeast(got, min string) bool {
	if got == "" {
		return false
	}
	gp, mp := splitVer(got), splitVer(min)
	for i := 0; i < 3; i++ {
		if gp[i] > mp[i] {
			return true
		}
		if gp[i] < mp[i] {
			return false
		}
	}
	return true
}
func splitVer(v string) [3]int {
	out := [3]int{}
	parts := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		out[i] = n
	}
	return out
}

// imageDigestKey is the cluster.image_digests map key.
func imageDigestKey(schematic, version, arch string) string {
	return schematic + "/" + version + "/" + arch
}

// compile-time interface check
var _ execx.Commander = execx.OSCommander{}
