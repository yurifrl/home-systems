package health

import (
	"context"
	"testing"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
	"github.com/yurifrl/nostos/internal/dashboard/upstream"
	"github.com/yurifrl/nostos/internal/registry"
)

func TestRunAllCoversRegistry(t *testing.T) {
	s := &State{
		Cfg: &config.Config{
			Cluster: config.Cluster{Name: "x", TalosVersion: "v1.10.3", SchematicID: "abcd"},
			Nodes: map[string]config.Node{
				"a": {IP: "1.2.3.4", Role: "controlplane", Arch: "amd64"},
			},
		},
		KubeconfigPresent: false,
		NodeStatuses: map[string]registry.NodeStatus{
			"a": {Name: "a", IP: "1.2.3.4", Ping: registry.Up, Apid: registry.Up, Version: "v1.10.3"},
		},
		UpstreamVersions: upstream.Versions{TalosLatest: "v1.12.0"},
	}
	got := RunAll(context.Background(), s)
	if len(got) != len(Registered) {
		t.Fatalf("expected %d checks, got %d", len(Registered), len(got))
	}
	// upstream check should warn (2 minor behind)
	for _, c := range got {
		if c.ID == string(CheckUpstreamTalos) && c.Severity != snapshot.SevWarn {
			t.Fatalf("upstream check should warn, got %v", c.Severity)
		}
	}
}
