package pxe

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Phase is one stage of a node's PXE boot lifecycle. The constants are
// declared in lifecycle order; phaseOrdinal maps each to a monotonically
// increasing ordinal so "latest phase" can be computed as the furthest phase
// observed for a node.
type Phase string

const (
	// PhaseUnknown is the zero value: an event whose path/line we could not
	// classify, or a node for which nothing has been observed yet.
	PhaseUnknown Phase = ""

	PhaseDiscover    Phase = "discover"    // DHCP DISCOVER/REQUEST seen
	PhaseTFTP        Phase = "tftp"        // firmware/iPXE chainload (ipxe.efi / boot.ipxe)
	PhaseKernel      Phase = "kernel"      // vmlinuz fetched
	PhaseInitramfs   Phase = "initramfs"   // initramfs fetched
	PhaseConfig      Phase = "config"      // machineconfig fetched (/configs/<mac>.yaml)
	PhaseMaintenance Phase = "maintenance" // node in Talos maintenance mode
	PhaseApid        Phase = "apid"        // apid reachable (TCP:50000)
	PhaseBootstrap   Phase = "bootstrap"   // etcd bootstrap in progress
	PhaseReady       Phase = "ready"       // node Ready
)

// phaseOrder defines the lifecycle ordering. Index = ordinal; higher means
// further along. PhaseUnknown is implicitly ordinal -1 (not in the slice).
var phaseOrder = []Phase{
	PhaseDiscover,
	PhaseTFTP,
	PhaseKernel,
	PhaseInitramfs,
	PhaseConfig,
	PhaseMaintenance,
	PhaseApid,
	PhaseBootstrap,
	PhaseReady,
}

// phaseOrdinal returns the lifecycle ordinal of p. PhaseUnknown (and any
// unrecognized value) returns -1 so it never wins a "furthest phase" compare.
func phaseOrdinal(p Phase) int {
	for i, ph := range phaseOrder {
		if ph == p {
			return i
		}
	}
	return -1
}

// Event is a single observed lifecycle datapoint. It is appended to the event
// stream as one NDJSON line.
type Event struct {
	Timestamp time.Time `json:"ts"`
	MAC       string    `json:"mac,omitempty"`
	IP        string    `json:"ip,omitempty"`
	Interface string    `json:"interface,omitempty"`
	Phase     Phase     `json:"phase"`
	Message   string    `json:"message,omitempty"`
}

// EventStore appends events as NDJSON to a file under Paths.State() and reads
// them back. It is safe for concurrent use.
type EventStore struct {
	mu   sync.Mutex
	path string
}

// EventsFile is the on-disk location of the event stream relative to State().
const EventsFile = "pxe/events.ndjson"

// NewEventStore returns a store writing to <stateDir>/pxe/events.ndjson.
func NewEventStore(stateDir string) *EventStore {
	return &EventStore{path: filepath.Join(stateDir, "pxe", "events.ndjson")}
}

