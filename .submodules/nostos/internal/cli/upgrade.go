package cli

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/errs"
	"github.com/yurifrl/nostos/internal/cli/inputx"
	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/registry"
	"github.com/yurifrl/nostos/internal/upgrade"
	upgradetui "github.com/yurifrl/nostos/internal/upgrade/tui"
)

func newUpgradeCmd() *cobra.Command {
	var (
		to     string
		all    bool
		yes    bool
		dryRun bool
		mode   string
	)
	cmd := &cobra.Command{
		Use:   "upgrade [NODE]",
		Short: "Safe rolling Talos OS upgrade (adjacent-minor sweeps; workers first, controlplane last)",
		Long: "Compute and run a SAFE rolling Talos OS upgrade. With no flags nostos\n" +
			"fetches the Talos release catalog, detects each node's running version,\n" +
			"and plans per-minor sweeps across the whole cluster (workers first,\n" +
			"controlplane last). --dry-run prints the plan and changes nothing; the\n" +
			"actual upgrade reboots nodes and is gated behind --yes.",
		Args: cobra.MaximumNArgs(1),
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("all") && all && len(args) > 0 {
				return errs.Validation("E_ARGS_CONFLICT",
					"pass either NODE or --all, not both").
					WithHint("nostos upgrade NODE  |  nostos upgrade --all")
			}

			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}

			if to == "" {
				to = cfg.Cluster.TalosVersion
			}
			target, err := upgrade.ParseVersion(to)
			if err != nil {
				return errs.Validation("E_INVALID_VERSION",
					fmt.Sprintf("invalid --to version %q: %v", to, err)).
					WithHint("use a Talos version like v1.13.3")
			}

			// Resolve target node names.
			var names []string
			if len(args) > 0 {
				if err := inputx.ValidateNodeName(args[0]); err != nil {
					return err
				}
				names = []string{args[0]}
			} else {
				for _, e := range registry.List(cfg) {
					names = append(names, e.Name)
				}
			}
			sort.Strings(names)
			if len(names) == 0 {
				return errs.Validation("E_NO_NODES", "no nodes in config.yaml").
					WithHint("nostos node list")
			}

			// Build NodeRefs.
			nodes := make([]config.Node, 0, len(names))
			refs := make([]upgrade.NodeRef, 0, len(names))
			for _, name := range names {
				n, err := registry.Get(cfg, name)
				if err != nil {
					return errs.NotFound("E_NODE_NOT_FOUND", err.Error()).
						WithDetails(map[string]any{"name": name}).
						WithHint("nostos node list")
				}
				nodes = append(nodes, n)
				refs = append(refs, upgrade.NodeRef{Name: name, IP: n.IP, Role: n.Role})
			}
			ordered := upgrade.OrderNodes(refs)

			// Fetch the release catalog (network).
			catalog, err := upgrade.FetchCatalog(cmd.Context(), nil)
			if err != nil {
				return errs.Network("E_CATALOG", err.Error()).
					WithHint("check network access to api.github.com")
			}

			// Detect each node's running version (network/exec).
			current := map[string]upgrade.Version{}
			for _, ref := range ordered {
				v, err := upgrade.DetectVersion(p.Talosconfig(), ref.IP)
				if err != nil {
					return errs.Network("E_DETECT",
						fmt.Sprintf("detect version for %s (%s): %v", ref.Name, ref.IP, err)).
						WithHint("ensure the node is reachable and talosconfig is valid")
				}
				current[ref.Name] = v
			}

			// Compute the per-minor step path from the lowest current version,
			// and build the per-step node lists (shared planner — see
			// upgrade.BuildPlan, reused by the interactive TUI).
			plan, err := upgrade.BuildPlan(cfg.Cluster.Name, ordered, current, target, catalog)
			if err != nil {
				return errs.Validation("E_PATH", err.Error()).
					WithHint("the release catalog is missing a required intermediate minor")
			}
			// Annotate each node with its resolved factory schematic so the TUI
			// detail toggle can reveal it (hidden by default).
			plan.Schematics = map[string]string{}
			for i, name := range names {
				plan.Schematics[name] = nodes[i].EffectiveSchematic(cfg.Cluster)
			}

			if len(plan.Steps) == 0 {
				if outputMode == "json" {
					return outputJSON(map[string]any{"status": "up-to-date", "target": target.String()})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "All nodes already at or above %s; nothing to do.\n", target)
				return nil
			}

			// Interactive plan screen: when stdout is a TTY and the operator
			// hasn't pre-committed via --dry-run/--yes (and isn't asking for
			// JSON), present the plan and let them choose. Dry-run prints the
			// plan; Proceed (after an explicit in-TUI confirm) takes the same
			// health-gated path --yes uses; Quit aborts.
			if !dryRun && !yes && outputMode != "json" && term.IsTerminal(os.Stdout.Fd()) {
				res, err := upgradetui.Run(cmd.Context(), plan)
				if err != nil {
					return errs.FromGo(err)
				}
				switch res.Action {
				case upgradetui.ActionDryRun:
					dryRun = true
				case upgradetui.ActionProceed:
					if !res.Confirmed {
						return nil
					}
					yes = true
				default: // ActionQuit / ActionNone
					return nil
				}
			}

			if dryRun {
				if outputMode == "json" {
					return outputJSON(map[string]any{
						"status": "preview",
						"method": "upgrade",
						"target": target.String(),
						"steps":  plan.Steps,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Upgrade plan (target %s):\n", target)
				for _, s := range plan.Steps {
					var nn []string
					for _, n := range s.Nodes {
						nn = append(nn, n.Name)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  step %s: %v\n", s.Version, nn)
				}
				return nil
			}

			// Mutating path reboots nodes — require --yes.
			if !yes {
				return errs.Conflict("E_CONFIRM_REQUIRED",
					"upgrade reboots nodes; refusing without --yes").
					WithDetails(map[string]any{"target": target.String(), "steps": plan.Steps}).
					WithHint("review with --dry-run, then re-run with --yes")
			}

			// Execute: for each step, upgrade each ordered node, health-gating
			// before moving to the next node.
			byName := map[string]config.Node{}
			for i, name := range names {
				byName[name] = nodes[i]
			}
			for _, s := range plan.Steps {
				sv, _ := upgrade.ParseVersion(s.Version)
				for _, ref := range s.Nodes {
					node := byName[ref.Name]
					image := upgrade.InstallerImage(node.EffectiveSchematic(cfg.Cluster), sv)
					fmt.Fprintf(cmd.OutOrStdout(), "→ upgrading %s (%s) to %s\n", ref.Name, ref.IP, sv)
					if err := upgrade.Upgrade(cmd.Context(), p.Talosconfig(), ref.IP, image); err != nil {
						return errs.FromGo(err)
					}
					progress := func(line string) {
						if outputMode != "json" {
							fmt.Fprintln(cmd.OutOrStdout(), line)
						}
					}
					if err := upgrade.WaitHealthy(p.Talosconfig(), ref.IP, sv, 15*time.Minute, 15*time.Second, progress); err != nil {
						return errs.Timeout("E_HEALTH", err.Error()).
							WithHint("node did not return healthy at the step version in time")
					}
				}
			}

			if outputMode == "json" {
				return outputJSON(map[string]any{"status": "upgraded", "target": target.String(), "steps": plan.Steps})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ cluster upgraded to %s\n", target)
			return nil
		}),
	}
	cmd.Flags().StringVar(&to, "to", "", "target Talos version (default: cluster.talos_version)")
	cmd.Flags().BoolVar(&all, "all", true, "upgrade every node (default when no NODE given)")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the upgrade (required: nodes reboot)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the upgrade plan; no changes")
	cmd.Flags().StringVar(&mode, "mode", "reboot", "talosctl upgrade mode (reboot)")
	_ = mode
	return cmd
}
