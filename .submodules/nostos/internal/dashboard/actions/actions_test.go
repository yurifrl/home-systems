package actions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
	"github.com/yurifrl/nostos/internal/execx/execxtest"
)

func TestIdentifyTPI(t *testing.T) {
	fc := execxtest.New()
	d := &ExecDispatcher{Cmd: fc}
	tgt := Target{Name: "tp1", BootMethod: "tpi", TPIHost: "tp.local", TPISlot: 2}
	r, err := d.Identify(context.Background(), tgt)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tpi", "power", "reset", "-n", "2", "--host", "tp.local"}
	if strings.Join(r.Argv, " ") != strings.Join(want, " ") {
		t.Fatalf("argv=%v want=%v", r.Argv, want)
	}
	if len(fc.Calls) != 1 || fc.Calls[0].Name != "tpi" {
		t.Fatalf("commander not invoked correctly: %+v", fc.Calls)
	}
}

func TestIdentifyPXE(t *testing.T) {
	fc := execxtest.New()
	d := &ExecDispatcher{Cmd: fc}
	r, err := d.Identify(context.Background(), Target{Name: "dell01", IP: "192.168.68.100", BootMethod: "pxe"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.Argv, " ") != "talosctl -n 192.168.68.100 reboot" {
		t.Fatalf("argv=%v", r.Argv)
	}
}

func TestIdentifyUnknownErrors(t *testing.T) {
	d := &ExecDispatcher{Cmd: execxtest.New()}
	_, err := d.Identify(context.Background(), Target{Name: "?", BootMethod: ""})
	if err == nil {
		t.Fatalf("expected error for unknown boot")
	}
}

func TestReinstallArgs(t *testing.T) {
	fc := execxtest.New()
	d := &ExecDispatcher{Cmd: fc}
	r, _ := d.Reinstall(context.Background(), Target{Name: "tp4"})
	if strings.Join(r.Argv, " ") != "nostos node install tp4 --reinstall --yes" {
		t.Fatalf("argv=%v", r.Argv)
	}
}

func TestDeleteOnlyOnOrphan(t *testing.T) {
	d := &ExecDispatcher{Cmd: execxtest.New()}
	_, err := d.Delete(context.Background(), Target{Name: "dell01", Bucket: "known"})
	if err == nil {
		t.Fatalf("expected delete error on non-orphan")
	}
	r, err := d.Delete(context.Background(), Target{Name: "ghost", IsOrphan: true, Bucket: "orphan"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.Argv, " ") != "nostos cluster cleanup --target ghost --yes" {
		t.Fatalf("argv=%v", r.Argv)
	}
}

func TestGoFixPicksWorstRemediable(t *testing.T) {
	d := &ExecDispatcher{DryRun: true}
	snap := snapshot.Snapshot{Checks: []snapshot.Check{
		{ID: "node.talos.apid", Severity: snapshot.SevFail, Message: "down", RanAt: time.Now()},
		{ID: "upstream.talos", Severity: snapshot.SevWarn, Message: "behind", RanAt: time.Now()},
	}}
	r, err := d.GoFix(context.Background(), snap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.Argv, " ") != "nostos cluster status" {
		t.Fatalf("expected cluster status (highest sev fail), got %v", r.Argv)
	}
}

func TestGoFixNothingToFix(t *testing.T) {
	d := &ExecDispatcher{DryRun: true}
	snap := snapshot.Snapshot{Checks: []snapshot.Check{
		{ID: "node.icmp", Severity: snapshot.SevWarn, Message: "ping", RanAt: time.Now()},
	}}
	_, err := d.GoFix(context.Background(), snap)
	if err != ErrNothingToFix {
		t.Fatalf("err=%v want ErrNothingToFix", err)
	}
}

func TestFakeDispatcherRecords(t *testing.T) {
	f := &FakeDispatcher{}
	f.Identify(context.Background(), Target{Name: "tp1", BootMethod: "tpi", TPISlot: 1})
	f.Reinstall(context.Background(), Target{Name: "dell01"})
	f.Delete(context.Background(), Target{Name: "ghost", IsOrphan: true, Bucket: "orphan"})
	f.GoFix(context.Background(), snapshot.Snapshot{Checks: []snapshot.Check{
		{ID: "node.talos.apid", Severity: snapshot.SevFail},
	}})
	if len(f.Calls) != 4 {
		t.Fatalf("want 4 calls got %d", len(f.Calls))
	}
	if f.Calls[0].Kind != KindIdentify || f.Calls[0].Argv[0] != "tpi" {
		t.Fatalf("identify call wrong: %+v", f.Calls[0])
	}
	if f.Calls[1].Kind != KindReinstall || f.Calls[1].Argv[0] != "nostos" || f.Calls[1].Argv[2] != "install" {
		t.Fatalf("reinstall call wrong: %+v", f.Calls[1])
	}
	if f.Calls[2].Kind != KindDelete || f.Calls[2].Argv[2] != "cleanup" {
		t.Fatalf("delete call wrong: %+v", f.Calls[2])
	}
	if f.Calls[3].Kind != KindGoFix {
		t.Fatalf("gofix call wrong: %+v", f.Calls[3])
	}
}

func TestNoopDispatcherDryRun(t *testing.T) {
	r, err := NoopDispatcher{}.Identify(context.Background(), Target{Name: "x", BootMethod: "pxe", IP: "1.2.3.4"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.DryRun {
		t.Fatalf("noop must be dry-run")
	}
}
