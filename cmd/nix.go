package cmd

import (
	"fmt"

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

		err := executor.ExecuteCommand(
			"nix-build",
			"--show-trace",
			"<nixpkgs/nixos>",
			"-A", "config.system.build.sdImage.outPath",
			"-I", fmt.Sprintf("nixos-config=%s/nix/sd-image.nix", dockerWorkdir),
			"--argstr",
			"system", "aarch64-linux",
		)
		if err != nil {
			panic(err)
		}

		err = executor.ExecuteCommand("ls", "-la")
		if err != nil {
			panic(err)
		}

		err = executor.ExecuteCommand("mv", "/nix/store/*-nixos-sd-image-*/sd-image/*.img", fmt.Sprintf("%s/output/", dockerWorkdir))
		if err != nil {
			panic(err)
		}
	},
}
