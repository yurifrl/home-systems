// Package health is the hardcoded check registry. Each check declares a
// CheckID, Tier, Group, DocsAnchor, and a pure Run(ctx, *State) Result.
//
// v0.3 SHALL NOT expose a plugin interface.
package health

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
	"github.com/yurifrl/nostos/internal/dashboard/upstream"
	"github.com/yurifrl/nostos/internal/registry"
)

// CheckID is a typed-string identifier for a registered check.
type CheckID string

const (
	CheckTalosAPID         CheckID = "node.talos.apid"
	CheckTalosVersion      CheckID = "node.talos.version"
	CheckICMP              CheckID = "node.icmp"
	CheckSchematic         CheckID = "node.schematic"
	CheckK8sAPI            CheckID = "cluster.k8s.api"
	CheckEtcd              CheckID = "cluster.etcd.quorum"
	CheckTailscaleFleet    CheckID = "cluster.tailscale.fleet"
	CheckArgoCDApps        CheckID = "cluster.argocd.apps"
	CheckUpstreamTalos     CheckID = "upstream.talos"
)

// State is the read-only context passed to every Run().
type State struct {
	Cfg               *config.Config
	KubeconfigPresent bool
	NodeStatuses      map[string]registry.NodeStatus // keyed by node name
	UpstreamVersions  upstream.Versions
}

// Run is the per-check function signature.
type Run func(ctx context.Context, s *State) snapshot.Check

// Registered is the hardcoded registry. Keep order stable for deterministic
// snapshot output.
var Registered = []struct {
	ID    CheckID
	Group string
	Tier  snapshot.CheckTier
	Run   Run
}{
	{CheckK8sAPI, "cluster", snapshot.TierFast, runK8sAPI},
	{CheckEtcd, "cluster", snapshot.TierSlow, runEtcd},
	{CheckTailscaleFleet, "cluster", snapshot.TierFast, runTailscaleFleet},
	{CheckArgoCDApps, "cluster", snapshot.TierSlow, runArgoCDApps},
	{CheckTalosAPID, "node", snapshot.TierFast, runTalosAPID},
	{CheckICMP, "node", snapshot.TierFast, runICMP},
	{CheckTalosVersion, "node", snapshot.TierSlow, runTalosVersion},
	{CheckSchematic, "node", snapshot.TierSlow, runSchematic},
	{CheckUpstreamTalos, "upstream", snapshot.TierSlow, runUpstreamTalos},
}

// RunAll invokes every registered check exactly once.
func RunAll(ctx context.Context, s *State) []snapshot.Check {
	out := make([]snapshot.Check, 0, len(Registered))
	for _, r := range Registered {
		c := r.Run(ctx, s)
		c.ID = string(r.ID)
		c.Group = r.Group
		c.Tier = r.Tier
		c.RanAt = time.Now()
		out = append(out, c)
	}
	return out
}

// --- individual check implementations ---

func runK8sAPI(ctx context.Context, s *State) snapshot.Check {
	if !s.KubeconfigPresent {
		return snapshot.Check{
			Severity: snapshot.SevWarn,
			Message:  "kubeconfig not found — cluster checks disabled",
			RemediationHint: "run `nostos kubeconfig` after a controlplane is up",
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "kubectl", "version", "--output=yaml").CombinedOutput()
	if err != nil {
		return snapshot.Check{
			Severity: snapshot.SevFail,
			Message:  fmt.Sprintf("k8s API unreachable: %v", err),
			RemediationHint: trim(string(out)),
		}
	}
	return snapshot.Check{Severity: snapshot.SevInfo, Message: "k8s API reachable"}
}

func runEtcd(ctx context.Context, s *State) snapshot.Check {
	if _, err := exec.LookPath("talosctl"); err != nil {
		return snapshot.Check{Severity: snapshot.SevUnknown, Message: "talosctl not installed"}
	}
	// pick first controlplane
	var cp string
	for _, n := range s.Cfg.Nodes {
		if n.Role == "controlplane" {
			cp = n.IP
			break
		}
	}
	if cp == "" {
		return snapshot.Check{Severity: snapshot.SevUnknown, Message: "no controlplane in config"}
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "talosctl", "-n", cp, "etcd", "members").CombinedOutput()
	if err != nil {
		return snapshot.Check{
			Severity: snapshot.SevWarn,
			Message:  fmt.Sprintf("etcd members probe failed: %v", err),
			RemediationHint: trim(string(out)),
		}
	}
	return snapshot.Check{Severity: snapshot.SevInfo, Message: "etcd quorum healthy"}
}

func runTailscaleFleet(ctx context.Context, s *State) snapshot.Check {
	// Without OAuth credentials wired we surface as unknown.
	if s.Cfg.Secrets.Tailscale == nil {
		return snapshot.Check{Severity: snapshot.SevUnknown, Message: "Tailscale not configured"}
	}
	return snapshot.Check{Severity: snapshot.SevInfo, Message: "Tailscale OAuth backend configured"}
}

func runArgoCDApps(ctx context.Context, s *State) snapshot.Check {
	if !s.KubeconfigPresent {
		return snapshot.Check{Severity: snapshot.SevWarn, Message: "kubeconfig unavailable"}
	}
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "kubectl", "-n", "argocd", "get", "applications",
		"-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\t\"}{.status.sync.status}{\"\\t\"}{.status.health.status}{\"\\n\"}{end}").CombinedOutput()
	if err != nil {
		return snapshot.Check{Severity: snapshot.SevUnknown, Message: "argocd CRDs not installed or RBAC denied"}
	}
	bad := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		if parts[1] != "Synced" || parts[2] != "Healthy" {
			bad = append(bad, parts[0])
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return snapshot.Check{
			Severity: snapshot.SevWarn,
			Message:  fmt.Sprintf("argocd: %d app(s) not Synced/Healthy: %s", len(bad), strings.Join(bad, ", ")),
		}
	}
	return snapshot.Check{Severity: snapshot.SevInfo, Message: "argocd apps Synced+Healthy"}
}

