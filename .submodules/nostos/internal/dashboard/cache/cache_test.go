package cache

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
)

func TestRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "dashboard-state.json")
	in := snapshot.Snapshot{
		SchemaVersion:  snapshot.Version,
		AggregateState: snapshot.StateAllGreen,
		Nodes: []snapshot.Node{
			{Name: "dell01", IP: "192.168.68.100", Severity: snapshot.SevInfo, Bucket: "known"},
		},
		GeneratedAt: time.Now(),
	}
	if err := SaveTo(path, in); err != nil {
		t.Fatal(err)
	}
	out, ok := LoadFrom(path)
	if !ok {
		t.Fatalf("load failed")
	}
	if out.Snap.Cluster.Name != in.Cluster.Name || len(out.Snap.Nodes) != 1 {
		t.Fatalf("round-trip mismatch: %+v", out.Snap)
	}
	if out.CachedAt.IsZero() {
		t.Fatalf("cached_at not stamped")
	}
}

func TestMarkCached(t *testing.T) {
	s := snapshot.Snapshot{
		Nodes:       []snapshot.Node{{Name: "dell01"}},
		Discoveries: []snapshot.Discovery{{Hostname: "ghost", IP: "1.1.1.1"}},
	}
	MarkCached(&s)
	if !strings.HasPrefix(s.Nodes[0].Name, "~") {
		t.Fatalf("node not marked: %q", s.Nodes[0].Name)
	}
	if !strings.HasPrefix(s.Discoveries[0].Hostname, "~") {
		t.Fatalf("disc not marked: %q", s.Discoveries[0].Hostname)
	}
	// idempotent
	MarkCached(&s)
	if strings.HasPrefix(s.Nodes[0].Name, "~~") {
		t.Fatalf("MarkCached not idempotent: %q", s.Nodes[0].Name)
	}
}

func TestLoadMissing(t *testing.T) {
	if _, ok := LoadFrom(filepath.Join(t.TempDir(), "nope.json")); ok {
		t.Fatalf("expected ok=false for missing file")
	}
}
