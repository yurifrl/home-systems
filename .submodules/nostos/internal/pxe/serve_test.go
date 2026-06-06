package pxe

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yurifrl/nostos/internal/paths"
)

func TestFilterGatewayCollisions(t *testing.T) {
	tests := []struct {
		name string
		in   []NetworkInfo
		want []NetworkInfo
	}{
		{
			name: "interface whose IP is the gateway (.1) is excluded",
			in: []NetworkInfo{
				{Interface: "en5", IP: "192.168.68.50"},
				{Interface: "en0", IP: "192.168.68.1"}, // gateway collision
			},
			want: []NetworkInfo{
				{Interface: "en5", IP: "192.168.68.50"},
			},
		},
		{
			name: "all viable interfaces survive",
			in: []NetworkInfo{
				{Interface: "en5", IP: "192.168.68.50"},
				{Interface: "en1", IP: "10.0.0.42"},
			},
			want: []NetworkInfo{
				{Interface: "en5", IP: "192.168.68.50"},
				{Interface: "en1", IP: "10.0.0.42"},
			},
		},
		{
			name: "multiple gateway collisions all excluded",
			in: []NetworkInfo{
				{Interface: "en0", IP: "192.168.68.1"},
				{Interface: "en1", IP: "10.0.0.1"},
				{Interface: "en5", IP: "172.16.5.99"},
			},
			want: []NetworkInfo{
				{Interface: "en5", IP: "172.16.5.99"},
			},
		},
		{
			name: "invalid IP is dropped",
			in: []NetworkInfo{
				{Interface: "en9", IP: "not-an-ip"},
				{Interface: "en5", IP: "192.168.1.20"},
			},
			want: []NetworkInfo{
				{Interface: "en5", IP: "192.168.1.20"},
			},
		},
		{
			name: "empty input yields empty output",
			in:   nil,
			want: []NetworkInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterGatewayCollisions(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d nets %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("index %d: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGatewayForIP(t *testing.T) {
	tests := []struct {
		ip   string
		want string
	}{
		{"192.168.68.50", "192.168.68.1"},
		{"10.0.0.42", "10.0.0.1"},
		{"172.16.5.99", "172.16.5.1"},
		{"192.168.68.1", "192.168.68.1"}, // already the gateway
		{"not-an-ip", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := gatewayForIP(tt.ip); got != tt.want {
			t.Errorf("gatewayForIP(%q) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestSubnetBase(t *testing.T) {
	tests := []struct {
		ip   string
		want string
	}{
		{"192.168.68.50", "192.168.68"},
		{"10.0.0.42", "10.0.0"},
		{"bad", ""},
	}
	for _, tt := range tests {
		if got := subnetBase(tt.ip); got != tt.want {
			t.Errorf("subnetBase(%q) = %q, want %q", tt.ip, got, tt.want)
		}
	}
}

func TestIsVirtualIface(t *testing.T) {
	virtual := []string{"lo0", "utun3", "awdl0", "llw0", "bridge100", "anpi0", "ap1", "gif0", "stf0"}
	for _, n := range virtual {
		if !isVirtualIface(n) {
			t.Errorf("isVirtualIface(%q) = false, want true", n)
		}
	}
	physical := []string{"en0", "en5", "eth0", "enp3s0"}
	for _, n := range physical {
		if isVirtualIface(n) {
			t.Errorf("isVirtualIface(%q) = true, want false", n)
		}
	}
}

// countArgsWithPrefix returns how many args start with the given prefix.
func countArgsWithPrefix(args []string, prefix string) int {
	n := 0
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			n++
		}
	}
	return n
}

func argsContaining(args []string, substr string) []string {
	var out []string
	for _, a := range args {
		if strings.Contains(a, substr) {
			out = append(out, a)
		}
	}
	return out
}

func TestBuildDnsmasqArgsProxyMultiInterface(t *testing.T) {
	nets := []NetworkInfo{
		{Interface: "en5", IP: "192.168.68.50"},
		{Interface: "en1", IP: "10.0.0.42"},
	}
	args := buildDnsmasqArgs("/usr/sbin/dnsmasq", nets, "/tmp/nostos-tftp", 9080, true)

	// bin is first.
	if args[0] != "/usr/sbin/dnsmasq" {
		t.Errorf("expected bin first, got %q", args[0])
	}

	// --bind-interfaces must be present (point 5).
	if countArgsWithPrefix(args, "--bind-interfaces") != 1 {
		t.Errorf("expected exactly one --bind-interfaces, got args: %v", args)
	}

	// One --interface per NIC (point 3).
	if n := countArgsWithPrefix(args, "--interface="); n != 2 {
		t.Errorf("expected 2 --interface args, got %d: %v", n, argsContaining(args, "--interface="))
	}
	for _, want := range []string{"--interface=en5", "--interface=en1"} {
		if len(argsContaining(args, want)) == 0 {
			t.Errorf("missing %q", want)
		}
	}

	// One proxy --dhcp-range per NIC, keyed on each subnet gateway (point 3).
	ranges := argsContaining(args, "--dhcp-range=")
	if len(ranges) != 2 {
		t.Errorf("expected 2 proxy --dhcp-range args, got %d: %v", len(ranges), ranges)
	}
	wantRanges := map[string]bool{
		"--dhcp-range=192.168.68.1,proxy": false,
		"--dhcp-range=10.0.0.1,proxy":     false,
	}
	for _, r := range ranges {
		if _, ok := wantRanges[r]; !ok {
			t.Errorf("unexpected dhcp-range %q", r)
		}
		wantRanges[r] = true
	}
	for r, seen := range wantRanges {
		if !seen {
			t.Errorf("missing dhcp-range %q", r)
		}
	}

	// Stage 1 boot: no server IP hardcoded -> arrival interface IP is used
	// as next-server. The dnsmasq man page: omitting the address makes the
	// boot/next server the IP of the interface the request arrived on.
	stage1 := argsContaining(args, "--dhcp-boot=tag:!ipxe")
	if len(stage1) != 1 {
		t.Fatalf("expected exactly one stage-1 dhcp-boot, got %v", stage1)
	}
	if stage1[0] != "--dhcp-boot=tag:!ipxe,ipxe.efi" {
		t.Errorf("stage-1 dhcp-boot must omit a server IP, got %q", stage1[0])
	}

	// Stage 2 chainload: one per-interface HTTP URL keyed on the auto-set
	// interface-name tag, each advertising its OWN IP (point 4).
	stage2 := argsContaining(args, "--dhcp-boot=tag:ipxe")
	if len(stage2) != 2 {
		t.Fatalf("expected 2 stage-2 dhcp-boot args, got %v", stage2)
	}
	wantStage2 := map[string]bool{
		"--dhcp-boot=tag:ipxe,tag:en5,http://192.168.68.50:9080/boot.ipxe?mac=${mac:hexhyp}": false,
		"--dhcp-boot=tag:ipxe,tag:en1,http://10.0.0.42:9080/boot.ipxe?mac=${mac:hexhyp}":     false,
	}
	for _, b := range stage2 {
		if _, ok := wantStage2[b]; !ok {
			t.Errorf("unexpected stage-2 dhcp-boot %q", b)
		}
		wantStage2[b] = true
	}
	for b, seen := range wantStage2 {
		if !seen {
			t.Errorf("missing stage-2 dhcp-boot %q", b)
		}
	}

	// Guard against the original bug: no single foreign IP hardcoded across
	// all interfaces. Each interface's IP appears only with its own tag.
	for _, a := range args {
		if strings.Contains(a, "192.168.68.50") && strings.Contains(a, "10.0.0.42") {
			t.Errorf("arg mixes two interface IPs (foreign next-server bug): %q", a)
		}
	}
}

func TestBuildDnsmasqArgsSingleInterface(t *testing.T) {
	nets := []NetworkInfo{{Interface: "en0", IP: "192.168.1.10"}}
	args := buildDnsmasqArgs("dnsmasq", nets, "/tmp/t", 9080, true)

	if n := countArgsWithPrefix(args, "--interface="); n != 1 {
		t.Errorf("expected 1 --interface, got %d", n)
	}
	if n := len(argsContaining(args, "--dhcp-range=")); n != 1 {
		t.Errorf("expected 1 dhcp-range, got %d", n)
	}
	if got := argsContaining(args, "--dhcp-range="); got[0] != "--dhcp-range=192.168.1.1,proxy" {
		t.Errorf("got %q", got[0])
	}
	// Proxy mode must NOT emit authoritative / router options.
	if len(argsContaining(args, "--dhcp-authoritative")) != 0 {
		t.Errorf("proxy mode should not be authoritative")
	}
}

func TestBuildDnsmasqArgsFullDHCPMode(t *testing.T) {
	nets := []NetworkInfo{{Interface: "en0", IP: "192.168.1.10"}}
	args := buildDnsmasqArgs("dnsmasq", nets, "/tmp/t", 9080, false)

	// Full-DHCP mode: per-subnet allocation range tagged by interface.
	rng := argsContaining(args, "--dhcp-range=")
	if len(rng) != 1 {
		t.Fatalf("expected 1 dhcp-range, got %v", rng)
	}
	if !strings.Contains(rng[0], "set:en0,192.168.1.200,192.168.1.210") {
		t.Errorf("unexpected full-dhcp range: %q", rng[0])
	}
	// Router/DNS options + authoritative present.
	if len(argsContaining(args, "--dhcp-option=tag:en0,3,192.168.1.1")) != 1 {
		t.Errorf("missing router option; args: %v", args)
	}
	if len(argsContaining(args, "--dhcp-authoritative")) != 1 {
		t.Errorf("full-dhcp mode must be authoritative")
	}
}

// TestLocalIPs verifies LocalIPs() always includes loopback and every IP the
// server is bound to (s.networks).
func TestLocalIPs(t *testing.T) {
	s := &Server{
		networks: []NetworkInfo{
			{Interface: "en5", IP: "192.168.68.50"},
			{Interface: "en1", IP: "10.0.0.42"},
			{Interface: "en9", IP: ""}, // empty IPs are skipped
		},
	}
	got := map[string]bool{}
	for _, ip := range s.LocalIPs() {
		got[ip] = true
	}
	for _, want := range []string{"127.0.0.1", "::1", "192.168.68.50", "10.0.0.42"} {
		if !got[want] {
			t.Errorf("LocalIPs() missing %q; got %v", want, s.LocalIPs())
		}
	}
	if got[""] {
		t.Errorf("LocalIPs() should not include empty IP; got %v", s.LocalIPs())
	}
}

// TestSudoOK pins the pure CheckSudo decision logic: serving is safe when the
// sudoers drop-in is installed OR sudo runs non-interactively; otherwise
// ErrSudoRequired so the caller fails fast instead of hanging on a password
// prompt (critical: Preflight runs before the destructive wipe).
func TestSudoOK(t *testing.T) {
	tests := []struct {
		name             string
		sudoersInstalled bool
		nonInteractiveOK bool
		wantErr          bool
	}{
		{"sudoers installed only", true, false, false},
		{"non-interactive sudo only", false, true, false},
		{"both available", true, true, false},
		{"neither available -> ErrSudoRequired", false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sudoOK(tt.sudoersInstalled, tt.nonInteractiveOK)
			if tt.wantErr {
				if !errors.Is(err, ErrSudoRequired) {
					t.Fatalf("sudoOK(%v,%v) = %v, want ErrSudoRequired", tt.sudoersInstalled, tt.nonInteractiveOK, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("sudoOK(%v,%v) = %v, want nil", tt.sudoersInstalled, tt.nonInteractiveOK, err)
			}
		})
	}
}

// TestLocalIPsNoNetworks verifies loopback is present even with no interfaces.
func TestLocalIPsNoNetworks(t *testing.T) {
	s := &Server{}
	got := s.LocalIPs()
	if len(got) != 2 || got[0] != "127.0.0.1" || got[1] != "::1" {
		t.Errorf("LocalIPs() with no networks = %v, want [127.0.0.1 ::1]", got)
	}
}

// TestLoggingMiddlewareStampsSourceIP proves the middleware attributes each
// served request to its source IP (host portion of RemoteAddr) on the channel.
func TestLoggingMiddlewareStampsSourceIP(t *testing.T) {
	s := &Server{httpRequests: make(chan HTTPRequest, 4)}
	h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), s)

	req := httptest.NewRequest(http.MethodGet, "/configs/d0-94-66-d9-eb-a5.yaml", nil)
	req.RemoteAddr = "192.168.68.123:54321"
	h.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case got := <-s.httpRequests:
		if got.Path != "/configs/d0-94-66-d9-eb-a5.yaml" {
			t.Errorf("Path = %q", got.Path)
		}
		if got.SourceIP != "192.168.68.123" {
			t.Errorf("SourceIP = %q, want 192.168.68.123", got.SourceIP)
		}
	default:
		t.Fatal("middleware did not emit an HTTPRequest")
	}
}

// TestLoggingMiddlewareSourceIPNoPort verifies a RemoteAddr without a port
// falls back to the raw value.
func TestLoggingMiddlewareSourceIPNoPort(t *testing.T) {
	s := &Server{httpRequests: make(chan HTTPRequest, 4)}
	h := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), s)

	req := httptest.NewRequest(http.MethodGet, "/assets/boot.ipxe", nil)
	req.RemoteAddr = "192.168.68.200" // no port
	h.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case got := <-s.httpRequests:
		if got.SourceIP != "192.168.68.200" {
			t.Errorf("SourceIP = %q, want raw 192.168.68.200", got.SourceIP)
		}
	default:
		t.Fatal("middleware did not emit an HTTPRequest")
	}
}

// newTestServer builds a Server whose state dir is an isolated temp dir
// (via XDG_DATA_HOME) and writes a fake install-chain boot.ipxe under Assets.
// Returns the server and the install-chain bytes for comparison.
func newTestServer(t *testing.T) (*Server, []byte) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	p := paths.New(filepath.Join(t.TempDir(), "config.yaml"))
	if err := os.MkdirAll(p.Assets(), 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	chain := []byte("#!ipxe\n# install chain\nkernel http://x/assets/vmlinuz-amd64\nboot\n")
	if err := os.WriteFile(filepath.Join(p.Assets(), "boot.ipxe"), chain, 0o644); err != nil {
		t.Fatalf("write boot.ipxe: %v", err)
	}
	return &Server{Paths: p}, chain
}

// TestHandleBootIpxeInstallChain proves a MAC that is NOT installed receives
// the rendered install-chain bytes verbatim (the unchanged install behavior).
func TestHandleBootIpxeInstallChain(t *testing.T) {
	s, chain := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/boot.ipxe?mac=d0-94-66-d9-eb-a5", nil)
	rec := httptest.NewRecorder()
	s.handleBootIpxe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != string(chain) {
		t.Errorf("install chain mismatch:\ngot  %q\nwant %q", rec.Body.String(), string(chain))
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
}

// TestHandleBootIpxeBootFromDisk proves that after MarkInstalled the same MAC
// receives the boot-from-disk script (contains `exit`), not the install chain.
func TestHandleBootIpxeBootFromDisk(t *testing.T) {
	s, chain := newTestServer(t)
	if err := MarkInstalled(s.Paths.InstalledMACs(), "d0-94-66-d9-eb-a5"); err != nil {
		t.Fatalf("MarkInstalled: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/boot.ipxe?mac=d0-94-66-d9-eb-a5", nil)
	rec := httptest.NewRecorder()
	s.handleBootIpxe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body == string(chain) {
		t.Fatal("installed MAC got the install chain; want boot-from-disk script")
	}
	if !strings.Contains(body, "exit") {
		t.Errorf("boot-from-disk script missing `exit`:\n%s", body)
	}
	if !strings.Contains(body, "d0-94-66-d9-eb-a5") {
		t.Errorf("boot-from-disk script missing MAC echo:\n%s", body)
	}
}

// TestHandleBootIpxeNoMAC proves a request with no mac query param falls
// through to the install chain (a MAC-less probe never settles to disk).
func TestHandleBootIpxeNoMAC(t *testing.T) {
	s, chain := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/boot.ipxe", nil)
	rec := httptest.NewRecorder()
	s.handleBootIpxe(rec, req)

	if rec.Body.String() != string(chain) {
		t.Errorf("no-mac request did not get install chain:\ngot %q", rec.Body.String())
	}
}

// TestHandleBootIpxeMissingFile proves a missing rendered boot.ipxe yields a
// clear 404 rather than an empty 200.
func TestHandleBootIpxeMissingFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	s := &Server{Paths: paths.New(filepath.Join(t.TempDir(), "config.yaml"))}

	req := httptest.NewRequest(http.MethodGet, "/boot.ipxe?mac=aa-bb-cc-dd-ee-ff", nil)
	rec := httptest.NewRecorder()
	s.handleBootIpxe(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
