package pxe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yurifrl/nostos/internal/paths"
)

const (
	DefaultHTTPPort       = 9080
	DefaultDHCPRangeStart = "192.168.68.200"
	DefaultDHCPRangeEnd   = "192.168.68.210"
	DefaultGateway        = "192.168.68.1"
	DefaultTFTPStaging    = "/tmp/nostos-tftp"
)

// ErrSudoRequired is returned by CheckSudo (and surfaced from Preflight) when
// nostos cannot run sudo without an interactive password prompt. Spawning
// `sudo dnsmasq` in that state would block on an invisible prompt — so we
// refuse up front and tell the operator to run `nostos pxe setup`.
var ErrSudoRequired = errors.New("sudo required to bind privileged ports (67/69/4011); run `nostos pxe setup`")

// sudoNonInteractiveOK reports whether sudo can run without a password — either
// a NOPASSWD rule applies or a credential is cached. `sudo -n` NEVER prompts;
// it fails fast instead, so this probe can never hang the caller's terminal.
func sudoNonInteractiveOK() bool {
	return exec.Command("sudo", "-n", "true").Run() == nil
}

// sudoOK is the pure decision logic composed by CheckSudo: if the sudoers
// drop-in is installed OR sudo can already run non-interactively, serving is
// safe; otherwise spawning `sudo dnsmasq` would hang on a password prompt.
func sudoOK(sudoersInstalled, nonInteractiveOK bool) error {
	if sudoersInstalled || nonInteractiveOK {
		return nil
	}
	return ErrSudoRequired
}

// CheckSudo returns nil when the PXE server can spawn `sudo dnsmasq` without
// blocking on a password prompt (sudoers drop-in installed, or sudo runs
// non-interactively). Otherwise it returns ErrSudoRequired so the caller can
// fail fast — critical because Preflight runs BEFORE the destructive wipe in
// the install flow.
func CheckSudo() error {
	return sudoOK(SudoersInstalled(), sudoNonInteractiveOK())
}

// NetworkInfo is the detected operator-host networking.
type NetworkInfo struct {
	Interface string
	IP        string
}

// HTTPRequest is a single served HTTP request, attributed to its source.
// SourceIP is the request's remote IP (host portion of r.RemoteAddr), used by
// the consumer to distinguish the booting node from the operator host itself.
type HTTPRequest struct {
	Path     string
	SourceIP string
}

// Server supervises the HTTP + dnsmasq subprocesses for a PXE boot session.
type Server struct {
	Paths          paths.Paths
	HTTPPort       int
	Gateway        string
	DHCPRangeStart string
	DHCPRangeEnd   string
	TFTPRoot       string
	Interface      string // override auto-detect
	ProxyMode      bool   // coexist with consumer's existing DHCP (recommended)

	// LogJSONPath, when set, mirrors the recorded event stream as NDJSON to an
	// additional file (the detached-tail "--log-json" pattern). Optional.
	LogJSONPath string

	httpSrv      *http.Server
	dnsmasqProc  *exec.Cmd
	httpRequests chan HTTPRequest // relayed to consumer for progress tracking
	networks     []NetworkInfo    // all viable interfaces detected by Preflight
	events       *EventStore      // per-node lifecycle event recorder (best-effort)
	logJSON      *EventStore      // optional mirror of events to LogJSONPath
	mu           sync.Mutex
}

// recordEvent appends ev to the primary event store and, if configured, to the
// LogJSONPath mirror. Best-effort: errors are logged, never propagated, so
// recording never affects the HTTP response or dnsmasq lifecycle. Safe on a
// Server whose stores are nil.
func (s *Server) recordEvent(ev Event) {
	s.mu.Lock()
	store := s.events
	mirror := s.logJSON
	s.mu.Unlock()
	if store != nil {
		if err := store.Append(ev); err != nil {
			slog.Warn("pxe event record failed", "err", err)
		}
	}
	if mirror != nil {
		if err := mirror.Append(ev); err != nil {
			slog.Warn("pxe event log-json failed", "err", err)
		}
	}
}

// NewServer constructs a server with sensible defaults.
func NewServer(p paths.Paths) *Server {
	return &Server{
		Paths:          p,
		HTTPPort:       DefaultHTTPPort,
		Gateway:        DefaultGateway,
		DHCPRangeStart: DefaultDHCPRangeStart,
		DHCPRangeEnd:   DefaultDHCPRangeEnd,
		TFTPRoot:       DefaultTFTPStaging,
		ProxyMode:      true, // coexist with consumer's DHCP by default
	}
}

