package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/yurifrl/nostos/internal/upgrade"
)

func samplePlan() upgrade.Plan {
	tp1 := upgrade.NodeRef{Name: "tp1", IP: "10.0.0.3", Role: "worker"}
	tp4 := upgrade.NodeRef{Name: "tp4", IP: "10.0.0.2", Role: "worker"}
	dell := upgrade.NodeRef{Name: "dell01", IP: "10.0.0.1", Role: "controlplane"}
	return upgrade.Plan{
		Cluster:    "talos-default",
		Target:     "v1.13.3",
		MinCurrent: "v1.10.3",
		Nodes:      []upgrade.NodeRef{tp1, tp4, dell},
		Current: map[string]string{
			"tp1": "v1.10.3", "tp4": "v1.10.3", "dell01": "v1.10.3",
		},
		Steps: []upgrade.Step{
			{Version: "v1.11.6", Nodes: []upgrade.NodeRef{tp1, tp4, dell}},
			{Version: "v1.12.8", Nodes: []upgrade.NodeRef{tp1, tp4, dell}},
			{Version: "v1.13.3", Nodes: []upgrade.NodeRef{tp1, tp4, dell}},
		},
		Schematics: map[string]string{"tp1": "abc123schematic"},
	}
}

func keyPress(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	return tea.KeyPressMsg{Text: s}
}

func TestViewShowsHeaderNodesAndSteps(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := New(samplePlan())
	m.Width = 100
	v := m.View().Content
	for _, want := range []string{
		"talos-default", "Cluster on v1.10.3, target v1.13.3",
		"tp1", "tp4", "dell01", "controlplane",
		"v1.11.6", "v1.12.8", "v1.13.3",
		"Proceed", "Dry-run", "Quit",
	} {
		if !strings.Contains(v, want) {
			t.Fatalf("view missing %q in:\n%s", want, v)
		}
	}
}

func TestDetailToggleRevealsSchematic(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := New(samplePlan())
	m.Width = 100
	if strings.Contains(m.View().Content, "abc123schematic") {
		t.Fatalf("schematic should be hidden by default")
	}
	got, _ := m.Update(keyPress("d"))
	mm := got.(Model)
	if !strings.Contains(mm.View().Content, "abc123schematic") {
		t.Fatalf("d should reveal schematic IDs")
	}
}

func TestQuitKeySetsQuitAction(t *testing.T) {
	m := New(samplePlan())
	got, cmd := m.Update(keyPress("q"))
	mm := got.(Model)
	if cmd == nil {
		t.Fatalf("q should issue tea.Quit")
	}
	if mm.Action() != ActionQuit || !mm.Done() {
		t.Fatalf("q should set ActionQuit/done, got %q done=%v", mm.Action(), mm.Done())
	}
}

func TestSelectDryRun(t *testing.T) {
	m := New(samplePlan())
	// Move cursor to Dry-run (index 1).
	got, _ := m.Update(keyPress("right"))
	got, cmd := got.(Model).Update(keyPress("enter"))
	mm := got.(Model)
	if cmd == nil {
		t.Fatalf("selecting Dry-run should quit the program")
	}
	if mm.Action() != ActionDryRun || mm.Confirmed() {
		t.Fatalf("want ActionDryRun unconfirmed, got %q confirmed=%v", mm.Action(), mm.Confirmed())
	}
}

func TestProceedRequiresConfirm(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := New(samplePlan()) // cursor starts on Proceed (index 0)
	got, cmd := m.Update(keyPress("enter"))
	mm := got.(Model)
	if cmd != nil {
		t.Fatalf("Proceed should NOT quit until confirmed")
	}
	if mm.Done() || mm.Action() != ActionNone {
		t.Fatalf("Proceed should arm confirm, not finish: action=%q done=%v", mm.Action(), mm.Done())
	}
	if !strings.Contains(mm.View().Content, "press y to confirm") {
		t.Fatalf("confirm prompt missing: %s", mm.View().Content)
	}
	// Confirm with y.
	got, cmd = mm.Update(keyPress("y"))
	mm = got.(Model)
	if cmd == nil {
		t.Fatalf("y should commit and quit")
	}
	if mm.Action() != ActionProceed || !mm.Confirmed() {
		t.Fatalf("want ActionProceed confirmed, got %q confirmed=%v", mm.Action(), mm.Confirmed())
	}
}

func TestProceedConfirmCancelled(t *testing.T) {
	m := New(samplePlan())
	got, _ := m.Update(keyPress("enter")) // arm confirm
	got, cmd := got.(Model).Update(keyPress("x"))
	mm := got.(Model)
	if cmd != nil {
		t.Fatalf("cancelling confirm should not quit")
	}
	if mm.Done() || mm.Action() != ActionNone {
		t.Fatalf("cancel should leave model running: action=%q done=%v", mm.Action(), mm.Done())
	}
}
