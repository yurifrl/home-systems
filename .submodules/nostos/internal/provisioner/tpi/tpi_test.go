package tpi

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/clockx"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/execx/execxtest"
	"github.com/yurifrl/nostos/internal/provisioner"
)

func TestTPIBootNoCredsOmitsEnv(t *testing.T) {
	fc := execxtest.New(execxtest.Script{}, execxtest.Script{}, execxtest.Script{})
	cfg := &config.Config{
		Cluster: config.Cluster{SchematicID: "x", TalosVersion: "v1", ImageDigests: map[string]string{"x/v1/arm64": "sha256:00"}},
		Secrets: config.Secrets{Backend: "env"},
	}
	deps := provisioner.Deps{Cfg: cfg, Cmd: fc, Clock: clockx.NewFakeClock(time.Time{})}
	p := New(deps).(*Provisioner)
	p.imgPath = "/tmp/fake.raw"
	// no p.user / p.pass set, no IdentityFileRef on the node
	node := &config.Node{Boot: config.Boot{TPI: &config.TPIBoot{Host: "10.0.0.1", Slot: 2}}}
	if err := p.Boot(context.Background(), node, func(provisioner.Event) {}); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	for i, c := range fc.Calls {
		for _, e := range c.Env {
			if strings.HasPrefix(e, "TPI_USERNAME=") || strings.HasPrefix(e, "TPI_PASSWORD=") {
				t.Errorf("call %d unexpected creds env %q", i, e)
			}
		}
	}
	// Cleanup with no secrets dir / key path is a no-op.
	if err := p.Cleanup(context.Background(), node, func(provisioner.Event) {}); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
}

func TestTPIMethodAndKey(t *testing.T) {
	p := New(provisioner.Deps{Clock: clockx.NewFakeClock(time.Time{})})
	if p.Method() != "tpi" {
		t.Fatalf("Method = %q", p.Method())
	}
	node := &config.Node{Boot: config.Boot{TPI: &config.TPIBoot{Host: "192.168.68.10", Slot: 1}}}
	if got := p.ContentionKey(node); got != "tpi:192.168.68.10" {
		t.Fatalf("ContentionKey = %q", got)
	}
}

func TestTPIBootEnvAndArgv(t *testing.T) {
	const password = "S3CR3T-DO-NOT-LEAK"
	fc := execxtest.New(execxtest.Script{}, execxtest.Script{}, execxtest.Script{})
	cfg := &config.Config{
		Cluster: config.Cluster{SchematicID: "x", TalosVersion: "v1", ImageDigests: map[string]string{"x/v1/arm64": "sha256:00"}},
		Secrets: config.Secrets{Backend: "env"},
	}
	deps := provisioner.Deps{
		Cfg:   cfg,
		Cmd:   fc,
		Clock: clockx.NewFakeClock(time.Time{}),
	}
	p := New(deps).(*Provisioner)
	// Pretend image is ready.
	p.imgPath = "/tmp/fake.raw"
	// Inject username/password without going through real backends.
	p.user = "admin"
	p.pass = password

	node := &config.Node{Boot: config.Boot{TPI: &config.TPIBoot{Host: "10.0.0.1", Slot: 2}}}
	if err := p.Boot(context.Background(), node, func(provisioner.Event) {}); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	if len(fc.Calls) != 3 {
		t.Fatalf("expected 3 tpi calls, got %d: %+v", len(fc.Calls), fc.Calls)
	}
	wantArg0 := []string{"power", "flash", "power"}
	for i, c := range fc.Calls {
		argline := strings.Join(c.Args, " ")
		if !strings.Contains(argline, wantArg0[i]) {
			t.Errorf("call %d args missing %q: %v", i, wantArg0[i], c.Args)
		}
		if strings.Contains(argline, password) {
			t.Errorf("call %d argv leaks password: %v", i, c.Args)
		}
		hasPW := false
		for _, e := range c.Env {
			if e == "TPI_PASSWORD="+password {
				hasPW = true
			}
		}
		if !hasPW {
			t.Errorf("call %d missing TPI_PASSWORD env", i)
		}
	}
}

func TestTPIVersionParse(t *testing.T) {
	if v := parseVersion("tpi version 1.2.3 (commit ...)"); v != "1.2.3" {
		t.Fatalf("parseVersion = %q", v)
	}
	if !versionAtLeast("1.2.3", "1.0.0") {
		t.Fatal("1.2.3 should satisfy 1.0.0")
	}
	if versionAtLeast("0.4.0", "1.0.0") {
		t.Fatal("0.4.0 must not satisfy 1.0.0")
	}
}

func TestTPIWaitMaintenanceTimeout(t *testing.T) {
	fc := execxtest.New(execxtest.Script{Err: errSentinel{}})
	clk := clockx.NewFakeClock(time.Time{})
	deps := provisioner.Deps{Cmd: fc, Clock: clk}
	p := New(deps).(*Provisioner)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.WaitMaintenance(ctx, &config.Node{IP: "1.2.3.4"}, func(provisioner.Event) {})
	if err == nil {
		t.Fatal("expected error from cancelled ctx")
	}
}

func TestTPICleanupIdempotent(t *testing.T) {
	p := New(provisioner.Deps{Clock: clockx.NewFakeClock(time.Time{})}).(*Provisioner)
	ctx := context.Background()
	if err := p.Cleanup(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := p.Cleanup(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "boom" }

// Sanity: package import surface is valid.
var _ = os.Getenv