// HTTPRequests returns a channel of requests served (path + source IP).
// Only populated while the server runs.
func (s *Server) HTTPRequests() <-chan HTTPRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpRequests == nil {
		s.httpRequests = make(chan HTTPRequest, 64)
	}
	return s.httpRequests
}

// LocalIPs returns the IPs that belong to the operator host running this
// server: loopback (127.0.0.1, ::1) plus every interface IP the server is
// bound to (s.networks). The consumer uses this to reject a config-fetch
// signal originating from the operator host itself (e.g. a hand-rolled curl),
// which would otherwise be a false "installing" progress event.
func (s *Server) LocalIPs() []string {
	s.mu.Lock()
	nets := s.networks
	s.mu.Unlock()
	ips := make([]string, 0, len(nets)+2)
	ips = append(ips, "127.0.0.1", "::1")
	for _, n := range nets {
		if n.IP != "" {
			ips = append(ips, n.IP)
		}
	}
	return ips
}

// Preflight verifies assets + dnsmasq presence, detects every viable LAN
// interface, and returns the first one (for CLI display / logging). The full
// set is stashed on the Server so Start() can serve PXE on all of them
// simultaneously.
//
// It fails only when ZERO interfaces survive the gateway-collision filter.
func (s *Server) Preflight() (NetworkInfo, error) {
	for _, req := range []string{"ipxe.efi", "boot.ipxe"} {
		if _, err := os.Stat(filepath.Join(s.Paths.Assets(), req)); err != nil {
			return NetworkInfo{}, fmt.Errorf("missing %s under %s; run `nostos build` first", req, s.Paths.Assets())
		}
	}
	if !dnsmasqAvailable() {
		return NetworkInfo{}, errors.New("dnsmasq not found; install: brew install dnsmasq")
	}
	// Fail fast if `sudo dnsmasq` would block on an invisible password prompt.
	// This runs BEFORE the destructive wipe in the install flow, so a missing
	// sudoers config aborts cleanly instead of wiping-then-hanging.
	if err := CheckSudo(); err != nil {
		return NetworkInfo{}, err
	}
	if s.Interface != "" {
		ip, err := ipForInterface(s.Interface)
		if err != nil {
			return NetworkInfo{}, fmt.Errorf("interface %s: %w", s.Interface, err)
		}
		ni := NetworkInfo{Interface: s.Interface, IP: ip}
		// The user explicitly named this interface, so we honor it even if its
		// IP collides with the subnet gateway (.1). We only warn: an explicit
		// override is a deliberate operator choice, unlike auto-detection where
		// a gateway-IP interface is almost certainly the wrong NIC.
		if gw := gatewayForIP(ni.IP); gw != "" && ni.IP == gw {
			slog.Warn("override interface IP equals its subnet gateway; advertising it as next-server may break PXE", "iface", ni.Interface, "ip", ni.IP)
		}
		s.networks = []NetworkInfo{ni}
		return ni, nil
	}
	nets, err := detectNetworks()
	if err != nil {
		return NetworkInfo{}, err
	}
	if len(nets) == 0 {
		return NetworkInfo{}, errors.New("no usable IPv4 interface found after gateway-collision filter; pass --iface to override")
	}
	s.networks = nets
	return nets[0], nil
}

// StageTFTP copies ipxe.efi into a world-readable staging path for dnsmasq.
func (s *Server) StageTFTP() error {
	if err := os.MkdirAll(s.TFTPRoot, 0o755); err != nil {
		return err
	}
	src := filepath.Join(s.Paths.Assets(), "ipxe.efi")
	dst := filepath.Join(s.TFTPRoot, "ipxe.efi")
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Chmod(dst, 0o644)
}

