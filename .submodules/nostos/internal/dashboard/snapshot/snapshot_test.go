package snapshot

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAggregateRules(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name        string
		checks      []Check
		nodes       int
		unknown     int
		transition  bool
		want        AggregateState
	}{
		{"empty config", nil, 0, 0, false, StateUnconfigured},
		{"transition wins over fail", []Check{{Severity: SevFail, RanAt: now}}, 3, 0, true, StateTransitioning},
		{"fail beats warn", []Check{{Severity: SevWarn, RanAt: now}, {Severity: SevFail, RanAt: now}}, 3, 0, false, StateBroken},
		{"warn -> degraded", []Check{{Severity: SevWarn, RanAt: now}}, 3, 0, false, StateDegraded},
		{"unknown device -> degraded", []Check{{Severity: SevInfo, RanAt: now}}, 3, 1, false, StateDegraded},
		{"all info -> green", []Check{{Severity: SevInfo, RanAt: now}}, 3, 0, false, StateAllGreen},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Aggregate(c.checks, c.nodes, c.unknown, c.transition)
			if got != c.want {
				t.Fatalf("got %s want %s", got, c.want)
			}
		})
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	snap := Snapshot{
		SchemaVersion:  Version,
		AggregateState: StateAllGreen,
		Cluster:        Cluster{Name: "talos-default", NodesConfigured: 1},
		Nodes:          []Node{{Name: "dell01", IP: "1.2.3.4", Severity: SevInfo, Bucket: "known"}},
		Checks:         []Check{{ID: "talos.apid", Group: "node", Tier: TierFast, Severity: SevInfo, Message: "ok", RanAt: time.Now()}},
		GeneratedAt:    time.Now(),
	}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"schema_version":"1"`) {
		t.Fatalf("schema_version missing: %s", b)
	}
	var got Snapshot
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.AggregateState != StateAllGreen {
		t.Fatalf("aggregate roundtrip failed")
	}
}

func TestJSONSchema(t *testing.T) {
	s := JSONSchema()
	if s["title"] != "nostos.dashboard.snapshot" {
		t.Fatalf("missing title")
	}
	props, _ := s["properties"].(map[string]any)
	for _, k := range []string{
		"schema_version", "aggregate_state", "cluster", "nodes",
		"checks", "discoveries", "upstream_diff", "generated_at",
	} {
		if _, ok := props[k]; !ok {
			t.Fatalf("missing property %s", k)
		}
	}
}
