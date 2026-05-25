package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/yurifrl/nostos/internal/dashboard/actions"
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

// --- mutating-action tests ---------------------------------------------------

func keyPress(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	return tea.KeyPressMsg{Text: s}
}

func dispatcherTest(t *testing.T) (Model, *actions.FakeDispatcher) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	fd := &actions.FakeDispatcher{}
	m := Model{
		Snap:           sample(),
		Width:          120,
		Dispatcher:     fd,
		ConfirmTimeout: time.Second,
		Now:            func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
	return m, fd
}

func TestIdentifyChordFiresDispatcher(t *testing.T) {
	m, fd := dispatcherTest(t)
	got, _ := m.Update(keyPress("i"))
	mm := got.(Model)
	if mm.Pending() != actions.KindIdentify {
		t.Fatalf("i should arm identify, got %q", mm.Pending())
	}
	if !strings.Contains(mm.StatusPane, "talosctl") {
		t.Fatalf("status pane should preview command, got %q", mm.StatusPane)
	}
	got, _ = mm.Update(keyPress("y"))
	mm = got.(Model)
	if len(fd.Calls) != 1 || fd.Calls[0].Kind != actions.KindIdentify {
		t.Fatalf("dispatcher not called: %+v", fd.Calls)
	}
	want := "talosctl -n 192.168.68.100 reboot"
	if strings.Join(fd.Calls[0].Argv, " ") != want {
		t.Fatalf("argv=%v want=%q", fd.Calls[0].Argv, want)
	}
}

func TestChordCancelsOnOtherKey(t *testing.T) {
	m, fd := dispatcherTest(t)
	got, _ := m.Update(keyPress("i"))
	got, _ = got.(Model).Update(keyPress("x"))
	mm := got.(Model)
	if mm.Pending() != "" {
		t.Fatalf("chord should be cleared")
	}
	if len(fd.Calls) != 0 {
		t.Fatalf("dispatcher must not fire on cancel: %+v", fd.Calls)
	}
	if !strings.Contains(mm.StatusPane, "cancel") {
		t.Fatalf("status should mention cancel: %q", mm.StatusPane)
	}
}

func TestChordExpiresOnTimeout(t *testing.T) {
	m, fd := dispatcherTest(t)
	nowVal := time.Unix(1_700_000_000, 0)
	m.Now = func() time.Time { return nowVal }
	got, _ := m.Update(keyPress("i"))
	mm := got.(Model)
	// jump past expiry
	nowVal = nowVal.Add(10 * time.Second)
	mm.Now = func() time.Time { return nowVal }
	got, _ = mm.Update(keyPress("y"))
	mm = got.(Model)
	if len(fd.Calls) != 0 {
		t.Fatalf("timed-out chord should not dispatch: %+v", fd.Calls)
	}
	if !strings.Contains(mm.StatusPane, "timed out") {
		t.Fatalf("expected timeout status, got %q", mm.StatusPane)
	}
}

func TestReinstallChord(t *testing.T) {
	m, fd := dispatcherTest(t)
	got, _ := m.Update(keyPress("r"))
	got, _ = got.(Model).Update(keyPress("y"))
	_ = got
	if len(fd.Calls) != 1 || fd.Calls[0].Kind != actions.KindReinstall {
		t.Fatalf("reinstall not dispatched: %+v", fd.Calls)
	}
	want := "nostos node install dell01 --reinstall --yes"
	if strings.Join(fd.Calls[0].Argv, " ") != want {
		t.Fatalf("argv=%v want=%q", fd.Calls[0].Argv, want)
	}
}

func TestDeleteRequiresOrphan(t *testing.T) {
	m, fd := dispatcherTest(t)
	got, _ := m.Update(keyPress("d"))
	mm := got.(Model)
	if mm.Pending() != "" {
		t.Fatalf("delete must not arm on known node")
	}
	if len(fd.Calls) != 0 {
		t.Fatalf("no dispatch expected: %+v", fd.Calls)
	}
	if !strings.Contains(mm.StatusPane, "orphan") {
		t.Fatalf("want orphan-only message, got %q", mm.StatusPane)
	}
}

func TestDeleteOrphanChord(t *testing.T) {
	m, fd := dispatcherTest(t)
	m.Snap.Discoveries = []snapshot.Discovery{{IP: "10.0.0.99", Hostname: "ghost", Bucket: "orphan"}}
	m.Cursor = len(m.Snap.Nodes) // first discovery
	got, _ := m.Update(keyPress("d"))
	mm := got.(Model)
	if mm.Pending() != actions.KindDelete {
		t.Fatalf("delete should arm on orphan: status=%q", mm.StatusPane)
	}
	got, _ = mm.Update(keyPress("y"))
	_ = got
	if len(fd.Calls) != 1 || fd.Calls[0].Kind != actions.KindDelete {
		t.Fatalf("delete not dispatched: %+v", fd.Calls)
	}
	if fd.Calls[0].Argv[2] != "cleanup" {
		t.Fatalf("argv wrong: %v", fd.Calls[0].Argv)
	}
}

func TestGoFixNothingToFix(t *testing.T) {
	m, fd := dispatcherTest(t)
	m.Snap.Checks = []snapshot.Check{{ID: "node.icmp", Severity: snapshot.SevWarn, Message: "icmp", RanAt: time.Now()}}
	got, _ := m.Update(keyPress("G"))
	mm := got.(Model)
	if mm.Pending() != "" {
		t.Fatalf("G must not arm when nothing remediable")
	}
	if !strings.Contains(mm.StatusPane, "nothing to auto-fix") {
		t.Fatalf("want nothing-to-fix status, got %q", mm.StatusPane)
	}
	if len(fd.Calls) != 0 {
		t.Fatalf("no dispatch expected")
	}
}

func TestGoFixFiresHighestSeverityRemediable(t *testing.T) {
	m, fd := dispatcherTest(t)
	m.Snap.Checks = []snapshot.Check{
		{ID: "node.talos.apid", Severity: snapshot.SevFail, Message: "down", RanAt: time.Now()},
		{ID: "upstream.talos", Severity: snapshot.SevWarn, Message: "behind", RanAt: time.Now()},
	}
	got, _ := m.Update(keyPress("G"))
	got, _ = got.(Model).Update(keyPress("y"))
	_ = got
	if len(fd.Calls) != 1 || fd.Calls[0].Kind != actions.KindGoFix {
		t.Fatalf("gofix not dispatched: %+v", fd.Calls)
	}
	want := "nostos cluster status"
	if strings.Join(fd.Calls[0].Argv, " ") != want {
		t.Fatalf("argv=%v want=%q", fd.Calls[0].Argv, want)
	}
}

func TestFooterMentionsActionKeys(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := Model{Snap: sample(), Width: 120}
	v := m.View().Content
	for _, want := range []string{"[i]dentify", "[r]einstall", "[d]elete", "[G]o-fix"} {
		if !strings.Contains(v, want) {
			t.Fatalf("footer missing %q in %q", want, v)
		}
	}
}

func TestDispatcherInterface(t *testing.T) {
	// compile-time guard so the FakeDispatcher remains a Dispatcher.
	var _ actions.Dispatcher = (*actions.FakeDispatcher)(nil)
	_, _ = (&actions.FakeDispatcher{}).Identify(context.Background(), actions.Target{Name: "x", BootMethod: "pxe", IP: "1.2.3.4"})
}
