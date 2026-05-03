package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/registry"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show per-node reachability + Talos version",
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
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
			if outputMode == "json" {
				return outputJSON(rows)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Cluster %s\n", cfg.Cluster.Name)
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
			return nil
		}),
	}
}
