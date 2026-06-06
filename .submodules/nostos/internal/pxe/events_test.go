package pxe

import (
	"path/filepath"
	"testing"
	"time"
)

func TestClassifyHTTPPath(t *testing.T) {
	tests := []struct {
		path      string
		wantPhase Phase
		wantMAC   string
	}{
		{"/assets/boot.ipxe", PhaseTFTP, ""},
		{"/boot.ipxe", PhaseTFTP, ""},
		{"/assets/amd64/vmlinuz", PhaseKernel, ""},
		{"/assets/initramfs-amd64.xz", PhaseInitramfs, ""},
		{"/configs/de-ad-be-ef-00-01.yaml", PhaseConfig, "de-ad-be-ef-00-01"},
		{"/configs/D0-94-66-D9-EB-A5.yaml", PhaseConfig, "d0-94-66-d9-eb-a5"},
		{"/favicon.ico", PhaseUnknown, ""},
		{"/", PhaseUnknown, ""},
	}
	for _, tt := range tests {
		gotPhase, gotMAC := ClassifyHTTPPath(tt.path)
		if gotPhase != tt.wantPhase || gotMAC != tt.wantMAC {
			t.Errorf("ClassifyHTTPPath(%q) = (%q, %q), want (%q, %q)",
				tt.path, gotPhase, gotMAC, tt.wantPhase, tt.wantMAC)
		}
	}
}

func TestParseDnsmasqLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantOK    bool
		wantPhase Phase
		wantMAC   string
		wantIP    string
		wantIface string
	}{
		{
			name:      "discover",
			line:      "dnsmasq-dhcp: DHCPDISCOVER(en5) 11:22:33:44:55:66",
			wantOK:    true,
			wantPhase: PhaseDiscover,
			wantMAC:   "11-22-33-44-55-66",
			wantIface: "en5",
		},
		{
			name:      "request",
			line:      "dnsmasq-dhcp: DHCPREQUEST(en5) 192.168.68.205 11:22:33:44:55:66",
			wantOK:    true,
			wantPhase: PhaseDiscover,
			wantMAC:   "11-22-33-44-55-66",
			wantIface: "en5",
		},
		{
			name:      "ack ties ip<->mac",
			line:      "dnsmasq-dhcp: DHCPACK(en5) 192.168.68.205 11:22:33:44:55:66",
			wantOK:    true,
			wantPhase: PhaseTFTP,
			wantMAC:   "11-22-33-44-55-66",
			wantIP:    "192.168.68.205",
			wantIface: "en5",
		},
		{
			name:      "sent ipxe.efi",
			line:      "dnsmasq-tftp: sent /tmp/nostos-tftp/ipxe.efi to 192.168.68.205",
			wantOK:    true,
			wantPhase: PhaseTFTP,
			wantIP:    "192.168.68.205",
		},
		{
			name:   "junk",
			line:   "dnsmasq: started, version 2.90 cachesize 150",
			wantOK: false,
		},
		{
			name:   "empty",
			line:   "   ",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev, ok := ParseDnsmasqLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (ev=%+v)", ok, tt.wantOK, ev)
			}
			if !ok {
				return
			}
			if ev.Phase != tt.wantPhase {
				t.Errorf("phase = %q, want %q", ev.Phase, tt.wantPhase)
			}
			if ev.MAC != tt.wantMAC {
				t.Errorf("mac = %q, want %q", ev.MAC, tt.wantMAC)
			}
			if ev.IP != tt.wantIP {
				t.Errorf("ip = %q, want %q", ev.IP, tt.wantIP)
			}
			if ev.Interface != tt.wantIface {
				t.Errorf("interface = %q, want %q", ev.Interface, tt.wantIface)
			}
		})
	}
}

