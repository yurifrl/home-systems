package match

import (
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/dashboard/discovery"
)

func TestBindMatrix(t *testing.T) {
	cfg := &config.Config{
		Cluster: config.Cluster{Name: "test", SchematicID: "x"},
		Nodes: map[string]config.Node{
			"dell01": {MAC: "aa:bb:cc:dd:ee:ff", IP: "1.2.3.4", Role: "controlplane", Arch: "amd64"},
			"tp1":    {MAC: "11:22:33:44:55:66", IP: "1.2.3.5", Role: "worker", Arch: "arm64"},
		},
	}
	res := discovery.Result{
		Devices: []discovery.Device{
			{IP: "1.2.3.4", MAC: "aa:bb:cc:dd:ee:ff", DiscoveredAt: time.Now(), ProbeID: "talos-maintenance"},
			{IP: "192.168.68.250", MAC: "ff:ee:dd:cc:bb:aa", DiscoveredAt: time.Now(), ProbeID: "arp"},
		},
		StartedAt: time.Now(),
	}
	b := Bind(cfg, res, nil, false)
	if len(b.Known) != 1 || b.Known[0].Name != "dell01" {
		t.Fatalf("known wrong: %+v", b.Known)
	}
	if len(b.Orphan) != 1 || b.Orphan[0].Hostname != "tp1" {
		t.Fatalf("orphan wrong: %+v", b.Orphan)
	}
	if len(b.Unknown) != 1 || b.Unknown[0].IP != "192.168.68.250" {
		t.Fatalf("unknown wrong: %+v", b.Unknown)
	}
}

func TestHiddenDevice(t *testing.T) {
	cfg := &config.Config{Nodes: map[string]config.Node{}}
	res := discovery.Result{
		Devices: []discovery.Device{
			{IP: "10.0.0.1", MAC: "ab:cd:ef:00:11:22", ProbeID: "arp"},
		},
		StartedAt: time.Now(),
	}
	hidden := map[string]bool{"ab:cd:ef:00:11:22": true}
	b := Bind(cfg, res, hidden, false)
	if len(b.Unknown) != 0 {
		t.Fatalf("hidden should be filtered: %+v", b.Unknown)
	}
	b = Bind(cfg, res, hidden, true)
	if len(b.Unknown) != 1 {
		t.Fatalf("show-hidden should reveal: %+v", b.Unknown)
	}
}
