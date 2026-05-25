// Package snapshot defines the public structured shape emitted by
// `nostos dashboard --once --output json` and consumed by the TUI.
//
// Schema version is part of the public contract; the JSON envelope MUST
// only change in a major release. Bumping schema_version requires bumping
// nostos's documented MAJOR version.
package snapshot

import "time"

// Version is the schema version emitted in every snapshot.
const Version = "1"

// Severity is a check or row-level state.
type Severity string

const (
	SevInfo    Severity = "info"
	SevWarn    Severity = "warn"
	SevFail    Severity = "fail"
	SevUnknown Severity = "unknown"
)

// AggregateState is the top-bar status badge.
type AggregateState string

const (
	StateAllGreen      AggregateState = "ALL_GREEN"
	StateDegraded      AggregateState = "DEGRADED"
	StateBroken        AggregateState = "BROKEN"
	StateUnconfigured  AggregateState = "UNCONFIGURED"
	StateTransitioning AggregateState = "TRANSITIONING"
)

// CheckTier is the cadence bucket (fast=5s, slow=5min).
type CheckTier string

const (
	TierFast CheckTier = "fast"
	TierSlow CheckTier = "slow"
)

// Cluster is the cluster summary row.
type Cluster struct {
	Name              string `json:"name"`
	Endpoint          string `json:"endpoint"`
	TalosVersion      string `json:"talos_version"`
	NodesConfigured   int    `json:"nodes_configured"`
	NodesReady        int    `json:"nodes_ready"`
	KubeconfigPresent bool   `json:"kubeconfig_present"`
}

// Node is a per-node row.
type Node struct {
	Name        string   `json:"name"`
	IP          string   `json:"ip"`
	Tailscale   string   `json:"tailscale,omitempty"`
	Role        string   `json:"role"`
	Arch        string   `json:"arch"`
	Version     string   `json:"version,omitempty"`
	Ping        string   `json:"ping"`
	Apid        string   `json:"apid"`
	KubeReady   string   `json:"kube_ready,omitempty"`
	Severity    Severity `json:"severity"`
	Bucket      string   `json:"bucket"` // known|orphan
	SchematicID string   `json:"schematic_id,omitempty"`
}

// Discovery is an unbound device seen on the wire.
type Discovery struct {
	IP           string    `json:"ip"`
	MAC          string    `json:"mac,omitempty"`
	Hostname     string    `json:"hostname,omitempty"`
	Tailscale    string    `json:"tailscale,omitempty"`
	TalosMaint   bool      `json:"talos_maintenance,omitempty"`
	BMCRole      string    `json:"bmc_role,omitempty"`
	DiscoveredAt time.Time `json:"discovered_at"`
	ProbeID      string    `json:"probe_id"`
	Bucket       string    `json:"bucket"` // unknown|orphan
}

// Check is a registry-driven probe result.
type Check struct {
	ID              string    `json:"id"`
	Group           string    `json:"group"`
	Tier            CheckTier `json:"tier"`
	Severity        Severity  `json:"severity"`
	Message         string    `json:"message"`
	RemediationHint string    `json:"remediation_hint,omitempty"`
	DocsAnchor      string    `json:"docs_anchor,omitempty"`
	RanAt           time.Time `json:"ran_at"`
}

// App is an ArgoCD application row.
type App struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Sync      string   `json:"sync"`
	Health    string   `json:"health"`
	Severity  Severity `json:"severity"`
}

// UpstreamDiff is the diff against upstream Talos / charts / images.
type UpstreamDiff struct {
	TalosCurrent  string    `json:"talos_current,omitempty"`
	TalosLatest   string    `json:"talos_latest,omitempty"`
	TalosBehind   int       `json:"talos_behind"`
	FetchedAt     time.Time `json:"fetched_at"`
	Stale         bool      `json:"stale"`
	OfflineFellBack bool    `json:"offline_fell_back,omitempty"`
}

// Snapshot is the top-level emitted object.
type Snapshot struct {
	SchemaVersion     string         `json:"schema_version"`
	AggregateState    AggregateState `json:"aggregate_state"`
	Imperative        string         `json:"imperative,omitempty"`
	KubeconfigPresent bool           `json:"kubeconfig_present"`
	Cluster           Cluster        `json:"cluster"`
	Nodes             []Node         `json:"nodes"`
	Apps              []App          `json:"apps"`
	Discoveries       []Discovery    `json:"discoveries"`
	Checks            []Check        `json:"checks"`
	UpstreamDiff      UpstreamDiff   `json:"upstream_diff"`
	GeneratedAt       time.Time      `json:"generated_at"`
}

