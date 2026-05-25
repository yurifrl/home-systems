package discovery

import (
	"testing"
	"time"
)

func TestMergeMDNSAddsHostnameToExistingIP(t *testing.T) {
	existing := []Device{
		{IP: "192.168.1.5", MAC: "aa:bb:cc:dd:ee:01", ProbeID: "arp", DiscoveredAt: time.Now()},
	}
	mdnsHits := []Device{
		{IP: "192.168.1.5", Hostname: "printer.local", ProbeID: "mdns:_workstation._tcp"},
		{IP: "192.168.1.99", Hostname: "newbox.local", ProbeID: "mdns:_smb._tcp"},
	}
	out := mergeMDNS(existing, mdnsHits)
	if len(out) != 2 {
		t.Fatalf("len=%d want 2", len(out))
	}
	if out[0].Hostname != "printer.local" {
		t.Fatalf("hostname not merged onto existing IP: %+v", out[0])
	}
	if out[0].ProbeID != "arp" {
		t.Fatalf("probe id should be preserved on existing entry: %q", out[0].ProbeID)
	}
	if out[1].IP != "192.168.1.99" || out[1].Hostname != "newbox.local" {
		t.Fatalf("new mdns row not appended: %+v", out[1])
	}
}

func TestMergeMDNSDoesNotOverwriteHostname(t *testing.T) {
	existing := []Device{{IP: "10.0.0.1", Hostname: "configured", ProbeID: "talos-maintenance"}}
	out := mergeMDNS(existing, []Device{{IP: "10.0.0.1", Hostname: "from-mdns"}})
	if out[0].Hostname != "configured" {
		t.Fatalf("merge clobbered existing hostname: %q", out[0].Hostname)
	}
}

func TestHasIPv4MulticastDoesNotPanic(t *testing.T) {
	_ = hasIPv4Multicast()
}
