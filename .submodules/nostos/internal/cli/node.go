package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"charm.land/lipgloss/v2"

	"github.com/yurifrl/nostos/internal/registry"
)

func newNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Manage node registrations",
	}
	cmd.AddCommand(newNodeListCmd(), newNodeRemoveCmd())
	return cmd
}

func newNodeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered nodes with live reachability",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig()
			if err != nil {
				return err
			}
			entries := registry.List(cfg)

			if outputMode == "json" {
				rows := make([]registry.NodeStatus, 0, len(entries))
				for _, e := range entries {
					s := registry.Probe(e.Node, 1500*time.Millisecond)
					s.Name = e.Name
					rows = append(rows, s)
				}
				return outputJSON(rows)
			}

			header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
			dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", header.Render(fmt.Sprintf("Nodes in %s", cfg.Cluster.Name)))
			fmt.Fprintf(cmd.OutOrStdout(), "%-10s  %-16s  %-12s  %-8s  %-8s  %s\n",
				"NAME", "IP", "ROLE", "PING", "APID", "VERSION")
			for _, e := range entries {
				s := registry.Probe(e.Node, 1500*time.Millisecond)
				ver := s.Version
				if ver == "" {
					ver = dim.Render("—")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-10s  %-16s  %-12s  %s  %s  %s\n",
					e.Name, e.Node.IP, e.Node.Role, pill(s.Ping), pill(s.Apid), ver)
			}
			return nil
		}),
	}
}

func newNodeRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove a node from config.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			_, p, err := loadConfig()
			if err != nil {
				return err
			}
			if !yes {
				return fmt.Errorf("refusing to remove without --yes confirmation")
			}
			if err := registry.Remove(p.Config, args[0]); err != nil {
				return err
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
