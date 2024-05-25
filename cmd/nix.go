package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/internal/executors"
)

// Nix command group
var nixCmd = &cobra.Command{
	Use:   "nix",
	Short: "TODO",
	Long:  `TODO`,
}

// Build nix image
var nixBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Nix package",
	Long:  `Builds a Nix package from the specified configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Choose the executor based on an environment variable
		executor := &executors.LocalExecutor{}

		// New approach using nix build which is more up-to-date with Nix version 2.x
		// --json ???
		// --debugger - Opens iterative shell
		// --debug - show debug level log
		err := executor.ExecuteCommand(
			"nix", "build", ".#nixosConfigurations.rpi.config.system.build.sdImage",
			"--show-trace",
			"--print-out-paths",
			"--json",
		)
		if err != nil {
			fmt.Printf("Error during the build process: %v\n", err)
			os.Exit(1)
		}

		matches, err := filepath.Glob("/src/nix/result/sd-image/*.img")
		if err != nil {
			fmt.Printf("Failed to find files: %v\n", err)
			os.Exit(1)
		}
		if len(matches) == 0 {
			fmt.Println("No files to copy.")
			os.Exit(1)
		}

		for _, match := range matches {
			executor := &executors.LocalExecutor{}

			// Extract filename for destination
			filename := filepath.Base(match)
			destination := filepath.Join("/src", filename)

			err := executor.ExecuteCommand("cp", match, destination)
			if err != nil {
				fmt.Printf("Failed to copy %s: %v\n", match, err)
				os.Exit(1)
			}
		}
		fmt.Println("Files copied successfully.")
	},
}
