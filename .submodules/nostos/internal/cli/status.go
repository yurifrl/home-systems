package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/inputx"
	"github.com/yurifrl/nostos/internal/cli/jsonio"
	"github.com/yurifrl/nostos/internal/registry"
)

func newStatusCmd() *cobra.Command {
	var fieldsRaw string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show per-node reachability + Talos version",
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
			if outputMode == "json" {
				if len(fields) > 0 {
					projected, err := jsonio.ProjectSlice(rows, fields)
					if err != nil {
						return err
					}
					return jsonio.EncodePretty(cmd.OutOrStdout(), map[string]any{
						"cluster": map[string]any{"name": cfg.Cluster.Name, "healthy": isHealthy(rows)},
						"nodes":   projected,
					})
				}
				return jsonio.EncodePretty(cmd.OutOrStdout(), map[string]any{
					"cluster": map[string]any{"name": cfg.Cluster.Name, "healthy": isHealthy(rows)},
					"nodes":   rows,
				})
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
	cmd.Flags().StringVar(&fieldsRaw, "fields", "", "comma-separated subset of "+joinFields(nodeStatusFields))
	_ = inputx.SanitizeForJSON
	return cmd
}

func isHealthy(rows []registry.NodeStatus) bool {
	if len(rows) == 0 {
		return false
	}
	for _, s := range rows {
		if s.Ping != registry.Up {
			return false
		}
	}
	return true
}
