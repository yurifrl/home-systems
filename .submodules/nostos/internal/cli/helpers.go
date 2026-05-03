package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/config"
	"github.com/yurifrl/nostos/internal/paths"
)

// loadConfig resolves + parses the config.yaml. Cobra-level errors are printed.
func loadConfig() (*config.Config, paths.Paths, error) {
	p, err := config.FindConfig(configPath, "")
	if err != nil {
		return nil, paths.Paths{}, err
	}
	cfg, err := config.Load(p)
	if err != nil {
		return nil, paths.Paths{}, err
	}
	return cfg, paths.New(p), nil
}

// outputJSON prints v as pretty JSON. Used when --output json is set.
func outputJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// runEFuncSimple wraps common cobra RunE boilerplate: print errors, return code.
func runEFuncSimple(fn func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := fn(cmd, args); err != nil {
			if !verbose {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return err
		}
		return nil
	}
}