// Path returns the backing file path.
func (s *EventStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Append writes one event as an NDJSON line, creating the parent dir on first
// write. A nil store is a no-op (safe to call when recording is disabled).
func (s *EventStore) Append(ev Event) error {
	if s == nil {
		return nil
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}

// Load reads the whole event stream back. A missing file yields an empty
// slice (not an error). A nil store yields an empty slice.
func (s *EventStore) Load() ([]Event, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Event{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // tolerate a partially-written / corrupt tail line
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	return out, nil
}

// configPathRE matches the served machineconfig path and captures the
// hyphen-form MAC: /configs/d0-94-66-d9-eb-a5.yaml
var configPathRE = regexp.MustCompile(`/configs/([0-9a-fA-F]{2}(?:-[0-9a-fA-F]{2}){5})\.yaml$`)

// ClassifyHTTPPath maps a served HTTP request path to a lifecycle phase and,
// when the path carries it, the node MAC (hyphen form). It is pure.
//
//   - boot.ipxe        -> tftp     (iPXE chainload script)
//   - *vmlinuz*        -> kernel
//   - *initramfs*      -> initramfs
//   - /configs/<mac>.yaml -> config, and the <mac> is extracted
//   - anything else    -> unknown, ""
func ClassifyHTTPPath(path string) (Phase, string) {
	if m := configPathRE.FindStringSubmatch(path); m != nil {
		return PhaseConfig, strings.ToLower(m[1])
	}
	switch {
	case strings.HasSuffix(path, "/boot.ipxe") || strings.HasSuffix(path, "boot.ipxe"):
		return PhaseTFTP, ""
	case strings.Contains(path, "vmlinuz"):
		return PhaseKernel, ""
	case strings.Contains(path, "initramfs"):
		return PhaseInitramfs, ""
	default:
		return PhaseUnknown, ""
	}
}

// NodeState is the folded view of a single node (keyed by MAC, or by IP when
// no MAC correlation exists yet).
type NodeState struct {
	MAC       string    `json:"mac,omitempty"`
	IP        string    `json:"ip,omitempty"`
	Interface string    `json:"interface,omitempty"`
	Phase     Phase     `json:"phase"`
	LastSeen  time.Time `json:"last_seen"`
	Known     bool      `json:"known"`
}

// macRE / ipRE help normalize dnsmasq tokens.
var (
	dnsmasqMACRE = regexp.MustCompile(`([0-9a-fA-F]{2}(?::[0-9a-fA-F]{2}){5})`)
	dnsmasqIPRE  = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)
	// dnsmasqIfaceRE captures the arrival interface dnsmasq names in parens
	// after the DHCP message keyword, e.g. DHCPDISCOVER(en5) / DHCPACK(en5).
	dnsmasqIfaceRE = regexp.MustCompile(`DHCP\w+\(([A-Za-z0-9._-]+)\)`)
)

// macHyphen normalizes a colon- or hyphen-form MAC to lowercase hyphen form.
func macHyphen(mac string) string {
	return strings.ToLower(strings.ReplaceAll(mac, ":", "-"))
}

// ParseDnsmasqLine extracts a lifecycle Event from a single dnsmasq --log-dhcp
// line. It recognizes:
//
//   - DHCPDISCOVER / DHCPREQUEST -> discover (MAC, maybe IP)
//   - DHCPACK                    -> tftp     (ties IP <-> MAC: the correlation source)
//   - "sent .../ipxe.efi ..."    -> tftp     (firmware delivered over TFTP)
//
// Returns ok=false for any line it does not recognize. It is pure.
func ParseDnsmasqLine(line string) (Event, bool) {
	l := strings.TrimSpace(line)
	if l == "" {
		return Event{}, false
	}
	mac := ""
	if m := dnsmasqMACRE.FindString(l); m != "" {
		mac = macHyphen(m)
	}
	ip := ""
	if m := dnsmasqIPRE.FindString(l); m != "" {
		ip = m
	}
	iface := ""
	if m := dnsmasqIfaceRE.FindStringSubmatch(l); m != nil {
		iface = m[1]
	}

	switch {
	case strings.Contains(l, "DHCPDISCOVER"), strings.Contains(l, "DHCPREQUEST"):
		if mac == "" {
			return Event{}, false
		}
		return Event{MAC: mac, Interface: iface, Phase: PhaseDiscover, Message: "dhcp discover/request"}, true
	case strings.Contains(l, "DHCPACK"):
		// DHCPACK is the IP<->MAC binding event; both should be present.
		if mac == "" && ip == "" {
			return Event{}, false
		}
		return Event{MAC: mac, IP: ip, Interface: iface, Phase: PhaseTFTP, Message: "dhcp ack (ip<->mac bound)"}, true
	case strings.Contains(l, "sent ") && strings.Contains(l, "ipxe.efi"):
		// TFTP firmware delivery. MAC/IP usually absent on this line.
		return Event{MAC: mac, IP: ip, Interface: iface, Phase: PhaseTFTP, Message: "tftp sent ipxe.efi"}, true
	default:
		return Event{}, false
	}
}

// FoldState reduces an ordered event stream into per-node state. Events are
// keyed by MAC when known, otherwise by IP. A DHCPACK (Phase tftp carrying
// both IP and MAC) establishes an IP<->MAC binding which is then used to
// relabel IP-only HTTP events (kernel/initramfs) onto the owning MAC.
//
// The returned map is keyed by MAC for correlated nodes and by IP (with a
// leading "ip:" sentinel) for events that never got correlated.
func FoldState(events []Event) map[string]*NodeState {
	// First pass: build IP->MAC bindings from DHCPACK events.
	ipToMAC := map[string]string{}
	for _, ev := range events {
		if ev.Phase == PhaseTFTP && ev.MAC != "" && ev.IP != "" {
			ipToMAC[ev.IP] = ev.MAC
		}
	}

	states := map[string]*NodeState{}

	// resolveKey returns the canonical store key + the MAC/IP to record.
	get := func(key string) *NodeState {
		st := states[key]
		if st == nil {
			st = &NodeState{}
			states[key] = st
		}
		return st
	}

	for _, ev := range events {
		mac := ev.MAC
		ip := ev.IP
		// Correlate IP-only events to a MAC if we learned the binding.
		if mac == "" && ip != "" {
			if bound, ok := ipToMAC[ip]; ok {
				mac = bound
			}
		}

		var st *NodeState
		if mac != "" {
			st = get(mac)
			st.MAC = mac
			if ip != "" {
				st.IP = ip
			} else if known, ok := bindingIP(ipToMAC, mac); ok {
				st.IP = known
			}
		} else if ip != "" {
			st = get("ip:" + ip)
			st.IP = ip
		} else {
			continue
		}

		if phaseOrdinal(ev.Phase) > phaseOrdinal(st.Phase) {
			st.Phase = ev.Phase
		}
		if ev.Interface != "" {
			st.Interface = ev.Interface
		}
		if ev.Timestamp.After(st.LastSeen) {
			st.LastSeen = ev.Timestamp
		}
	}

	// Merge any IP-only buckets whose IP we can now bind to a MAC. This covers
	// the case where the kernel/initramfs IP events were processed before the
	// DHCPACK relabeling could apply (defensive; the single pass above already
	// relabels using the fully-built ipToMAC map).
	for ip, mac := range ipToMAC {
		ipKey := "ip:" + ip
		ipState, ok := states[ipKey]
		if !ok {
			continue
		}
		macState := get(mac)
		macState.MAC = mac
		if macState.IP == "" {
			macState.IP = ip
		}
		if phaseOrdinal(ipState.Phase) > phaseOrdinal(macState.Phase) {
			macState.Phase = ipState.Phase
		}
		if macState.Interface == "" && ipState.Interface != "" {
			macState.Interface = ipState.Interface
		}
		if ipState.LastSeen.After(macState.LastSeen) {
			macState.LastSeen = ipState.LastSeen
		}
		delete(states, ipKey)
	}

	return states
}

// bindingIP returns the IP bound to mac (reverse lookup over ipToMAC), if any.
func bindingIP(ipToMAC map[string]string, mac string) (string, bool) {
	for ip, m := range ipToMAC {
		if m == mac {
			return ip, true
		}
	}
	return "", false
}