func runTalosAPID(ctx context.Context, s *State) snapshot.Check {
	bad := []string{}
	for name, st := range s.NodeStatuses {
		if st.Apid != registry.Up {
			bad = append(bad, name)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return snapshot.Check{
			Severity: snapshot.SevFail,
			Message:  fmt.Sprintf("talos apid down on: %s", strings.Join(bad, ", ")),
			RemediationHint: "check the node is powered + on the network",
		}
	}
	return snapshot.Check{Severity: snapshot.SevInfo, Message: "talos apid up on all nodes"}
}

func runICMP(ctx context.Context, s *State) snapshot.Check {
	bad := []string{}
	for name, st := range s.NodeStatuses {
		if st.Ping != registry.Up {
			bad = append(bad, name)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return snapshot.Check{Severity: snapshot.SevWarn, Message: fmt.Sprintf("icmp down on: %s", strings.Join(bad, ", "))}
	}
	return snapshot.Check{Severity: snapshot.SevInfo, Message: "icmp ok on all nodes"}
}

func runTalosVersion(ctx context.Context, s *State) snapshot.Check {
	want := s.Cfg.Cluster.TalosVersion
	mismatch := []string{}
	for name, st := range s.NodeStatuses {
		if st.Version != "" && st.Version != want {
			mismatch = append(mismatch, fmt.Sprintf("%s=%s", name, st.Version))
		}
	}
	if len(mismatch) > 0 {
		sort.Strings(mismatch)
		return snapshot.Check{
			Severity: snapshot.SevWarn,
			Message:  fmt.Sprintf("talos version drift (want %s): %s", want, strings.Join(mismatch, ", ")),
		}
	}
	return snapshot.Check{Severity: snapshot.SevInfo, Message: fmt.Sprintf("talos version pinned at %s", want)}
}

func runSchematic(ctx context.Context, s *State) snapshot.Check {
	// We can only verify schematic match against the actual node by talking
	// to talosctl get extensions — that's deferred. v0.3 reports based on
	// effective-config-only check (does each node resolve a non-empty schematic).
	bad := []string{}
	for name, n := range s.Cfg.Nodes {
		if n.EffectiveSchematic(s.Cfg.Cluster) == "" {
			bad = append(bad, name)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return snapshot.Check{Severity: snapshot.SevFail, Message: fmt.Sprintf("nodes without schematic_id: %s", strings.Join(bad, ", "))}
	}
	return snapshot.Check{Severity: snapshot.SevInfo, Message: "schematic_id resolved for all nodes"}
}

func runUpstreamTalos(ctx context.Context, s *State) snapshot.Check {
	cur := s.Cfg.Cluster.TalosVersion
	lat := s.UpstreamVersions.TalosLatest
	if lat == "" {
		return snapshot.Check{Severity: snapshot.SevUnknown, Message: "upstream talos version not available"}
	}
	behind := upstream.CountMinorBehind(cur, lat)
	switch {
	case behind == 0:
		return snapshot.Check{Severity: snapshot.SevInfo, Message: fmt.Sprintf("Talos %s ⟶ %s (current)", cur, lat)}
	case behind >= 100:
		return snapshot.Check{
			Severity: snapshot.SevWarn,
			Message:  fmt.Sprintf("Talos %s ⟶ %s (major bump available)", cur, lat),
			RemediationHint: "press u to preview `nostos cluster upgrade`",
		}
	default:
		return snapshot.Check{
			Severity: snapshot.SevWarn,
			Message:  fmt.Sprintf("Talos %s ⟶ %s (%d minor behind)", cur, lat, behind),
			RemediationHint: "press u to preview `nostos cluster upgrade`",
		}
	}
}

// trim collapses a multiline blob into a single line for hint display.
func trim(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120] + "…"
	}
	return s
}

// HasKubeconfig returns true if KUBECONFIG is set or ~/.kube/config exists.
func HasKubeconfig() bool {
	if p := os.Getenv("KUBECONFIG"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(home + "/.kube/config"); err == nil {
			return true
		}
	}
	return false
}
