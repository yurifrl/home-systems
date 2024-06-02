package cmd

import (
	"os"

	"github.com/k0kubun/pp/v3"
	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/src/internal/executors"
	"github.com/yurifrl/home-systems/src/pkg/types"
)

var (
	cfgFile       string
	verbose       bool
	dockerWorkdir = "/src"
	isosDir       = "isos"
	_             = pp.Println
	// Flash
	isoImage = ""
	device   = ""
	// NixOps
	nixopsWorkdir    = "/workdir/nix/nixops/"
	nixDeployVersion = ""
)

// Simplified global help command
var helpCmd = &cobra.Command{
	Use:   "help",
	Short: "Display global help",
	Long:  `Display help information for all commands.`,
	Run: func(cmd *cobra.Command, args []string) {
		rootCmd.Help()
	},
}

// Build nix image
var TestCmd = &cobra.Command{
	Use:   "test",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		var executor types.Executor

		if os.Getenv("USE_DOCKER") == "true" {
			// If inside docker, then you can run locally
			executor = &executors.LocalExecutor{}
		} else {
			// Otherwise run it inside the container
			executor = &executors.DockerExecutor{}
		}

		_, err := executor.ExecuteCommand("test")
		if err != nil {
			panic(err)
		}
	},
}

// Root command setup
var rootCmd = &cobra.Command{
	Use:   "hs",
	Short: "Home Systems, the cli to automate things at home",
	Long:  `Builds docker images, creates bootable sd images, and more.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Root command code here
		cmd.Help()
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	//
	rootCmd.SetHelpCommand(helpCmd)
	//
	rootCmd.AddCommand(flashCmd)
	rootCmd.AddCommand(nixCmd)
	rootCmd.AddCommand(TestCmd)
	//
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.app.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	//
	flashCmd.PersistentFlags().StringVarP(&isoImage, "iso", "i", "", "Path to the ISO image file")
	flashCmd.PersistentFlags().StringVarP(&device, "device", "d", "", "Device path (e.g., /dev/sdx)")
	//
	nixCmd.AddCommand(nixBuildCmd)
	// NixOps
	rootCmd.AddCommand(nixOpsCmd)
	nixOpsDeployCmd.PersistentFlags().StringVarP(&nixDeployVersion, "version", "n", "", "Version to deploy to or X to redeploy the last")
	nixOpsCmd.AddCommand(nixOpsDeployCmd)
	nixOpsCmd.AddCommand(nixOpsListCmd)
	nixOpsCmd.AddCommand(nixOpsPurgeCmd)
}