// KillStaleHTTP kills any process currently bound to the HTTP port.
func (s *Server) KillStaleHTTP() {
	out, err := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", s.HTTPPort)).Output()
	if err != nil || len(out) == 0 {
		return
	}
	for _, line := range strings.Fields(string(out)) {
		var pid int
		fmt.Sscanf(line, "%d", &pid)
		if pid > 0 {
			slog.Info("killing stale process", "port", s.HTTPPort, "pid", pid)
			syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	time.Sleep(500 * time.Millisecond)
}

// KillStaleDnsmasq asks sudo to kill any leftover nostos-managed dnsmasq.
func (s *Server) KillStaleDnsmasq() {
	_ = exec.Command("sudo", "-n", "pkill", "-f", "dnsmasq.*tftp-root="+s.TFTPRoot).Run()
	time.Sleep(500 * time.Millisecond)
}

// Start launches HTTP + dnsmasq. Returns on ctx cancel or dnsmasq exit.
// Progress events are surfaced via HTTPRequests().
func (s *Server) Start(ctx context.Context, net NetworkInfo) error {
	s.inferDefaults(net)
	// Initialize the per-node event recorder under State(). LogJSONPath, when
	// set, gets a second NDJSON sink for detached tailing.
	s.mu.Lock()
	s.events = NewEventStore(s.Paths.State())
	if s.LogJSONPath != "" {
		s.logJSON = &EventStore{path: s.LogJSONPath}
	}
	s.mu.Unlock()
	s.KillStaleHTTP()
	s.KillStaleDnsmasq()
	if err := s.StageTFTP(); err != nil {
		return fmt.Errorf("stage TFTP: %w", err)
	}
	if err := s.startHTTP(); err != nil {
		return fmt.Errorf("HTTP: %w", err)
	}
	// Serve on every viable interface Preflight discovered. Fall back to the
	// single passed-in NetworkInfo if Preflight wasn't routed through this
	// Server (defensive; the CLI always calls Preflight first).
	nets := s.networks
	if len(nets) == 0 {
		nets = []NetworkInfo{net}
	}
	if err := s.startDnsmasq(nets); err != nil {
		s.Stop()
		return fmt.Errorf("dnsmasq: %w", err)
	}
	ifaces := make([]string, 0, len(nets))
	ips := make([]string, 0, len(nets))
	for _, n := range nets {
		ifaces = append(ifaces, n.Interface)
		ips = append(ips, n.IP)
	}
	slog.Info("PXE server listening", "ifaces", strings.Join(ifaces, ","), "ips", strings.Join(ips, ","), "port", s.HTTPPort)
	return nil
}

// Wait blocks until dnsmasq exits (or ctx is cancelled) and then stops everything.
func (s *Server) Wait(ctx context.Context) error {
	if s.dnsmasqProc == nil {
		return errors.New("dnsmasq not running")
	}
	done := make(chan error, 1)
	go func() {
		done <- s.dnsmasqProc.Wait()
	}()
	select {
	case <-ctx.Done():
		s.Stop()
		return ctx.Err()
	case err := <-done:
		s.Stop()
		return err
	}
}

// Stop terminates the subprocesses.
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dnsmasqProc != nil && s.dnsmasqProc.Process != nil {
		_ = s.dnsmasqProc.Process.Signal(syscall.SIGTERM)
		// give sudo + dnsmasq a moment
		done := make(chan struct{})
		go func() { s.dnsmasqProc.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = s.dnsmasqProc.Process.Kill()
		}
	}
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.httpSrv.Shutdown(ctx)
		cancel()
	}
	if s.httpRequests != nil {
		close(s.httpRequests)
		s.httpRequests = nil
	}
}

// --- internals ---

func (s *Server) startHTTP() error {
	// A ServeMux routes /boot.ipxe to the dynamic per-MAC handler and everything
	// else (/assets/..., /configs/...) to the static FileServer rooted at
	// State(). The WHOLE mux is wrapped in loggingMiddleware so event recording
	// still observes every request (kernel/initramfs/config fetches included).
	mux := http.NewServeMux()
	mux.HandleFunc("/boot.ipxe", s.handleBootIpxe)
	mux.Handle("/", http.FileServer(http.Dir(s.Paths.State())))
	handler := loggingMiddleware(mux, s)
	s.httpSrv = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.HTTPPort),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute, // initramfs is 100+ MB
	}
	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		return err
	}
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP serve error", "err", err)
		}
	}()
	return nil
}

