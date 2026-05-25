package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/cli/errs"
	"github.com/yurifrl/nostos/internal/cli/inputx"
	"github.com/yurifrl/nostos/internal/cli/jsonio"
	"github.com/yurifrl/nostos/internal/cli/schema"
	"github.com/yurifrl/nostos/internal/dashboard/snapshot"
)

func newSchemaCmd() *cobra.Command {
	var (
		all       bool
		exitCodes bool
	)
	cmd := &cobra.Command{
		Use:   "schema [METHOD]",
		Short: "Print machine-readable schema descriptors for nostos commands",
		Args:  cobra.MaximumNArgs(1),
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			w := cmd.OutOrStdout()
			if exitCodes {
				return jsonio.EncodePretty(w, errs.Catalog())
			}
			if all {
				return jsonio.EncodePretty(w, schema.All(root))
			}
			if len(args) == 0 {
				ids := schema.IDs(root)
				if outputMode == "json" {
					rows := make([]map[string]any, 0, len(ids))
					all := schema.All(root)
					for _, id := range ids {
						rows = append(rows, map[string]any{
							"method":      id,
							"description": all[id].Description,
						})
					}
					return jsonio.EncodeNDJSON(w, rows)
				}
				all := schema.All(root)
				for _, id := range ids {
					fmt.Fprintf(cmd.OutOrStdout(), "%-28s  %s\n", id, all[id].Description)
				}
				return nil
			}
			method := args[0]
			if method == "dashboard.snapshot" {
				return jsonio.EncodePretty(w, snapshot.JSONSchema())
			}
			all := schema.All(root)
			m, ok := all[method]
			if !ok {
				return errs.NotFound("E_SCHEMA_METHOD",
					fmt.Sprintf("no method %q (try `nostos schema` for the list)", inputx.SanitizeForJSON(method))).
					WithDetails(map[string]any{"method": method})
			}
			return jsonio.EncodePretty(w, m)
		}),
	}
	cmd.Flags().BoolVar(&all, "all", false, "emit a JSON object keyed by method ID for every command")
	cmd.Flags().BoolVar(&exitCodes, "exit-codes", false, "emit the exit-code catalog as JSON")
	return cmd
}
