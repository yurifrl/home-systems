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
	DefaultHTTPPort        = 9080
	DefaultDHCPRangeStart  = "192.168.68.200"
	DefaultDHCPRangeEnd    = "192.168.68.210"
	DefaultGateway         = "192.168.68.1"
	DefaultTFTPStaging     = "/tmp/nostos-tftp"
)

// NetworkInfo is the detected operator-host networking.
type NetworkInfo struct {
	Interface string
	IP        string
}

// Server supervises the HTTP + dnsmasq subprocesses for a PXE boot session.
type Server struct {
	Paths           paths.Paths
	HTTPPort        int
	Gateway         string
	DHCPRangeStart  string
	DHCPRangeEnd    string
	TFTPRoot        string
	Interface       string // override auto-detect
	ProxyMode       bool   // coexist with consumer's existing DHCP (recommended)

	httpSrv       *http.Server
	dnsmasqProc   *exec.Cmd
	httpRequests  chan string // relayed to consumer for progress tracking
	mu            sync.Mutex
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

// HTTPRequests returns a channel of paths served (e.g. "/configs/...").
// Only populated while the server runs.
func (s *Server) HTTPRequests() <-chan string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpRequests == nil {
		s.httpRequests = make(chan string, 64)
	}
	return s.httpRequests
}

// Preflight verifies assets + dnsmasq presence, detects interface, returns NetworkInfo.
func (s *Server) Preflight() (NetworkInfo, error) {
	for _, req := range []string{"ipxe.efi", "boot.ipxe"} {
		if _, err := os.Stat(filepath.Join(s.Paths.Assets(), req)); err != nil {
			return NetworkInfo{}, fmt.Errorf("missing %s under %s; run `nostos build` first", req, s.Paths.Assets())
		}
	}
	if !dnsmasqAvailable() {
		return NetworkInfo{}, errors.New("dnsmasq not found; install: brew install dnsmasq")
	}
	if s.Interface != "" {
		ip, err := ipForInterface(s.Interface)
		if err != nil {
			return NetworkInfo{}, fmt.Errorf("interface %s: %w", s.Interface, err)
		}
		return NetworkInfo{Interface: s.Interface, IP: ip}, nil
	}
	return detectNetwork()
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
	s.KillStaleHTTP()
	s.KillStaleDnsmasq()
	if err := s.StageTFTP(); err != nil {
		return fmt.Errorf("stage TFTP: %w", err)
	}
	if err := s.startHTTP(); err != nil {
		return fmt.Errorf("HTTP: %w", err)
	}
	if err := s.startDnsmasq(net); err != nil {
		s.Stop()
		return fmt.Errorf("dnsmasq: %w", err)
	}
	slog.Info("PXE server listening", "iface", net.Interface, "ip", net.IP, "port", s.HTTPPort)
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
	handler := loggingMiddleware(http.FileServer(http.Dir(s.Paths.State())), s)
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

func (s *Server) startDnsmasq(ni NetworkInfo) error {
	bin := "/opt/homebrew/sbin/dnsmasq"
	if _, err := os.Stat(bin); err != nil {
		if path, e := exec.LookPath("dnsmasq"); e == nil {
			bin = path
		}
	}

	var dhcpRangeArg string
	if s.ProxyMode {
		// Proxy mode: we answer only the PXE/bootfile part. Deco (or whatever
		// existing DHCP server exists on the LAN) keeps giving out IPs.
		dhcpRangeArg = "--dhcp-range=" + s.Gateway + ",proxy"
	} else {
		dhcpRangeArg = fmt.Sprintf("--dhcp-range=%s,%s,255.255.255.0,5m", s.DHCPRangeStart, s.DHCPRangeEnd)
	}

	args := []string{
		bin,
		"--no-daemon",
		"--port=0",
		"--interface=" + ni.Interface,
		dhcpRangeArg,
		"--dhcp-match=set:pxe,60,PXEClient",
		"--dhcp-ignore=tag:!pxe",
		"--enable-tftp",
		"--tftp-root=" + s.TFTPRoot,
		"--dhcp-userclass=set:ipxe,iPXE",
		fmt.Sprintf("--dhcp-boot=tag:!ipxe,ipxe.efi,,%s", ni.IP),
		fmt.Sprintf("--dhcp-boot=tag:ipxe,http://%s:%d/assets/boot.ipxe", ni.IP, s.HTTPPort),
		"--pxe-prompt=nostos boot,1", // force PXE clients to pick our bootfile immediately
		"--log-queries",
		"--log-dhcp",
	}

	if !s.ProxyMode {
		// Full-DHCP mode: we need to provide gateway/router info too.
		args = append(args,
			"--dhcp-option=3,"+s.Gateway,
			"--dhcp-option=6,"+s.Gateway,
			"--dhcp-authoritative",
		)
	}

	cmd := exec.Command("sudo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	s.dnsmasqProc = cmd
	return nil
}

// loggingMiddleware emits each served path to the HTTPRequests channel.
type srvLogger struct {
	s *Server
	w http.ResponseWriter
}

func (l *srvLogger) Header() http.Header         { return l.w.Header() }
func (l *srvLogger) Write(b []byte) (int, error) { return l.w.Write(b) }
func (l *srvLogger) WriteHeader(c int)            { l.w.WriteHeader(c) }

func loggingMiddleware(next http.Handler, s *Server) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		ch := s.httpRequests
		s.mu.Unlock()
		if ch != nil {
			select {
			case ch <- r.URL.Path:
			default: // drop if consumer is behind
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ipForInterface looks up the first IPv4 192.168.68.x address of an interface.
func ipForInterface(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if ip4 := ipn.IP.To4(); ip4 != nil && strings.HasPrefix(ip4.String(), "192.168.68.") {
				return ip4.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no 192.168.68.x IPv4 address on %s", name)
}

// detectNetwork finds an ethernet interface with a 192.168.68.x IPv4 address.
// Skips loopback / utun / awdl / bridge / etc.
func detectNetwork() (NetworkInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return NetworkInfo{}, err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		lower := strings.ToLower(iface.Name)
		if strings.HasPrefix(lower, "lo") || strings.HasPrefix(lower, "utun") ||
			strings.HasPrefix(lower, "awdl") || strings.HasPrefix(lower, "llw") ||
			strings.HasPrefix(lower, "bridge") || strings.HasPrefix(lower, "anpi") ||
			strings.HasPrefix(lower, "ap") || strings.HasPrefix(lower, "gif") ||
			strings.HasPrefix(lower, "stf") {
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
			if strings.HasPrefix(ip4.String(), "192.168.68.") {
				return NetworkInfo{Interface: iface.Name, IP: ip4.String()}, nil
			}
		}
	}
	return NetworkInfo{}, errors.New("no interface found on 192.168.68.0/24; plug in the operator host or pass --iface")
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