// handleBootIpxe serves the per-MAC stage-2 iPXE script. An already-installed
// MAC (config previously fetched, not currently being reinstalled) gets the
// boot-from-disk script so its post-install reboot settles to local disk. A
// fresh or reinstalling MAC gets the rendered install chain verbatim.
//
// Install vs reinstall is disambiguated purely by installed-state: the
// provisioner's Prepare() ClearInstalled()s the MAC at the start of every
// (re)install, so "installed" already means "not currently being reinstalled".
// This keeps the SERVE side free of any internal/cluster import.
func (s *Server) handleBootIpxe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	mac := strings.ToLower(r.URL.Query().Get("mac"))
	if mac != "" && IsInstalled(s.Paths.InstalledMACs(), mac) {
		fmt.Fprintf(w, bootFromDiskScript, mac)
		return
	}
	// Install chain: serve the bytes of the already-rendered boot.ipxe.
	install := filepath.Join(s.Paths.Assets(), "boot.ipxe")
	b, err := os.ReadFile(install)
	if err != nil {
		http.Error(w, "boot.ipxe not rendered; run `nostos build` first", http.StatusNotFound)
		return
	}
	_, _ = w.Write(b)
}

// bootFromDiskScript is served to an already-installed MAC. On the lab's Dell
// UEFI hardware, iPXE `exit` hands control back to the UEFI boot manager,
// which proceeds to the next boot entry (the local disk) — so the node settles
// WITHOUT any BIOS boot-order change. The %s is the MAC, for log clarity.
//
// BIOS-mode alternative (not used here, kept for reference): replace `exit`
// with `sanboot --no-describe --drive 0x80` to chain the first BIOS disk.
const bootFromDiskScript = `#!ipxe
echo nostos: %s already installed; booting from local disk
exit
`

func (s *Server) startDnsmasq(nets []NetworkInfo) error {
	bin := dnsmasqBinary()

	args := buildDnsmasqArgs(bin, nets, s.TFTPRoot, s.HTTPPort, s.ProxyMode)

	cmd := exec.Command("sudo", args...)
	// Tee dnsmasq output to the console (unchanged behavior) AND a line
	// recorder that parses --log-dhcp lines into lifecycle events. This is how
	// the IP<->MAC correlation (DHCPACK) and tftp delivery get recorded without
	// touching the HTTP-request channel.
	rec := newDnsmasqRecorder(s)
	cmd.Stdout = io.MultiWriter(os.Stdout, rec)
	cmd.Stderr = io.MultiWriter(os.Stderr, rec)
	if err := cmd.Start(); err != nil {
		return err
	}
	s.dnsmasqProc = cmd
	return nil
}

// dnsmasqRecorder is an io.Writer that buffers bytes into lines, runs
// ParseDnsmasqLine over each complete line, and records recognized events.
// It preserves all bytes for the console writer it is multiplexed with.
type dnsmasqRecorder struct {
	s   *Server
	mu  sync.Mutex
	buf []byte
}

func newDnsmasqRecorder(s *Server) *dnsmasqRecorder { return &dnsmasqRecorder{s: s} }

