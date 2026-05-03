// Package cli wires cobra subcommands.
package cli

import (
	"github.com/spf13/cobra"
)

// Global flag values populated during cobra parsing.
var (
	configPath string
	outputMode string
	verbose    bool
)

// NewRoot constructs the top-level `nostos` cobra command with all subcommands attached.
func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "nostos",
		Short:   "bring your bare metal home to the Talos cluster",
		Version: version,
		Long: "nostos orchestrates bare-metal Talos provisioning: " +
			"PXE serve, machineconfig render, admin-cert regen, bootstrap, status.\n" +
			"Invocation: go run ./.submodules/nostos/cmd/nostos <subcommand>.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&configPath, "config", "",
		"path to config.yaml (overrides discovery)")
	root.PersistentFlags().StringVar(&outputMode, "output", "text",
		"output format: text|json")
	root.PersistentFlags().BoolVar(&verbose, "verbose", false,
		"show stack traces on errors")

	root.AddCommand(
		newInitCmd(),
		newNodeCmd(),
		newRenderCmd(),
		newBuildCmd(),
		newPxeCmd(),
		newStatusCmd(),
		newWipeCmd(),
		newBootstrapCmd(),
		newUpCmd(),
		newKubeconfigCmd(),
		newNukeCmd(),
		newConfigCmd(),
	)

	return root
}