// JSONSchema returns the JSON-Schema descriptor for a Snapshot.
// Returned as map[string]any so it slots into nostos schema's stdout_schema.
func JSONSchema() map[string]any {
	stringT := map[string]any{"type": "string"}
	intT := map[string]any{"type": "integer"}
	boolT := map[string]any{"type": "boolean"}
	sevT := map[string]any{"type": "string", "enum": []string{"info", "warn", "fail", "unknown"}}
	stateT := map[string]any{"type": "string", "enum": []string{
		"ALL_GREEN", "DEGRADED", "BROKEN", "UNCONFIGURED", "TRANSITIONING",
	}}
	timeT := map[string]any{"type": "string", "format": "date-time"}

	clusterT := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":               stringT,
			"endpoint":           stringT,
			"talos_version":      stringT,
			"nodes_configured":   intT,
			"nodes_ready":        intT,
			"kubeconfig_present": boolT,
		},
	}
	nodeT := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":         stringT,
			"ip":           stringT,
			"tailscale":    stringT,
			"role":         stringT,
			"arch":         stringT,
			"version":      stringT,
			"ping":         stringT,
			"apid":         stringT,
			"kube_ready":   stringT,
			"severity":     sevT,
			"bucket":       stringT,
			"schematic_id": stringT,
		},
	}
	checkT := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":               stringT,
			"group":            stringT,
			"tier":             map[string]any{"type": "string", "enum": []string{"fast", "slow"}},
			"severity":         sevT,
			"message":          stringT,
			"remediation_hint": stringT,
			"docs_anchor":      stringT,
			"ran_at":           timeT,
		},
		"required": []string{"id", "group", "tier", "severity", "message", "ran_at"},
	}
	discT := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ip":                 stringT,
			"mac":                stringT,
			"hostname":           stringT,
			"tailscale":          stringT,
			"talos_maintenance":  boolT,
			"bmc_role":           stringT,
			"discovered_at":      timeT,
			"probe_id":           stringT,
			"bucket":             stringT,
		},
	}
	appT := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":      stringT,
			"namespace": stringT,
			"sync":      stringT,
			"health":    stringT,
			"severity":  sevT,
		},
	}
	upstreamT := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"talos_current":     stringT,
			"talos_latest":      stringT,
			"talos_behind":      intT,
			"fetched_at":        timeT,
			"stale":             boolT,
			"offline_fell_back": boolT,
		},
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title":   "nostos.dashboard.snapshot",
		"type":    "object",
		"required": []string{"schema_version", "aggregate_state", "cluster", "nodes", "checks", "generated_at"},
		"properties": map[string]any{
			"schema_version":     stringT,
			"aggregate_state":    stateT,
			"imperative":         stringT,
			"kubeconfig_present": boolT,
			"cluster":            clusterT,
			"nodes":              map[string]any{"type": "array", "items": nodeT},
			"apps":               map[string]any{"type": "array", "items": appT},
			"discoveries":        map[string]any{"type": "array", "items": discT},
			"checks":             map[string]any{"type": "array", "items": checkT},
			"upstream_diff":      upstreamT,
			"generated_at":       timeT,
		},
	}
}

// Aggregate computes the AggregateState from check severities and config size.
//
// Rules:
//   - UNCONFIGURED if nodesConfigured == 0
//   - TRANSITIONING if any check carries reason=transitioning (explicit signal)
//   - BROKEN if any check has Severity==fail
//   - DEGRADED if any check has Severity==warn
//   - else ALL_GREEN, but only when nodesConfigured > 0 AND no unknowns
func Aggregate(checks []Check, nodesConfigured int, unknownDevices int, transitioning bool) AggregateState {
	if nodesConfigured == 0 {
		return StateUnconfigured
	}
	if transitioning {
		return StateTransitioning
	}
	hasFail, hasWarn := false, false
	for _, c := range checks {
		switch c.Severity {
		case SevFail:
			hasFail = true
		case SevWarn:
			hasWarn = true
		}
	}
	if hasFail {
		return StateBroken
	}
	if hasWarn || unknownDevices > 0 {
		return StateDegraded
	}
	return StateAllGreen
}
