package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/yurifrl/home-systems/pkg/nixops"
	"github.com/yurifrl/home-systems/pkg/utils"
)

// Global variables
var (
	cfgFile        string
	verbose        bool
	image          = "hs"
	workdir        = "."
	isosDir        = "isos"
	dockerfilePath = "docker"
	nixopsWorkdir  = "/workdir/nix/nixops/"
	redeploy       = false

	// Flash
	isoImage = ""
	device   = ""
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

// Docker command group
var dockerCmd = &cobra.Command{
	Use:   "docker",
	Short: "Docker operations from the builder machine",
	Long:  `Commands to manage Docker containers.`,
}

// Nix command group
var nixCmd = &cobra.Command{
	Use:   "nix",
	Short: "TODO",
	Long:  `TODO`,
}

// Docker run command
var dockerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Docker container",
	Long:  `Runs a Docker container from the specified image.`,
	Run: func(cmd *cobra.Command, args []string) {
		// executeCommand("docker", "volume", "create", "nixops")
		executeCommand("docker", "run", "-it", "--rm", "-v", "nixops:/nixops", "-v", "./secrets:/etc/secrets", "-v", ".:/workdir", "--entrypoint=fish", image)
		// executeCommand("docker", "run", "--rm", "--entrypoint=fish", "-it", "-v", ".:/workdir", image)
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

// Build nix image
var nixBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build Nix package",
	Long:  `Builds a Nix package from the specified configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		executeCommand("nix-build", "--show-trace", "<nixpkgs/nixos>", "-A", "config.system.build.sdImage", "-I", "nixos-config=/workdir/nix/sd-image.nix", "--argstr", "system", "aarch64-linux")
	},
}

// nixDeployCmd represents the nix deploy command
var nixDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy using NixOps with version increment",
	Long:  `Deploys NixOS configuration using NixOps, auto-incrementing the deployment version.`,
	Run: func(cmd *cobra.Command, args []string) {
		nixosData, err := nixops.FetchEverything()
		if err != nil {
			log.Fatal(err)
		}

		// Use the fetched data as needed
		// For example, printing some of it:
		fmt.Println(nixosData)
		// versionInfo := utils.GetLatestVersion(nixopsWorkdir)
		// var version = ""
		// if redeploy {
		// 	fmt.Println("Redeploying version:", versionInfo.OldVersion)
		// 	version = versionInfo.UUID
		// } else {
		// 	fmt.Println("Deploying version:", versionInfo.NewVersion)
		// 	version = versionInfo.NewVersion
		// }
		// panic("ono")

		// // Nixops Create
		// nixCmd := exec.Command("nixops", "create", "-d", version)
		// nixCmd.Dir = filepath.Clean(workdir)

		// // Execute the command
		// _, err := nixCmd.Output()
		// if err != nil {
		// 	fmt.Println("Error executing nixops create:", err)
		// }

		// // Nixops Create
		// nixCmd = exec.Command("nixops", "deploy", "-d", version)
		// nixCmd.Dir = filepath.Clean(workdir)

		// // Execute the command
		// _, err = nixCmd.Output()
		// if err != nil {
		// 	fmt.Println("Error executing nixops deploy:", err)
		// }
	},
}

// Find connectable devices in network
var findInNetwork = &cobra.Command{
	Use:   "find-in-network",
	Short: "TODO",
	Long:  `TODO`,
	Run: func(cmd *cobra.Command, args []string) {
		subnet := "192.168.1."
		for i := 1; i <= 255; i++ {
			go utils.ScanAddress(fmt.Sprintf("%s%d", subnet, i))
		}

		// Wait to prevent the program from exiting immediately
		// In a real-world scenario, use proper synchronization
		time.Sleep(5 * time.Minute)
	},
}

// flashCmd represents the flash command
var flashCmd = &cobra.Command{
	Use:   "flash",
	Short: "Flash an ISO image to a device",
	Long:  `Flash an ISO image to a specified device.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if the device parameter is provided
		device, _ := cmd.Flags().GetString("device")
		if device == "" {
			fmt.Println("Error: Device parameter is required")
			os.Exit(1)
		}

		// Check if the isoImage image parameter is provided, if not, list available ISOs
		isoImage, _ := cmd.Flags().GetString("iso")

		// Code that can go to a function
		if isoImage == "" {
			isoFiles, err := filepath.Glob(filepath.Join(isosDir, "*.img"))
			if err != nil {
				fmt.Println("Error listing ISO images files:", err)
				return
			}
			if len(isoFiles) == 0 {
				fmt.Println("No ISO images files found in", isosDir)
				return
			}
			// Sort and display ISO files for user to select
			sort.Strings(isoFiles)
			for i, file := range isoFiles {
				fmt.Printf("%d: %s\n", i+1, file)
			}
			fmt.Print("Enter the number of the ISO images file to flash: ")
			var choice int
			fmt.Scanln(&choice)
			if choice < 1 || choice > len(isoFiles) {
				fmt.Println("Invalid choice")
				return
			}
			isoImage = isoFiles[choice-1]
		}
		comand := []string{"sudo", "dd", "bs=4M", "status=progress", "conv=fsync", "of=" + device, "if=" + isoImage}

		// Prompt user for confirmation before proceeding
		fmt.Println(strings.Join(comand, " "))
		fmt.Println()
		fmt.Printf("Are you sure you want to flash '%s' to '%s'? This will erase all data on the device. Type 'y' to confirm: ", isoImage, device)
		var confirmation string
		fmt.Scanln(&confirmation)
		if confirmation != "y" {
			fmt.Println("Flash operation cancelled.")
			return
		}
		executeCommand("diskutil", "unmountDisk", "/dev/disk2")
		// Execute the dd command to flash the ISO to the device
		// executeCommand("sudo", "dd", "bs=4M", "status=progress", "conv=fsync", "of="+device, "if="+isoImage)
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
	nixDeployCmd.PersistentFlags().BoolVarP(&redeploy, "redeploy", "r", false, "Redeploy the last version")
	//
	dockerCmd.AddCommand(dockerBuildCmd)
	dockerCmd.AddCommand(dockerRunCmd)
	//
	nixCmd.AddCommand(nixBuildCmd)
	nixCmd.AddCommand(nixDeployCmd)
}

// /nix/store/dgsinnmdaak3gjh9pqlbgwfpzbia5h2m-nixos-sd-image-24.05pre568310.eabe8d3eface-aarch64-linux.img/sd-image/nixos-sd-image-24.05pre568310.eabe8d3eface-aarch64-linux.img nixos-sd-image-24.05pre568310.eabe8d3eface-aarch64-linux.img