func TestFoldStateCorrelatesIPToMAC(t *testing.T) {
	base := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	mac := "de-ad-be-ef-00-01"
	ip := "192.168.68.205"
	events := []Event{
		{Timestamp: base.Add(0 * time.Second), MAC: mac, Interface: "en5", Phase: PhaseDiscover},
		{Timestamp: base.Add(1 * time.Second), MAC: mac, IP: ip, Interface: "en5", Phase: PhaseTFTP}, // DHCPACK binding
		{Timestamp: base.Add(2 * time.Second), IP: ip, Phase: PhaseKernel},                           // IP-only
		{Timestamp: base.Add(3 * time.Second), IP: ip, Phase: PhaseInitramfs},                        // IP-only
		{Timestamp: base.Add(4 * time.Second), MAC: mac, Phase: PhaseConfig},                         // by MAC
	}

	states := FoldState(events)

	// Exactly one node, keyed by MAC; no leftover ip: bucket.
	if len(states) != 1 {
		t.Fatalf("expected 1 folded state, got %d: %+v", len(states), states)
	}
	st, ok := states[mac]
	if !ok {
		t.Fatalf("no state for MAC %q; got keys %v", mac, keysOf(states))
	}
	if st.MAC != mac {
		t.Errorf("MAC = %q, want %q", st.MAC, mac)
	}
	if st.IP != ip {
		t.Errorf("IP = %q, want %q (IP-keyed events should correlate to MAC)", st.IP, ip)
	}
	if st.Interface != "en5" {
		t.Errorf("interface = %q, want %q (arrival interface should propagate)", st.Interface, "en5")
	}
	// Furthest phase observed is config.
	if st.Phase != PhaseConfig {
		t.Errorf("phase = %q, want %q", st.Phase, PhaseConfig)
	}
	if !st.LastSeen.Equal(base.Add(4 * time.Second)) {
		t.Errorf("last seen = %v, want %v", st.LastSeen, base.Add(4*time.Second))
	}
}

func TestFoldStateUncorrelatedIPOnly(t *testing.T) {
	// An IP-only event with no DHCPACK binding stays in its own ip: bucket.
	events := []Event{
		{IP: "10.0.0.9", Phase: PhaseKernel},
	}
	states := FoldState(events)
	st, ok := states["ip:10.0.0.9"]
	if !ok {
		t.Fatalf("expected ip: bucket, got keys %v", keysOf(states))
	}
	if st.Phase != PhaseKernel || st.IP != "10.0.0.9" || st.MAC != "" {
		t.Errorf("unexpected state %+v", st)
	}
}

func TestEventStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewEventStore(dir)

	// Missing file -> empty, no error.
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty load, got %d", len(got))
	}

	want := []Event{
		{Timestamp: time.Date(2026, 6, 6, 1, 0, 0, 0, time.UTC), MAC: "de-ad-be-ef-00-01", Phase: PhaseDiscover, Message: "a"},
		{Timestamp: time.Date(2026, 6, 6, 1, 0, 1, 0, time.UTC), MAC: "de-ad-be-ef-00-01", IP: "192.168.68.205", Phase: PhaseTFTP, Message: "b"},
		{Timestamp: time.Date(2026, 6, 6, 1, 0, 2, 0, time.UTC), IP: "192.168.68.205", Phase: PhaseKernel},
	}
	for _, ev := range want {
		if err := store.Append(ev); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	// File lives under <dir>/pxe/events.ndjson.
	if store.Path() != filepath.Join(dir, "pxe", "events.ndjson") {
		t.Errorf("unexpected path %q", store.Path())
	}

	got, err = store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("loaded %d events, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].MAC != want[i].MAC || got[i].IP != want[i].IP ||
			got[i].Phase != want[i].Phase || got[i].Message != want[i].Message ||
			!got[i].Timestamp.Equal(want[i].Timestamp) {
			t.Errorf("event %d round-trip mismatch:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
}

func TestEventStoreNilSafe(t *testing.T) {
	var s *EventStore
	if err := s.Append(Event{Phase: PhaseReady}); err != nil {
		t.Errorf("nil Append: %v", err)
	}
	got, err := s.Load()
	if err != nil || len(got) != 0 {
		t.Errorf("nil Load = (%v, %v)", got, err)
	}
	if s.Path() != "" {
		t.Errorf("nil Path = %q", s.Path())
	}
}

func keysOf(m map[string]*NodeState) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
