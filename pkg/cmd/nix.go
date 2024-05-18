package cmd

import (
	"github.com/spf13/cobra"
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
		// ...
		err := nctx.ExecuteCommand("nix-build", "--show-trace", "<nixpkgs/nixos>", "-A", "config.system.build.sdImage.outPath", "-I", "nixos-config=/workdir/nix/sd-image.nix", "--argstr", "system", "aarch64-linux")
		if err != nil {
			panic(err)
		}

		// Move the output to /workdir/output
		err = nctx.ExecuteCommand("mv", "/nix/store/*-nixos-sd-image-*/sd-image/*.img", "/workdir/output/")
		if err != nil {
			panic(err)
		}
	},
}
