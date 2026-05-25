package cluster

import (
	"context"
	"testing"
	"time"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/provisioner"
)

// fakeProv is a Provisioner double for orchestrator tests. The only knob
// that matters here is MaxWaitMaintenance; the other hooks are stubbed.
type fakeProv struct {
	max time.Duration
}

func (f *fakeProv) Method() string                       { return "fake" }
func (f *fakeProv) ContentionKey(*config.Node) string    { return "fake" }
func (f *fakeProv) MaxWaitMaintenance() time.Duration    { return f.max }
func (f *fakeProv) Preflight(context.Context, *config.Node, provisioner.EventEmitter) error {
	return nil
}
func (f *fakeProv) Prepare(context.Context, *config.Node, provisioner.EventEmitter) error {
	return nil
}
func (f *fakeProv) Boot(context.Context, *config.Node, provisioner.EventEmitter) error { return nil }
func (f *fakeProv) WaitMaintenance(context.Context, *config.Node, provisioner.EventEmitter) error {
	return nil
}
func (f *fakeProv) Apply(context.Context, *config.Node, string, provisioner.EventEmitter) error {
	return nil
}
func (f *fakeProv) Cleanup(context.Context, *config.Node, provisioner.EventEmitter) error {
	return nil
}

// TestChooseWaitTimeout pins the contract from A1: a Provisioner returning a
// non-zero MaxWaitMaintenance dictates the WaitMaintenance ctx deadline.
func TestChooseWaitTimeout(t *testing.T) {
	cases := []struct {
		name string
		opts InstallOpts
		prov provisioner.Provisioner
		want time.Duration
	}{
		{
			name: "provisioner_wins_25m",
			opts: InstallOpts{WaitMaintenanceDeadline: 20 * time.Minute, BootTimeout: 10 * time.Minute},
			prov: &fakeProv{max: 25 * time.Minute},
			want: 25 * time.Minute,
		},
		{
			name: "provisioner_zero_falls_back_to_opts",
			opts: InstallOpts{WaitMaintenanceDeadline: 17 * time.Minute, BootTimeout: 10 * time.Minute},
			prov: &fakeProv{max: 0},
			want: 17 * time.Minute,
		},
		{
			name: "all_zero_falls_back_to_boot_timeout",
			opts: InstallOpts{BootTimeout: 9 * time.Minute},
			prov: &fakeProv{max: 0},
			want: 9 * time.Minute,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chooseWaitTimeout(tc.opts, tc.prov); got != tc.want {
				t.Fatalf("chooseWaitTimeout = %v, want %v", got, tc.want)
			}
		})
	}
}
