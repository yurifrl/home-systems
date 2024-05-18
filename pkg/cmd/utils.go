package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/pkg/utils"
)

// flashCmd represents the flash command
var flashCmd = &cobra.Command{
	Use:   "flash",
	Short: "Flash an ISO image to a device",
	Long:  `Flash an ISO image to a specified device.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if the device parameter is provided
		device, _ := cmd.Flags().GetString("device")
		if device == "" {
			log.Println("Error: Device parameter is required")
			os.Exit(1)
		}

		// Check if the isoImage image parameter is provided, if not, list available ISOs
		isoImage, _ := cmd.Flags().GetString("iso")

		// Code that can go to a function
		if isoImage == "" {
			isoFiles, err := filepath.Glob(filepath.Join(isosDir, "*.img"))
			if err != nil {
				log.Println("Error listing ISO images files:", err)
				return
			}
			if len(isoFiles) == 0 {
				log.Println("No ISO images files found in", isosDir)
				return
			}
			// Sort and display ISO files for user to select
			sort.Strings(isoFiles)
			for i, file := range isoFiles {
				log.Printf("%d: %s\n", i+1, file)
			}
			log.Print("Enter the number of the ISO images file to flash: ")
			var choice int
			fmt.Scanln(&choice)
			if choice < 1 || choice > len(isoFiles) {
				log.Println("Invalid choice")
				return
			}
			isoImage = isoFiles[choice-1]
		}
		comand := []string{"sudo", "dd", "bs=4M", "status=progress", "conv=fsync", "of=" + device, "if=" + isoImage}

		// Prompt user for confirmation before proceeding
		log.Println(strings.Join(comand, " "))
		log.Println()
		log.Printf("Are you sure you want to flash '%s' to '%s'? This will erase all data on the device. Type 'y' to confirm: ", isoImage, device)
		var confirmation string
		fmt.Scanln(&confirmation)
		if confirmation != "y" {
			log.Println("Flash operation cancelled.")
			return
		}
		nctx.ExecuteCommand("diskutil", "unmountDisk", "/dev/disk2")
		// Execute the dd command to flash the ISO to the device
		// executeCommand("sudo", "dd", "bs=4M", "status=progress", "conv=fsync", "of="+device, "if="+isoImage)
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
