package pxe

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yurifrl/nostos/internal/paths"
)

// Check is a single preflight diagnosis result.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
}

// DoctorReport aggregates every preflight check plus the viable interfaces the
// PXE server would serve on. OK is true iff every check passed.
type DoctorReport struct {
	OK         bool          `json:"ok"`
	Interfaces []NetworkInfo `json:"interfaces"` // viable (post-collision) NICs that will be served
	Checks     []Check       `json:"checks"`
}

// RunDoctor runs every preflight check and returns an aggregated report.
// All network / port / HTTP side effects are confined to this function (and
// the check helpers it calls); the pure decision helpers it relies on
// (aggregateOK, gatewayCollisionCheck) are unit-tested without sockets.
func RunDoctor(p paths.Paths, httpPort int) DoctorReport {
	var report DoctorReport

	// 1. Assets present under p.Assets().
	checks := []Check{assetsCheck(p)}

	// 2. dnsmasq binary present.
	checks = append(checks, dnsmasqCheck())

	// 3. Viable interfaces (post gateway-collision filter).
	nets, derr := detectNetworks()
	report.Interfaces = nets
	checks = append(checks, interfacesCheck(nets, derr))

	// 4. Gateway collision: compare the pre-filter candidate set against the
	// post-filter viable set so the operator always sees a router-IP NIC.
	cands, _ := candidateNetworks()
	checks = append(checks, gatewayCollisionCheck(cands, nets))

	// 5. HTTP port bindable.
	checks = append(checks, httpPortCheck(httpPort))

	// 6. HTTP self-test fetch over an ephemeral port.
	checks = append(checks, httpSelfTestCheck(p.Assets()))

	// 7. Privileged ports / sudoers readiness.
	checks = append(checks, sudoersCheck())

	report.Checks = checks
	report.OK = aggregateOK(checks)
	return report
}

// aggregateOK reports whether every check passed. Pure; unit-tested.
func aggregateOK(checks []Check) bool {
	for _, c := range checks {
		if !c.OK {
			return false
		}
	}
	return true
}

// gatewayCollisionCheck builds the gateway-collision Check from the pre-filter
// candidate list and the post-filter viable list. A NIC whose IP equals its
// subnet gateway (.1) is a collision — advertising the router's own IP as
// next-server is exactly what caused the reprovision postmortem. Collisions
// are ALWAYS listed in Detail so the operator sees the router-IP situation,
// but a collision is only fatal (OK=false) when it leaves zero viable NICs.
// Pure; unit-tested.
func gatewayCollisionCheck(candidates, filtered []NetworkInfo) Check {
	c := Check{Name: "gateway-collision"}
	var collisions []string
	for _, n := range candidates {
		gw := gatewayForIP(n.IP)
		if gw != "" && n.IP == gw {
			collisions = append(collisions, fmt.Sprintf("%s=%s (gateway %s)", n.Interface, n.IP, gw))
		}
	}
	if len(collisions) == 0 {
		c.OK = true
		c.Detail = fmt.Sprintf("no interface advertises its router IP (.1) as next-server (%d candidate interface(s))", len(candidates))
		return c
	}
	c.Detail = "interface(s) whose IP equals their subnet gateway (.1), excluded from PXE: " + strings.Join(collisions, ", ")
	if len(filtered) > 0 {
		c.OK = true
		c.Detail += fmt.Sprintf("; %d viable NIC(s) remain", len(filtered))
	} else {
		c.OK = false
		c.Detail += "; no viable NIC remains after exclusion"
		c.Hint = "the only interface(s) advertise the router's own IP; connect a wired LAN whose host IP is not the .1 gateway, or pass --iface"
	}
	return c
}

// assetsCheck verifies ipxe.efi and boot.ipxe exist under p.Assets().
func assetsCheck(p paths.Paths) Check {
	c := Check{Name: "assets"}
	var missing []string
	for _, req := range []string{"ipxe.efi", "boot.ipxe"} {
		if _, err := os.Stat(filepath.Join(p.Assets(), req)); err != nil {
			missing = append(missing, req)
		}
	}
	if len(missing) == 0 {
		c.OK = true
		c.Detail = "ipxe.efi and boot.ipxe present under " + p.Assets()
		return c
	}
	c.OK = false
	c.Detail = fmt.Sprintf("missing %s under %s", strings.Join(missing, ", "), p.Assets())
	c.Hint = "run `nostos build`"
	return c
}

