package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/k0kubun/pp/v3"
	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/internal/executors"
	"github.com/yurifrl/home-systems/pkg/utils"
)

var distDir = "/src/dist"
var nixBuildString = "./nix/#nixosConfigurations.rpi.config.system.build.sdImage"

// Nix command group
var nixCmd = &cobra.Command{
	Use:   "nix",
	Short: "TODO",
	Long:  `TODO`,
}

// Define the build command
var nixBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Nix package",
	Long:  `Builds a Nix package from the specified configuration.`,
	Run:   runBuild,
}

// Run the build command
func runBuild(cmd *cobra.Command, args []string) {
	executor := &executors.LocalExecutor{}
	stdout, err := executor.ExecuteCommand(
		"nix", "build", nixBuildString,
		"--show-trace",
		"--print-out-paths",
		"--no-link",
		"--json",
	)
	if err != nil {
		fmt.Printf("Error during the build process: %v\n", err)
		os.Exit(1)
	}

	parsedJSON, err := utils.ExtractAndParseJSON(stdout.String())
	if err != nil {
		log.Fatalf("Failed to parse JSON: %s", err)
	}

	fmt.Printf("Nix build output: %+v\n", parsedJSON)

	outputPaths := utils.GetOutputPaths(parsedJSON)
	pp.Println(outputPaths)
	if err := utils.HandleBuildArtifacts(outputPaths, distDir); err != nil {
		fmt.Printf("Failed to handle build artifacts: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Files copied successfully.")
}
