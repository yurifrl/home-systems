package cmd

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/internal/executors"
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
		utils.Flash(device, isoImage, executors.LocalExecutor{})
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
