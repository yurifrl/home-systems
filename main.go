package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Global variables
var (
	cfgFile string
	verbose bool
	image   = "nixos-sd-image"
	workdir = "."
)

// ExecuteCommand runs a command with given arguments.
func executeCommand(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = filepath.Join(".", workdir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if verbose {
		cmdStr := fmt.Sprintf("Executing command: `%s %s`", name, strings.Join(args, " "))
		fmt.Println(cmdStr)
	}
	if err := cmd.Run(); err != nil {
		if verbose {
			fmt.Printf("Error executing command: %s\n", err)
		} else {
			fmt.Printf("Error: command failed\n")
		}
	}
}

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

// Docker run command
var dockerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Docker container",
	Long:  `Runs a Docker container from the specified image.`,
	Run: func(cmd *cobra.Command, args []string) {
		executeCommand("docker", "run", "-it", "--rm", "-v", ".:/workdir", "--entrypoint", "bash", image)
	},
}

// Docker build command
var dockerBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Docker image",
	Long:  `Builds a Docker image from the specified Dockerfile.`,
	Run: func(cmd *cobra.Command, args []string) {
		executeCommand("docker", "build", "-t", image, ".")
	},
}

// Docker command group
var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Docker operations",
	Long:  `Commands to manage Docker containers.`,
}

// Build nix image
var buildNixCmd = &cobra.Command{
	Use:   "build-nix",
	Short: "Build Nix package",
	Long:  `Builds a Nix package from the specified configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		executeCommand("nix-build", "<nixpkgs/nixos>", "-A", "config.system.build.sdImage", "-I", "nixos-config=sd-image.nix", "--argstr", "system", "aarch64-linux")
	},
}

// Initialize config
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

// Main entry point of the application
func main() {
	cobra.OnInitialize(initConfig)
	if err := Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.app.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.AddCommand(dockerCmd)
	rootCmd.SetHelpCommand(helpCmd)
	dockerCmd.AddCommand(dockerBuildCmd)
	dockerCmd.AddCommand(dockerRunCmd)
}
