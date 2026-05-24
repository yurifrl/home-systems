// NOTE: Taskfile wiring (`task nostos:install NODE=<name>`) is parent-repo
// concern; not modified in this wave.
package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cluster"
	"github.com/yurifrl/nostos/internal/registry"

	// Register provisioner factories.
	_ "github.com/yurifrl/nostos/internal/provisioner/pxe"
	_ "github.com/yurifrl/nostos/internal/provisioner/tpi"
)

func newNodeInstallCmd() *cobra.Command {
	var (
		reinstall bool
		yes       bool
	)
	cmd := &cobra.Command{
		Use:   "install NAME",
		Short: "End-to-end install for NAME (method-dispatched: pxe|tpi)",
		Args:  cobra.ExactArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			n, err := registry.Get(cfg, args[0])
			if err != nil {
				return err
			}
			if !yes {
				fmt.Fprintf(cmd.OutOrStdout(), "About to install %s (method=%s). Pass --yes to skip prompt.\n", args[0], boot(n.Boot.Method))
			}

			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			events := make(chan cluster.Event, 32)
			done := make(chan error, 1)
			go func() {
				done <- cluster.Install(ctx, cfg, p, n, args[0],
					cluster.InstallOpts{
						Reinstall: reinstall,
					}, events)
				close(events)
			}()
			for ev := range events {
				printEvent(cmd, ev)
			}
			return <-done
		}),
	}
	cmd.Flags().BoolVar(&reinstall, "reinstall", false, "force reinstall even if node is currently Ready")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	_ = time.Second
	return cmd
}

func boot(m string) string {
	if m == "" {
		return "pxe"
	}
	return m
}
