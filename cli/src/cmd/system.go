package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/src/internal/executors"
	"github.com/yurifrl/home-systems/src/pkg/utils"
)

// flashCmd represents the flash command
var flashCmd = &cobra.Command{
	Use:   "flash",
	Short: "Flash an ISO image to a device",
	Long:  `Flash an ISO image to a specified device.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if the device parameter is provided
		device, _ := cmd.Flags().GetString("device")
		device = "/dev/disk2"

		if device == "" {
			log.Println("Error: Device parameter is required")
			os.Exit(1)
		}
		// Check if the isoImage image parameter is provided, if not, list available ISOs
		isoImage, _ := cmd.Flags().GetString("iso")
		utils.Flash(device, isoImage, executors.DudExecutor{})
	},
}
