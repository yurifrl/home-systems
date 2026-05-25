// Package discovery probes the local network and external services to
// surface devices that the dashboard's matcher and TUI render.
//
// v0.3 scope is intentionally minimal:
//   - Talos maintenance API probe (TCP 50000) on every configured node IP
//   - ARP-table lookup via `arp -an` (no /24 sweep — read what the OS already knows)
//   - BMC probe scoped to the operator-configured turingpi host(s)
//
// Out of scope (deferred or stubbed):
//   - mDNS/zeroconf scan: stubbed (returns nil); see hosts via OS arp instead
//   - Full ICMP fan-out across /24: skipped (privileged ICMP requires raw sockets)
//   - Tailscale device list: returned via TailscaleDevices below if backend wired;
//     in v0.3 this is a structural placeholder.
package discovery

import (
	"context"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
)

// Device is the raw output of any probe.
type Device = snapshot.Discovery

// Result aggregates the per-pass probe output.
type Result struct {
	Devices    []Device
	BMCs       []Device
	Tailscale  []TailscaleDevice
	StartedAt  time.Time
	FinishedAt time.Time
}

// TailscaleDevice mirrors the API shape we care about.
type TailscaleDevice struct {
	Name    string `json:"name"`
	Addr    string `json:"addr"`
	Online  bool   `json:"online"`
}

// Run executes all configured probes in parallel and returns the merged Result.
// Every sub-probe is context-cancellable.
func Run(ctx context.Context, cfg *config.Config) Result {
	r := Result{StartedAt: time.Now()}
	var mu sync.Mutex
	var wg sync.WaitGroup

	add := func(dev Device) {
		mu.Lock()
		r.Devices = append(r.Devices, dev)
		mu.Unlock()
	}
	addBMC := func(dev Device) {
		mu.Lock()
		r.BMCs = append(r.BMCs, dev)
		mu.Unlock()
	}

	// Talos maintenance probe on every configured node IP.
	wg.Add(1)
	go func() {
		defer wg.Done()
		probeTalosFromConfig(ctx, cfg, add)
	}()

	// ARP table snapshot.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, dev := range arpTable(ctx) {
			add(dev)
		}
	}()

	// BMC probe scoped to configured tpi.host values.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, dev := range probeBMCs(ctx, cfg) {
			addBMC(dev)
		}
	}()

	// mDNS multicast probe — best effort; merged into Devices via IP key.
	var mdnsHits []Device
	if hasIPv4Multicast() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { recover() }()
			mdnsHits = mdnsScan(ctx)
		}()
	}

	wg.Wait()
	if len(mdnsHits) > 0 {
		r.Devices = mergeMDNS(r.Devices, mdnsHits)
	}
	r.FinishedAt = time.Now()
	return r
}

// probeTalos opens a TCP connection to host:50000 and returns true if reachable.
func probeTalos(ctx context.Context, host string, timeout time.Duration) bool {
	d := net.Dialer{Timeout: timeout}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := d.DialContext(cctx, "tcp", net.JoinHostPort(host, "50000"))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func probeTalosFromConfig(ctx context.Context, cfg *config.Config, emit func(Device)) {
	type job struct{ name, ip string }
	jobs := make(chan job, 32)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				ok := probeTalos(ctx, j.ip, 200*time.Millisecond)
				if !ok {
					continue
				}
				emit(Device{
					IP:           j.ip,
					Hostname:     j.name,
					TalosMaint:   true,
					DiscoveredAt: time.Now(),
					ProbeID:      "talos-maintenance",
					Bucket:       "known",
				})
			}
		}()
	}
	for name, n := range cfg.Nodes {
		select {
		case <-ctx.Done():
			break
		default:
		}
		jobs <- job{name, n.IP}
	}
	close(jobs)
	wg.Wait()
}

var arpRE = regexp.MustCompile(`\(([0-9.]+)\) at ([0-9a-f:]+)`)

// arpTable shells out to `arp -an` and parses IP/MAC pairs. Best-effort;
// returns empty if the binary is absent.
func arpTable(ctx context.Context) []Device {
	if _, err := exec.LookPath("arp"); err != nil {
		return nil
	}
	cctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(cctx, "arp", "-an").Output()
	if err != nil {
		return nil
	}
	now := time.Now()
	var devices []Device
	for _, line := range strings.Split(string(out), "\n") {
		m := arpRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		devices = append(devices, Device{
			IP:           m[1],
			MAC:          strings.ToLower(m[2]),
			DiscoveredAt: now,
			ProbeID:      "arp",
		})
	}
	return devices
}

// probeBMCs probes the configured turingpi BMC host(s) (tpi method only) on
// the standard Redfish/HTTP ports. Probes are deduped per host.
func probeBMCs(ctx context.Context, cfg *config.Config) []Device {
	seen := map[string]bool{}
	var out []Device
	for _, n := range cfg.Nodes {
		if n.Boot.TPI == nil {
			continue
		}
		host := n.Boot.TPI.Host
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		// Try resolving + a quick TCP touch on 80/443 to detect presence.
		ip := host
		if addrs, err := net.DefaultResolver.LookupHost(ctx, host); err == nil && len(addrs) > 0 {
			ip = addrs[0]
		}
		ok := probeTCP(ctx, host, "80", 500*time.Millisecond) ||
			probeTCP(ctx, host, "443", 500*time.Millisecond)
		role := "tpi"
		if !ok {
			role = "tpi (unreachable)"
		}
		out = append(out, Device{
			IP:           ip,
			Hostname:     host,
			BMCRole:      role,
			DiscoveredAt: time.Now(),
			ProbeID:      "bmc-redfish",
		})
	}
	return out
}

func probeTCP(ctx context.Context, host, port string, timeout time.Duration) bool {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(cctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
