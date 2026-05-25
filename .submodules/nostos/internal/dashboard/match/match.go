// Package match buckets discovered devices into known/orphan/unknown
// against the operator's nostos config and applies hidden-device filtering.
package match

import (
	"sort"
	"strings"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/dashboard/discovery"
	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
)

// Buckets is the matcher output.
type Buckets struct {
	Known   []snapshot.Node
	Orphan  []snapshot.Discovery
	Unknown []snapshot.Discovery
}

// Bind binds discovery results to configured nodes.
//
// Priority: MAC > IP > Tailscale-100.x address.
//
// Each device falls into exactly one of:
//   - known  — matches a configured node by any priority
//   - orphan — configured node but no matching live device
//   - unknown — live device with no config entry
func Bind(cfg *config.Config, res discovery.Result, hiddenMACs map[string]bool, showHidden bool) Buckets {
	b := Buckets{}

	type liveKey struct{ kind, val string }
	live := map[liveKey]discovery.Device{}
	for _, d := range res.Devices {
		if d.MAC != "" {
			live[liveKey{"mac", strings.ToLower(d.MAC)}] = d
		}
		if d.IP != "" {
			live[liveKey{"ip", d.IP}] = d
		}
		if d.Tailscale != "" {
			live[liveKey{"ts", d.Tailscale}] = d
		}
	}

	// Walk configured nodes to build known/orphan.
	configured := map[string]bool{} // matched device IDs
	names := make([]string, 0, len(cfg.Nodes))
	for n := range cfg.Nodes {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, name := range names {
		n := cfg.Nodes[name]
		var matched *discovery.Device
		if d, ok := live[liveKey{"mac", strings.ToLower(n.MAC)}]; ok && n.MAC != "" {
			matched = &d
		} else if d, ok := live[liveKey{"ip", n.IP}]; ok {
			matched = &d
		}
		if matched != nil {
			configured[matched.ProbeID+"|"+matched.IP+"|"+matched.MAC] = true
			b.Known = append(b.Known, snapshot.Node{
				Name:        name,
				IP:          n.IP,
				Role:        n.Role,
				Arch:        n.Arch,
				Severity:    snapshot.SevInfo,
				Bucket:      "known",
				SchematicID: n.EffectiveSchematic(cfg.Cluster),
			})
		} else {
			b.Orphan = append(b.Orphan, snapshot.Discovery{
				IP:           n.IP,
				MAC:          n.MAC,
				Hostname:     name,
				DiscoveredAt: res.StartedAt,
				ProbeID:      "orphan",
				Bucket:       "orphan",
			})
		}
	}

	// Devices the matcher didn't bind to any node => unknown.
	seen := map[string]bool{}
	for _, d := range res.Devices {
		// Skip devices that bound to a known node.
		if isBoundToConfig(cfg, d) {
			continue
		}
		if d.MAC != "" && hiddenMACs[strings.ToLower(d.MAC)] && !showHidden {
			continue
		}
		key := strings.ToLower(d.MAC) + "|" + d.IP
		if seen[key] {
			continue
		}
		seen[key] = true
		d.Bucket = "unknown"
		b.Unknown = append(b.Unknown, d)
	}
	return b
}

func isBoundToConfig(cfg *config.Config, d discovery.Device) bool {
	for _, n := range cfg.Nodes {
		if n.MAC != "" && strings.EqualFold(n.MAC, d.MAC) {
			return true
		}
		if n.IP != "" && n.IP == d.IP {
			return true
		}
	}
	return false
}
