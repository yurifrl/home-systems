// Package dashboard wires the discovery, match, health, and upstream layers
// into a single Snapshot. The orchestrator is shared by the headless --once
// path and the TUI's refresh loop.
package dashboard

import (
	"context"
	"sort"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/dashboard/discovery"
	"github.com/yurifrl/nostos/internal/dashboard/health"
	"github.com/yurifrl/nostos/internal/dashboard/match"
	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
	"github.com/yurifrl/nostos/internal/dashboard/upstream"
	"github.com/yurifrl/nostos/internal/registry"
)

// Options configures a single snapshot build.
type Options struct {
	HiddenMACs map[string]bool
	ShowHidden bool
	// FetchUpstream controls whether the build calls factory.talos.dev
	// (HTTP). The TUI's slow tier sets this to true; --once defaults true
	// but can be flipped off via --no-upstream for offline runs.
	FetchUpstream bool
}

// BuildSnapshot runs all probes and check tiers serially and returns a
// fully-populated Snapshot.
func BuildSnapshot(ctx context.Context, cfg *config.Config, opt Options) snapshot.Snapshot {
	now := time.Now()
	kc := health.HasKubeconfig()

	// 1. Discovery.
	disc := discovery.Run(ctx, cfg)

	// 2. Per-node liveness via existing registry.Probe.
	statuses := map[string]registry.NodeStatus{}
	for _, e := range registry.List(cfg) {
		s := registry.Probe(e.Node, 800*time.Millisecond)
		s.Name = e.Name
		statuses[e.Name] = s
	}

	// 3. Upstream version diff.
	versions := upstream.Versions{}
	if opt.FetchUpstream {
		versions, _ = upstream.Fetch(ctx)
	} else if cached, ok := upstream.LoadCache(); ok {
		versions = cached
	}

	// 4. Match.
	buckets := match.Bind(cfg, disc, opt.HiddenMACs, opt.ShowHidden)

	// Promote per-node statuses into the Known rows.
	for i, n := range buckets.Known {
		if st, ok := statuses[n.Name]; ok {
			buckets.Known[i].Ping = string(st.Ping)
			buckets.Known[i].Apid = string(st.Apid)
			buckets.Known[i].Version = st.Version
			if st.Apid != registry.Up {
				buckets.Known[i].Severity = snapshot.SevFail
			} else if st.Version != "" && st.Version != cfg.Cluster.TalosVersion {
				buckets.Known[i].Severity = snapshot.SevWarn
			}
		}
	}

	// 5. Run check registry.
	checks := health.RunAll(ctx, &health.State{
		Cfg:               cfg,
		KubeconfigPresent: kc,
		NodeStatuses:      statuses,
		UpstreamVersions:  versions,
	})

	// 6. Aggregate.
	nodesReady := 0
	for _, n := range buckets.Known {
		if n.Severity == snapshot.SevInfo {
			nodesReady++
		}
	}
	state := snapshot.Aggregate(checks, len(cfg.Nodes), len(buckets.Unknown), false)

	return snapshot.Snapshot{
		SchemaVersion:     snapshot.Version,
		AggregateState:    state,
		Imperative:        imperativeFor(state, checks),
		KubeconfigPresent: kc,
		Cluster: snapshot.Cluster{
			Name:              cfg.Cluster.Name,
			Endpoint:          cfg.Cluster.Endpoint,
			TalosVersion:      cfg.Cluster.TalosVersion,
			NodesConfigured:   len(cfg.Nodes),
			NodesReady:        nodesReady,
			KubeconfigPresent: kc,
		},
		Nodes:        sortedKnown(buckets.Known),
		Apps:         nil, // populated lazily via runArgoCDApps; v0.3 keeps Apps empty in headless
		Discoveries:  appendOrphans(buckets.Unknown, buckets.Orphan),
		Checks:       checks,
		UpstreamDiff: upstreamDiff(cfg.Cluster.TalosVersion, versions),
		GeneratedAt:  now,
	}
}

func sortedKnown(in []snapshot.Node) []snapshot.Node {
	out := append([]snapshot.Node{}, in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func appendOrphans(unknown, orphan []snapshot.Discovery) []snapshot.Discovery {
	out := make([]snapshot.Discovery, 0, len(unknown)+len(orphan))
	out = append(out, unknown...)
	out = append(out, orphan...)
	return out
}

func upstreamDiff(current string, v upstream.Versions) snapshot.UpstreamDiff {
	stale := time.Since(v.FetchedAt) > upstream.CacheTTL
	if v.FetchedAt.IsZero() {
		stale = false
	}
	return snapshot.UpstreamDiff{
		TalosCurrent: current,
		TalosLatest:  v.TalosLatest,
		TalosBehind:  upstream.CountMinorBehind(current, v.TalosLatest),
		FetchedAt:    v.FetchedAt,
		Stale:        stale,
	}
}

func imperativeFor(state snapshot.AggregateState, checks []snapshot.Check) string {
	if state == snapshot.StateAllGreen || state == snapshot.StateUnconfigured {
		return ""
	}
	// Find the worst-severity check with a Message.
	pick := snapshot.Check{}
	rank := func(s snapshot.Severity) int {
		switch s {
		case snapshot.SevFail:
			return 3
		case snapshot.SevWarn:
			return 2
		case snapshot.SevUnknown:
			return 1
		}
		return 0
	}
	for _, c := range checks {
		if rank(c.Severity) > rank(pick.Severity) {
			pick = c
		}
	}
	if pick.Message == "" {
		return ""
	}
	return "Press G to open the recovery section for: " + pick.Message
}