func (d *dnsmasqRecorder) Write(p []byte) (int, error) {
	d.mu.Lock()
	d.buf = append(d.buf, p...)
	for {
		idx := indexByte(d.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(d.buf[:idx])
		d.buf = d.buf[idx+1:]
		if ev, ok := ParseDnsmasqLine(line); ok {
			d.s.recordEvent(ev)
		}
	}
	d.mu.Unlock()
	return len(p), nil
}

// indexByte is a tiny local helper to avoid importing bytes for one call.
func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// buildDnsmasqArgs assembles the dnsmasq argv for a PXE session that serves
// every interface in nets simultaneously. It is pure (no I/O, no globals) so
// it can be unit-tested without spawning processes or touching real NICs.
//
// Per-interface next-server resolution is the crux of the bug fix:
//
//   - Stage 1 (firmware over TFTP): `--dhcp-boot=tag:!ipxe,ipxe.efi` omits the
//     server address entirely. In proxy mode dnsmasq then fills next-server
//     with the IP of the interface the request ARRIVED on (per the man page:
//     "if not provided ... the address set to the address of the machine
//     running dnsmasq"). This replaces the old single hardcoded ni.IP that
//     could advertise a foreign/router IP.
//
//   - Stage 2 (iPXE chainload over HTTP): the URL must embed a literal IP, so
//     we emit one --dhcp-boot per interface keyed on the interface-name tag
//     that dnsmasq sets automatically on every request ("Each request is also
//     tagged with the name of the interface on which the request arrived").
//     Each NIC therefore advertises its OWN IP in the chainload URL.
func buildDnsmasqArgs(bin string, nets []NetworkInfo, tftpRoot string, httpPort int, proxyMode bool) []string {
	args := []string{
		bin,
		"--no-daemon",
		"--port=0",
		// Bind only the listed interfaces' reply sockets to those NICs so we
		// never answer (or source replies) on the wrong interface.
		"--bind-interfaces",
	}

	// One --interface per detected NIC.
	for _, n := range nets {
		args = append(args, "--interface="+n.Interface)
	}

	// One dhcp-range per NIC so dnsmasq answers PXE on each subnet.
	if proxyMode {
		// Proxy mode: the LAN's existing DHCP server keeps handing out IPs; we
		// only answer the PXE/bootfile part. The address just identifies the
		// subnet to proxy for.
		for _, n := range nets {
			args = append(args, "--dhcp-range="+gatewayForIP(n.IP)+",proxy")
		}
	} else {
		// Full-DHCP mode: derive a small per-subnet range, tagged by interface.
		for _, n := range nets {
			base := subnetBase(n.IP)
			args = append(args, fmt.Sprintf("--dhcp-range=set:%s,%s.200,%s.210,255.255.255.0,5m", n.Interface, base, base))
		}
	}

	args = append(args,
		"--dhcp-match=set:pxe,60,PXEClient",
		"--dhcp-ignore=tag:!pxe",
		"--enable-tftp",
		"--tftp-root="+tftpRoot,
		"--dhcp-userclass=set:ipxe,iPXE",
		// Stage 1: no server address -> arrival-interface IP becomes next-server.
		"--dhcp-boot=tag:!ipxe,ipxe.efi",
	)

	// Stage 2: per-interface HTTP chainload URL keyed on the auto-set
	// interface-name tag so each NIC advertises its own IP.
	for _, n := range nets {
		args = append(args, fmt.Sprintf("--dhcp-boot=tag:ipxe,tag:%s,http://%s:%d/boot.ipxe?mac=${mac:hexhyp}", n.Interface, n.IP, httpPort))
	}

	args = append(args,
		"--pxe-prompt=nostos boot,1", // force PXE clients to pick our bootfile immediately
		"--log-queries",
		"--log-dhcp",
	)

	if !proxyMode {
		// Full-DHCP mode: provide gateway/router info per subnet.
		for _, n := range nets {
			gw := gatewayForIP(n.IP)
			args = append(args,
				"--dhcp-option=tag:"+n.Interface+",3,"+gw,
				"--dhcp-option=tag:"+n.Interface+",6,"+gw,
			)
		}
		args = append(args, "--dhcp-authoritative")
	}

	return args
}

// loggingMiddleware emits each served path to the HTTPRequests channel.
type srvLogger struct {
	s *Server
	w http.ResponseWriter
}

func (l *srvLogger) Header() http.Header         { return l.w.Header() }
func (l *srvLogger) Write(b []byte) (int, error) { return l.w.Write(b) }
func (l *srvLogger) WriteHeader(c int)           { l.w.WriteHeader(c) }

func loggingMiddleware(next http.Handler, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		ch := s.httpRequests
		s.mu.Unlock()
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			ip = host
		}
		if ch != nil {
			select {
			case ch <- HTTPRequest{Path: r.URL.Path, SourceIP: ip}:
			default: // drop if consumer is behind
			}
		}
		// Record the event at the SOURCE, independent of any channel consumer.
		// This avoids a second reader of httpRequests (which would steal events
		// from the install-flow provisioner). During a standalone serve there is
		// no consumer and recording still happens here.
		if phase, mac := ClassifyHTTPPath(r.URL.Path); phase != PhaseUnknown {
			s.recordEvent(Event{IP: ip, MAC: mac, Phase: phase, Message: r.URL.Path})
		}
		next.ServeHTTP(w, r)
	})
}

// ipForInterface returns the first usable IPv4 address on the interface,
// preferring private (RFC 1918) ranges over link-local / loopback.
func ipForInterface(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}
	var fallback string
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip4 := ipn.IP.To4()
		if ip4 == nil {
			continue
		}
		if ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsUnspecified() {
			continue
		}
		if ip4.IsPrivate() {
			return ip4.String(), nil
		}
		if fallback == "" {
			fallback = ip4.String()
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no usable IPv4 address on %s", name)
}

