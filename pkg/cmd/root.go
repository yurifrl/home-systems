package cmd

import (
	"github.com/k0kubun/pp/v3"
	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/pkg/utils"
)

var (
	cfgFile        string
	verbose        bool
	nctx           = &utils.Context{}
	image          = "hs"
	currentWorkdir = "."
	dockerWorkdir  = "/src"
	isosDir        = "isos"
	_              = pp.Println
	// Flash
	isoImage = ""
	device   = ""
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
	rootCmd.AddCommand(dockerCmd)
	rootCmd.AddCommand(nixCmd)
	rootCmd.AddCommand(flashCmd)
	rootCmd.AddCommand(findInNetwork)
	//
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.app.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	//
	flashCmd.PersistentFlags().StringVarP(&isoImage, "iso", "i", "", "Path to the ISO image file")
	flashCmd.PersistentFlags().StringVarP(&device, "device", "d", "", "Device path (e.g., /dev/sdx)")
	//
	dockerCmd.AddCommand(dockerBuildCmd)
	dockerCmd.AddCommand(dockerExecCmd)
	dockerCmd.AddCommand(dockerRunCmd)
	//
	nixCmd.AddCommand(nixBuildCmd)
}