// dnsmasqCheck verifies the dnsmasq binary is resolvable.
func dnsmasqCheck() Check {
	c := Check{Name: "dnsmasq"}
	if dnsmasqAvailable() {
		c.OK = true
		c.Detail = "dnsmasq found at " + dnsmasqBinary()
		return c
	}
	c.OK = false
	c.Detail = "dnsmasq not found on PATH or /opt/homebrew/sbin"
	c.Hint = "brew install dnsmasq"
	return c
}

// interfacesCheck reports the viable NICs detectNetworks() found.
func interfacesCheck(nets []NetworkInfo, err error) Check {
	c := Check{Name: "interfaces"}
	if err != nil {
		c.OK = false
		c.Detail = "interface detection failed: " + err.Error()
		c.Hint = "no LAN-viable interface; connect wired ethernet"
		return c
	}
	if len(nets) == 0 {
		c.OK = false
		c.Detail = "no LAN-viable interface after gateway-collision filter"
		c.Hint = "no LAN-viable interface; connect wired ethernet"
		return c
	}
	parts := make([]string, 0, len(nets))
	for _, n := range nets {
		parts = append(parts, n.Interface+"="+n.IP)
	}
	c.OK = true
	c.Detail = fmt.Sprintf("%d viable NIC(s): %s", len(nets), strings.Join(parts, ", "))
	return c
}

// httpPortCheck attempts to bind the HTTP port and closes immediately.
func httpPortCheck(port int) Check {
	c := Check{Name: "http-port"}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		c.OK = false
		c.Detail = fmt.Sprintf("cannot bind HTTP port %d: %v", port, err)
		c.Hint = "another process holds the HTTP port; stop it or pass --port"
		return c
	}
	_ = ln.Close()
	c.OK = true
	c.Detail = fmt.Sprintf("HTTP port %d is bindable", port)
	return c
}

// httpSelfTestFetch starts a throwaway file server over assetsDir on an
// ephemeral loopback port, GETs /boot.ipxe, and returns the status, body, and
// any error. Root-free and hermetic — the smallest factored helper so the
// HTTP-serving path can be unit-tested with a t.TempDir() asset dir.
func httpSelfTestFetch(assetsDir string) (int, []byte, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	srv := &http.Server{Handler: http.FileServer(http.Dir(assetsDir))}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	url := fmt.Sprintf("http://%s/boot.ipxe", ln.Addr().String())
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// httpSelfTestCheck verifies the asset-serving path end-to-end without root.
func httpSelfTestCheck(assetsDir string) Check {
	c := Check{Name: "http-self-test"}
	code, body, err := httpSelfTestFetch(assetsDir)
	if err != nil {
		c.OK = false
		c.Detail = "self-test fetch of /boot.ipxe failed: " + err.Error()
		c.Hint = "ensure assets exist (`nostos build`)"
		return c
	}
	if code != http.StatusOK || len(body) == 0 {
		c.OK = false
		c.Detail = fmt.Sprintf("self-test GET /boot.ipxe returned HTTP %d (%d bytes)", code, len(body))
		c.Hint = "ensure assets exist (`nostos build`)"
		return c
	}
	c.OK = true
	c.Detail = fmt.Sprintf("served /boot.ipxe over an ephemeral port (HTTP %d, %d bytes)", code, len(body))
	return c
}

// sudoersCheck reports whether the scoped sudoers drop-in is installed so
// dnsmasq can bind privileged ports (67/4011/69) without a prompt. It does NOT
// attempt to bind those ports (that needs root).
func sudoersCheck() Check {
	c := Check{Name: "privileged-ports"}
	if SudoersInstalled() {
		c.OK = true
		c.Detail = "sudoers drop-in present at " + SudoersDropInPath + "; dnsmasq runs sudo-less"
		return c
	}
	c.OK = false
	c.Detail = "no sudoers drop-in at " + SudoersDropInPath + "; dnsmasq would prompt for a password to bind 67/4011/69"
	c.Hint = "run `nostos pxe setup` so the PXE server can bind 67/69 without an interactive sudo prompt"
	return c
}