// isVirtualIface reports whether the interface name is a virtual / non-LAN
// device that should never carry PXE traffic (loopback, tunnels, AirDrop,
// bridges, etc.). Prefix list mirrors the historical exclusions.
func isVirtualIface(name string) bool {
	lower := strings.ToLower(name)
	for _, p := range []string{"lo", "utun", "awdl", "llw", "bridge", "anpi", "ap", "gif", "stf"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

// gatewayForIP returns the first host (.1) of the /24 containing ip, matching
// the inferDefaults convention. Returns "" if ip is not a valid IPv4 address.
func gatewayForIP(ipStr string) string {
	ip := net.ParseIP(ipStr).To4()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.1", ip[0], ip[1], ip[2])
}

// subnetBase returns the "x.x.x" /24 prefix of ip, or "" if invalid.
func subnetBase(ipStr string) string {
	ip := net.ParseIP(ipStr).To4()
	if ip == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", ip[0], ip[1], ip[2])
}

// filterGatewayCollisions drops any candidate whose own IPv4 equals the
// subnet gateway (.1 of its /24). Advertising the router's own IP as
// next-server is exactly what caused the reprovision bug, so such an
// interface must never be used. Pure helper for unit testing.
func filterGatewayCollisions(nets []NetworkInfo) []NetworkInfo {
	out := make([]NetworkInfo, 0, len(nets))
	for _, n := range nets {
		gw := gatewayForIP(n.IP)
		if gw == "" || n.IP == gw {
			continue
		}
		out = append(out, n)
	}
	return out
}

// candidateNetworks finds every up, non-loopback, non-virtual interface that
// has a usable private (RFC1918) IPv4 address — the PRE gateway-collision
// filter list. doctor uses this to surface collisions; detectNetworks layers
// the collision filter on top. Kept separate so both views share one scan.
func candidateNetworks() ([]NetworkInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var cands []NetworkInfo
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualIface(iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil {
				continue
			}
			if ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsUnspecified() {
				continue
			}
			if ip4.IsPrivate() {
				cands = append(cands, NetworkInfo{Interface: iface.Name, IP: ip4.String()})
				break // one address per interface is enough
			}
		}
	}
	return cands, nil
}

// detectNetworks finds every up, non-loopback, non-virtual interface that has
// a usable private (RFC1918) IPv4 address, then applies the gateway-collision
// filter. The PXE server serves on ALL of these simultaneously so it works
// regardless of whether the operator host and target node share a NIC.
func detectNetworks() ([]NetworkInfo, error) {
	cands, err := candidateNetworks()
	if err != nil {
		return nil, err
	}
	return filterGatewayCollisions(cands), nil
}

// detectNetwork returns the first viable interface from detectNetworks().
// Retained for backward compatibility with single-interface callers/tests.
func detectNetwork() (NetworkInfo, error) {
	nets, err := detectNetworks()
	if err != nil {
		return NetworkInfo{}, err
	}
	if len(nets) == 0 {
		return NetworkInfo{}, errors.New("no usable IPv4 interface found; pass --iface to override")
	}
	return nets[0], nil
}

// inferDefaults updates DHCP range and gateway to match the detected
// interface's subnet. Used after Preflight() chose a NetworkInfo so the
// dnsmasq invocation lines up with the operator's actual LAN.
func (s *Server) inferDefaults(ni NetworkInfo) {
	if ni.IP == "" {
		return
	}
	ip := net.ParseIP(ni.IP).To4()
	if ip == nil {
		return
	}
	// Default to /24 unless the operator overrode the gateway.
	if s.Gateway == DefaultGateway {
		// gateway = first host of the /24
		s.Gateway = fmt.Sprintf("%d.%d.%d.1", ip[0], ip[1], ip[2])
	}
	if s.DHCPRangeStart == DefaultDHCPRangeStart {
		s.DHCPRangeStart = fmt.Sprintf("%d.%d.%d.200", ip[0], ip[1], ip[2])
	}
	if s.DHCPRangeEnd == DefaultDHCPRangeEnd {
		s.DHCPRangeEnd = fmt.Sprintf("%d.%d.%d.210", ip[0], ip[1], ip[2])
	}
}

func dnsmasqAvailable() bool {
	if _, err := os.Stat("/opt/homebrew/sbin/dnsmasq"); err == nil {
		return true
	}
	_, err := exec.LookPath("dnsmasq")
	return err == nil
}

// tailReader is a no-op placeholder retained for backward compatibility.
var _ io.Reader = (*strings.Reader)(nil)
