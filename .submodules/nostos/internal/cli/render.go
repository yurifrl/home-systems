package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/registry"
)

func newRenderCmd() *cobra.Command {
	var noValidate bool
	cmd := &cobra.Command{
		Use:   "render NODE",
		Short: "Render NODE's machineconfig with secrets injected",
		Args:  cobra.ExactArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			cfg, p, err := loadConfig()
			if err != nil {
				return err
			}
			out, err := registry.Render(cfg, p, args[0], !noValidate)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rendered %s\n", out)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "skip talosctl validate on output")
	return cmd
}
