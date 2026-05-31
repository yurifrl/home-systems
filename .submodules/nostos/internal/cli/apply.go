package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/dryrun"
	"github.com/yurifrl/nostos/internal/cli/errs"
	"github.com/yurifrl/nostos/internal/cli/inputx"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
	"github.com/yurifrl/nostos/internal/registry"
)

func newApplyCmd() *cobra.Command {
	var (
		mode       string
		all        bool
		yes        bool
		noValidate bool
		dryRun     bool
	)
	cmd := &cobra.Command{
		Use:   "apply [NODE]",
		Short: "Render + apply machineconfig to a running NODE (config-only changes)",
		Long: "Render NODE's template (resolving secrets) and push it to the running\n" +
			"node via authenticated `talosctl apply-config`. Use for config-only\n" +
			"changes such as node labels. Pass --all to apply to every node.",
		Args: cobra.MaximumNArgs(1),
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			if !registry.ValidApplyMode(mode) {
				return errs.Validation("E_INVALID_MODE",
					fmt.Sprintf("invalid --mode %q", mode)).
					WithDetails(map[string]any{"valid": registry.ApplyModes}).
					WithHint("one of: auto, no-reboot, reboot, staged, try")
			}
			if all && len(args) > 0 {
				return errs.Validation("E_ARGS_CONFLICT",
					"pass either NODE or --all, not both").
					WithHint("nostos apply NODE  |  nostos apply --all")
			}
			if !all && len(args) == 0 {
				return errs.Validation("E_NODE_REQUIRED",
					"NODE argument required (or pass --all)").
					WithHint("nostos node list")
			}

			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}

			// Resolve target node names.
			var names []string
			if all {
				for _, e := range registry.List(cfg) {
					names = append(names, e.Name)
				}
			} else {
				if err := inputx.ValidateNodeName(args[0]); err != nil {
					return err
				}
				names = []string{args[0]}
			}
			sort.Strings(names)

			if dryRun {
				plan := dryrun.New("apply")
				for _, name := range names {
					plan.Add("render", "render machineconfig for "+name+" (inject secrets)")
					plan.AddArgv("apply", "talosctl apply-config to "+name,
						[]string{"talosctl", "apply-config", "--talosconfig", "<talosconfig>",
							"--nodes", "<ip>", "--endpoints", "<ip>", "--file", "<rendered>",
							"--mode", mode}, nil)
				}
				return emitDryRun(plan)
			}

			// Confirmation gate for reboot-capable modes.
			if registry.ApplyModeReboots(mode) && !yes {
				return errs.Conflict("E_CONFIRM_REQUIRED",
					fmt.Sprintf("--mode=%s can reboot the node; refusing without --yes", mode)).
					WithDetails(map[string]any{"mode": mode, "nodes": names}).
					WithHint("re-run with --yes, or use --mode=no-reboot for label-only changes")
			}

			applied := make([]map[string]any, 0, len(names))
			for _, name := range names {
				node, err := registry.Get(cfg, name)
				if err != nil {
					return errs.NotFound("E_NODE_NOT_FOUND", err.Error()).
						WithDetails(map[string]any{"name": name}).
						WithHint("nostos node list")
				}
				if err := applyOne(cmd, cfg, p, node, name, mode, noValidate); err != nil {
					return err
				}
				applied = append(applied, map[string]any{"node": name, "mode": mode})
			}

			if outputMode == "json" {
				return outputJSON(map[string]any{"status": "applied", "mode": mode, "nodes": applied})
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&mode, "mode", "auto", "talosctl apply-config mode: auto|no-reboot|reboot|staged|try")
	cmd.Flags().BoolVar(&all, "all", false, "apply to every node in config.yaml")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation for reboot-capable modes")
	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "skip talosctl validate during render")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview planned actions as JSON; no subprocesses are spawned")
	return cmd
}

// applyOne renders then applies a single node, emitting text progress.
func applyOne(cmd *cobra.Command, cfg *config.Config, p paths.Paths, node config.Node, name, mode string, noValidate bool) error {
	out, err := registry.Render(cfg, p, name, !noValidate)
	if err != nil {
		return errs.FromGo(err)
	}
	if outputMode != "json" {
		fmt.Fprintf(cmd.OutOrStdout(), "→ rendered %s (%s)\n", name, out)
	}
	if err := registry.Apply(p, node, out, mode); err != nil {
		return errs.FromGo(err)
	}
	if outputMode != "json" {
		fmt.Fprintf(cmd.OutOrStdout(), "✓ applied %s (mode=%s)\n", name, mode)
	}
	return nil
}
