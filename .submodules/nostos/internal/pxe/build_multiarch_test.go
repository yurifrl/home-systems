package pxe

import (
	"sort"
	"testing"

	"github.com/yurifrl/nostos/internal/config"
)

// TestCollectAssetSpecs verifies that multi-arch fleets produce one (schematic,
// arch) pair per node, with the rpi_generic flag set on RPi nodes only.
func TestCollectAssetSpecs(t *testing.T) {
	cfg := &config.Config{
		Cluster: config.Cluster{
			TalosVersion: "v1.13.3",
			SchematicID:  "8f04ea6b6016f12a593fa8a87441270075c648cb75482c2d9d3db8cecda47da1",
		},
		Nodes: map[string]config.Node{
			"dell01": {Arch: "amd64", IP: "192.168.68.100"},
			"tp1": {
				Arch:        "arm64",
				IP:          "192.168.68.107",
				SchematicID: "6f9371bccd9df78d8c26521528700b463f25bce1cad97691722a4189719e6aa9",
			},
			"tp4": {
				Arch:        "arm64",
				IP:          "192.168.68.114",
				SchematicID: "6f9371bccd9df78d8c26521528700b463f25bce1cad97691722a4189719e6aa9",
			},
			"rpi01": {
				Arch:        "arm64",
				IP:          "192.168.0.170",
				Overlay:     "rpi_generic",
				SchematicID: "d0e797e79d4e8c53a843776a1a5b57a3429aaaf7e8e3246d35df5df9f915da86",
			},
		},
	}
	specs := CollectAssetSpecs(cfg)
	if len(specs) != 3 {
		t.Fatalf("got %d specs, want 3 (dell01-amd64, tp*-arm64, rpi01-arm64): %+v", len(specs), specs)
	}
	// Sort for deterministic assertions.
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Schematic != specs[j].Schematic {
			return specs[i].Schematic < specs[j].Schematic
		}
		return specs[i].Arch < specs[j].Arch
	})
	// rpi01 spec must carry the IsRPi flag.
	var foundRPi bool
	for _, s := range specs {
		if s.Schematic == "d0e797e79d4e8c53a843776a1a5b57a3429aaaf7e8e3246d35df5df9f915da86" {
			if !s.IsRPi {
				t.Errorf("rpi01 schematic spec missing IsRPi flag")
			}
			foundRPi = true
		}
	}
	if !foundRPi {
		t.Errorf("rpi01 schematic spec not collected")
	}
}

// TestCollectAssetSpecs_DedupesByPair confirms two nodes sharing the same
// (schematic, arch) collapse into one spec.
func TestCollectAssetSpecsDedupesByPair(t *testing.T) {
	cfg := &config.Config{
		Cluster: config.Cluster{
			TalosVersion: "v1.13.3",
			SchematicID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		Nodes: map[string]config.Node{
			"a": {Arch: "amd64", IP: "10.0.0.1"},
			"b": {Arch: "amd64", IP: "10.0.0.2"}, // same arch+default schematic
		},
	}
	specs := CollectAssetSpecs(cfg)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec for two nodes sharing (schematic, arch); got %d", len(specs))
	}
}
