package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"charm.land/lipgloss/v2"

	"github.com/yurifrl/nostos/internal/cli/errs"
	"github.com/yurifrl/nostos/internal/cli/inputx"
	"github.com/yurifrl/nostos/internal/cli/jsonio"
	"github.com/yurifrl/nostos/internal/registry"
)

// nodeStatusFields is the schema used by --fields validation for node list/show/status.
var nodeStatusFields = []string{"name", "ip", "role", "ping", "apid", "version"}

func newNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Manage node registrations",
	}
	cmd.AddCommand(newNodeListCmd(), newNodeShowCmd(), newNodeRemoveCmd(), newNodeInstallCmd())
	return cmd
}

func newNodeListCmd() *cobra.Command {
	var fieldsRaw string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List registered nodes with live reachability",
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			fields, err := parseFields(fieldsRaw, nodeStatusFields)
			if err != nil {
				return err
			}
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}
			entries := registry.List(cfg)
			rows := make([]registry.NodeStatus, 0, len(entries))
			for _, e := range entries {
				s := registry.Probe(e.Node, 1500*time.Millisecond)
				s.Name = e.Name
				rows = append(rows, s)
			}
			return emitListOutput(rows, nodeStatusFields, fields, func() {
				header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", header.Render(fmt.Sprintf("Nodes in %s", cfg.Cluster.Name)))
				fmt.Fprintf(cmd.OutOrStdout(), "%-10s  %-16s  %-12s  %-8s  %-8s  %s\n",
					"NAME", "IP", "ROLE", "PING", "APID", "VERSION")
				for _, s := range rows {
					ver := s.Version
					if ver == "" {
						ver = "—"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%-10s  %-16s  %-12s  %s  %s  %s\n",
						s.Name, s.IP, s.Role, pill(s.Ping), pill(s.Apid), ver)
				}
			})
		}),
	}
	cmd.Flags().StringVar(&fieldsRaw, "fields", "", "comma-separated subset of "+joinFields(nodeStatusFields))
	cmd.Flags().Bool("dry-run", false, "no-op for read-only command (warns on stderr)")
	return cmd
}

func newNodeShowCmd() *cobra.Command {
	var fieldsRaw string
	cmd := &cobra.Command{
		Use:   "show NAME",
		Short: "Show one node's reachability and config",
		Args:  cobra.ExactArgs(1),
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			if err := inputx.ValidateNodeName(args[0]); err != nil {
				return err
			}
			fields, err := parseFields(fieldsRaw, nodeStatusFields)
			if err != nil {
				return err
			}
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}
			n, err := registry.Get(cfg, args[0])
			if err != nil {
				return errs.NotFound("E_NODE_NOT_FOUND", err.Error()).
					WithDetails(map[string]any{"name": args[0]}).
					WithHint("nostos node list")
			}
			s := registry.Probe(n, 1500*time.Millisecond)
			s.Name = args[0]
			return emitObjectOutput(s, fields, func() {
				m, _ := jsonio.ToMap(s)
				for _, k := range nodeStatusFields {
					fmt.Fprintf(cmd.OutOrStdout(), "%-10s %v\n", k+":", m[k])
				}
			})
		}),
	}
	cmd.Flags().StringVar(&fieldsRaw, "fields", "", "comma-separated subset of "+joinFields(nodeStatusFields))
	return cmd
}

func newNodeRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove a node from config.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			if err := inputx.ValidateNodeName(args[0]); err != nil {
				return err
			}
			_, p, err := loadConfig()
			if err != nil {
				return err
			}
			if !yes {
				return errs.Conflict("E_CONFIRM_REQUIRED", "refusing to remove without --yes").
					WithHint("re-run with --yes")
			}
			if err := registry.Remove(p.Config, args[0]); err != nil {
				return errs.FromGo(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", args[0])
			return nil
		}),
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip interactive confirmation")
	return cmd
}

// pill returns a styled reachability badge suitable for a plain-text column.
func pill(r registry.Reachability) string {
	style := lipgloss.NewStyle().Bold(true)
	switch r {
	case registry.Up:
		return style.Foreground(lipgloss.Color("10")).Render(padRight(string(r), 8))
	case registry.Down:
		return style.Foreground(lipgloss.Color("9")).Render(padRight(string(r), 8))
	case registry.Refused:
		return style.Foreground(lipgloss.Color("11")).Render(padRight(string(r), 8))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(padRight(string(r), 8))
	}
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + "        "[:n-len(s)]
}

func joinFields(fs []string) string {
	out := ""
	for i, f := range fs {
		if i > 0 {
			out += ","
		}
		out += f
	}
	return out
}
