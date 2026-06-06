// NOTE: Taskfile wiring (`task nostos:install NODE=<name>`) is parent-repo
// concern; not modified in this wave.
package cli

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/dryrun"
	"github.com/yurifrl/nostos/internal/cli/errs"
	"github.com/yurifrl/nostos/internal/cli/inputx"
	"github.com/yurifrl/nostos/internal/cluster"
	"github.com/yurifrl/nostos/internal/pxe"
	"github.com/yurifrl/nostos/internal/registry"

	// Register provisioner factories.
	_ "github.com/yurifrl/nostos/internal/provisioner/pxe"
	_ "github.com/yurifrl/nostos/internal/provisioner/tpi"
)

func newNodeInstallCmd() *cobra.Command {
	var (
		reinstall bool
		yes       bool
		dryRun    bool
	)
	cmd := &cobra.Command{
		Use:   "install NAME",
		Short: "End-to-end install for NAME (method-dispatched: pxe|tpi)",
		Args:  cobra.ExactArgs(1),
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			if err := inputx.ValidateNodeName(args[0]); err != nil {
				return err
			}
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			n, err := registry.Get(cfg, args[0])
			if err != nil {
				return errs.NotFound("E_NODE_NOT_FOUND", err.Error()).
					WithDetails(map[string]any{"name": args[0]}).
					WithHint("nostos node list")
			}

			if dryRun {
				method := boot(n.Boot.Method)
				plan := dryrun.New("node.install").
					Add("preflight", fmt.Sprintf("validate config + reachability for %s", args[0])).
					Add("provisioner.preflight", fmt.Sprintf("prov=%s, host=%s", method, n.IP))
				if !reinstall {
					plan.Add("guard.reinstall", "would short-circuit if node is already Ready (pass --reinstall to bypass)")
				}
				if method == "pxe" {
					plan.AddArgv("pxe.serve", "serve iPXE chain + assets to "+n.MAC,
						[]string{"nostos", "pxe"}, []string{"PATH"})
					plan.Add("pxe.boot", "wait for node to fetch machineconfig.yaml")
				} else {
					plan.AddArgv("tpi.flash", "flash node "+args[0],
						[]string{"tpi", "flash", "-i", "<image>", "-n", "<slot>"},
						[]string{"TPI_USERNAME", "TPI_PASSWORD"})
				}
				plan.Add("apid.wait", "wait for talos apid to come up at "+n.IP)
				plan.Add("bootstrap", "talosctl bootstrap (controlplane only)")
				plan.Add("kubeconfig", "fetch kubeconfig to "+p.Kubeconfig())
				return emitDryRun(plan)
			}

			if !yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "About to install %s (method=%s). Pass --yes to skip prompt.\n", args[0], boot(n.Boot.Method))
			}

			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			events := make(chan cluster.Event, 32)
			done := make(chan error, 1)
			go func() {
				done <- cluster.Install(ctx, cfg, p, n, args[0],
					cluster.InstallOpts{Reinstall: reinstall}, events)
				close(events)
			}()
			collected := []cluster.Event{}
			for ev := range events {
				collected = append(collected, ev)
				if outputMode != "json" {
					printEvent(cmd, ev)
				}
			}
			if err := <-done; err != nil {
				if errors.Is(err, pxe.ErrSudoRequired) {
					return errs.Auth("E_SUDO_REQUIRED", err.Error()).WithHint("run: nostos pxe setup")
				}
				return errs.FromGo(err)
			}
			if outputMode == "json" {
				return outputJSON(map[string]any{
					"status": "installed",
					"name":   args[0],
					"events": collected,
				})
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&reinstall, "reinstall", false, "force reinstall even if node is currently Ready")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview planned actions as JSON; no subprocesses are spawned")
	return cmd
}

func boot(m string) string {
	if m == "" {
		return "pxe"
	}
	return m
}
