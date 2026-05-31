// Package upgrade computes and executes safe rolling Talos OS upgrades.
//
// plan.go holds the PURE planning logic: version parsing, adjacent-minor path
// computation, and node ordering. These functions have no side effects (no
// network, no exec) so they are trivially unit-testable. The network/exec
// pieces live in catalog.go and detect.go.
package upgrade

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Version is a parsed semantic-ish Talos version (major.minor.patch).
type Version struct {
	Major int
	Minor int
	Patch int
}

// String renders the version in canonical "vMAJOR.MINOR.PATCH" form.
func (v Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Less reports whether v sorts before o.
func (v Version) Less(o Version) bool {
	if v.Major != o.Major {
		return v.Major < o.Major
	}
	if v.Minor != o.Minor {
		return v.Minor < o.Minor
	}
	return v.Patch < o.Patch
}

// ParseVersion parses "v1.10.3" or "1.10.3" into a Version. A leading "v" is
// optional. Exactly three dot-separated numeric components are required.
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimSpace(s)
	raw = strings.TrimPrefix(raw, "v")
	if raw == "" {
		return Version{}, fmt.Errorf("empty version string")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q: want MAJOR.MINOR.PATCH", s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return Version{}, fmt.Errorf("invalid version %q: %q is not a number", s, p)
		}
		if n < 0 {
			return Version{}, fmt.Errorf("invalid version %q: negative component", s)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

// ComputePath returns the adjacent-minor step sequence from current to target.
//
// For each intermediate minor between current.Minor+1 and target.Minor-1, it
// picks the highest patch available in catalog for that minor; the final step
// is exactly target. If current >= target, the path is empty. If a required
// intermediate minor is missing from catalog, it returns an error.
//
// Example: current v1.10.3, target v1.13.3, catalog {1.11.6, 1.12.8, 1.13.3}
// → [v1.11.6, v1.12.8, v1.13.3].
func ComputePath(current, target Version, catalog []Version) ([]Version, error) {
	if !current.Less(target) {
		return []Version{}, nil
	}
	if current.Major != target.Major {
		return nil, fmt.Errorf("cross-major upgrade %s -> %s not supported", current, target)
	}

	// Highest patch per minor available in the catalog (same major as target).
	best := map[int]Version{}
	for _, v := range catalog {
		if v.Major != target.Major {
			continue
		}
		if cur, ok := best[v.Minor]; !ok || cur.Less(v) {
			best[v.Minor] = v
		}
	}

	var path []Version
	for minor := current.Minor + 1; minor < target.Minor; minor++ {
		v, ok := best[minor]
		if !ok {
			return nil, fmt.Errorf("no release for required intermediate minor v%d.%d.x in catalog", target.Major, minor)
		}
		path = append(path, v)
	}
	path = append(path, target)
	return path, nil
}

// NodeRef is a minimal node descriptor used by the planner.
type NodeRef struct {
	Name string
	IP   string
	Role string
}

// OrderNodes returns nodes ordered workers-first, controlplane-last, with a
// stable secondary sort by name. The input slice is not mutated.
func OrderNodes(nodes []NodeRef) []NodeRef {
	out := make([]NodeRef, len(nodes))
	copy(out, nodes)
	sort.SliceStable(out, func(i, j int) bool {
		ci := out[i].Role == "controlplane"
		cj := out[j].Role == "controlplane"
		if ci != cj {
			// non-controlplane (worker) sorts first
			return !ci
		}
		return out[i].Name < out[j].Name
	})
	return out
}
