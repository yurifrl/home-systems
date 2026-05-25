package discovery

import (
	"context"
	"io"
	stdlog "log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"

	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
)

// mdnsServices is the list of services we probe on the LAN. Workstations
// (Linux/macOS), Windows file shares, Apple devices, and printers cover the
// majority of "what is on the operator's home network".
var mdnsServices = []string{
	"_workstation._tcp",
	"_smb._tcp",
}

// mdnsTimeout is the per-service probe timeout.
var mdnsTimeout = 2 * time.Second

// mdnsScan probes the LAN via multicast DNS and emits Devices. Returns nil
// on any error (best-effort: many home networks block multicast).
func mdnsScan(ctx context.Context) []Device {
	var (
		mu  sync.Mutex
		out []Device
	)
	emit := func(d Device) {
		mu.Lock()
		defer mu.Unlock()
		out = append(out, d)
	}
	var wg sync.WaitGroup
	for _, svc := range mdnsServices {
		wg.Add(1)
		go func(svc string) {
			defer wg.Done()
			defer func() { recover() }() // hashicorp/mdns can panic on goroutine teardown
			entries := make(chan *mdns.ServiceEntry, 16)
			done := make(chan struct{})
			go func() {
				for e := range entries {
					if e == nil {
						continue
					}
					ip := ""
					if e.AddrV4 != nil {
						ip = e.AddrV4.String()
					} else if e.AddrV6 != nil {
						ip = e.AddrV6.String()
					}
					if ip == "" {
						continue
					}
					emit(Device{
						IP:           ip,
						Hostname:     strings.TrimSuffix(e.Host, "."),
						DiscoveredAt: time.Now(),
						ProbeID:      "mdns:" + svc,
					})
				}
				close(done)
			}()
			params := mdns.DefaultParams(svc)
			params.Entries = entries
			params.Timeout = mdnsTimeout
			params.DisableIPv6 = true
			params.WantUnicastResponse = false
			params.Logger = stdlog.New(io.Discard, "", 0)
			_ = mdns.Query(params)
			close(entries)
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
			}
		}(svc)
	}
	doneAll := make(chan struct{})
	go func() { wg.Wait(); close(doneAll) }()
	select {
	case <-doneAll:
	case <-ctx.Done():
	case <-time.After(mdnsTimeout + time.Second):
	}
	return out
}

// merge folds mdns devices into existing ARP/talos hits keyed by IP.
// New IPs are appended; matching IPs gain a Hostname when missing.
func mergeMDNS(existing []Device, mdnsHits []Device) []Device {
	byIP := map[string]int{}
	for i, d := range existing {
		byIP[d.IP] = i
	}
	for _, d := range mdnsHits {
		if idx, ok := byIP[d.IP]; ok {
			if existing[idx].Hostname == "" {
				existing[idx].Hostname = d.Hostname
			}
			continue
		}
		existing = append(existing, d)
		byIP[d.IP] = len(existing) - 1
	}
	return existing
}

// hasIPv4Multicast returns true if at least one non-loopback interface is up
// with a v4 address — a heuristic for "we can reasonably try mDNS".
func hasIPv4Multicast() bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				return true
			}
		}
	}
	return false
}

// ensure we use snapshot import even when this file is the only consumer.
var _ = snapshot.SevInfo
