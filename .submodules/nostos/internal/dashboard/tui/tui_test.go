package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
)

func sample() snapshot.Snapshot {
	return snapshot.Snapshot{
		SchemaVersion:  snapshot.Version,
		AggregateState: snapshot.StateAllGreen,
		Cluster:        snapshot.Cluster{Name: "talos-default", NodesConfigured: 1, NodesReady: 1, KubeconfigPresent: true},
		Nodes: []snapshot.Node{
			{Name: "dell01", IP: "192.168.68.100", Role: "controlplane", Arch: "amd64", Version: "v1.10.3", Severity: snapshot.SevInfo, Bucket: "known"},
		},
		Checks:      []snapshot.Check{{ID: "x", Severity: snapshot.SevInfo, Message: "ok", RanAt: time.Now()}},
		GeneratedAt: time.Now(),
	}
}

func TestViewIncludesHeader(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := Model{Snap: sample(), Width: 100}
	v := m.View()
	if !strings.Contains(v.Content, "talos-default") {
		t.Fatalf("missing cluster name: %q", v.Content)
	}
	if !strings.Contains(v.Content, "ALL_GREEN") {
		t.Fatalf("missing aggregate state badge: %q", v.Content)
	}
	if !strings.Contains(v.Content, "dell01") {
		t.Fatalf("missing node row: %q", v.Content)
	}
	if !v.AltScreen {
		t.Fatalf("AltScreen should be true")
	}
}

func TestQuitKey(t *testing.T) {
	m := Model{Snap: sample()}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatalf("q should issue tea.Quit cmd")
	}
}

func TestHelpToggle(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := Model{Snap: sample()}
	got, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	mm := got.(Model)
	if !mm.ShowHelp {
		t.Fatalf("? should open help")
	}
	if !strings.Contains(mm.View().Content, "keymap") {
		t.Fatalf("help view missing keymap header")
	}
}

func TestAsciiSymbols(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if symbolFor(snapshot.SevInfo, "") != "[OK]" {
		t.Fatalf("expected [OK] under NO_COLOR")
	}
}

func TestRenderDeterminism(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := Model{Snap: sample(), Width: 100}
	a := m.View().Content
	b := m.View().Content
	if a != b {
		t.Fatalf("render is non-deterministic")
	}
}
