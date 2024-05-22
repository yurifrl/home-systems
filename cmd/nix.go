package cmd

import (
	"fmt"
	"os"

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
		// --json???
		err := executor.ExecuteCommand(
			"nix", "build", ".#nixosConfigurations.rpi.config.system.build.sdImage",
			"--show-trace",
			"-I", fmt.Sprintf("nixos-config=%s/nix/sd-image.nix", dockerWorkdir),
		)
		if err != nil {
			fmt.Printf("Error during the build process: %v\n", err)
			os.Exit(1)
		}

		// // Move built image to output directory
		// err = executor.ExecuteCommand(
		// 	"mv",
		// 	"/nix/store/*-nixos-sd-image-*/sd-image/*.img",
		// 	fmt.Sprintf("%s/output/", dockerWorkdir),
		// )
		// if err != nil {
		// 	fmt.Printf("Error moving the image: %v\n", err)
		// 	os.Exit(1)
		// }
		// fmt.Println("SD image built and moved successfully.")
	},
}
