package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/yurifrl/home-systems/pkg/nixops"
	"github.com/yurifrl/home-systems/pkg/utils"
)

// NixOps command group
var nixOpsCmd = &cobra.Command{
	Use:   "nixops",
	Short: "TODO",
	Long:  `TODO`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		nctx = utils.NewContext(nixopsWorkdir, verbose)
	},
}

// nixDeployCmd represents the nix deploy command
var nixOpsDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy using NixOps with version increment",
	Long:  `Deploys NixOS configuration using NixOps, auto-incrementing the deployment version.`,
	Run: func(cmd *cobra.Command, args []string) {
		n, err := nixops.NewNixOps()
		if err != nil {
			log.Fatal(err)
		}

		// nctx.ExecuteCommand("nixops", "list")
		nctx.ExecuteCommand("nixops", "create", "-d", "latest")

		uuid := n.GetLatestDeploymentUUID()
		log.Println("nixops deploy -d", uuid)
		// nctx.ExecuteCommand("nixops", "deploy", "-d", uuid)
	},
}

// nixOpsListCmd ...
var nixOpsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List NixOps releases",
	Long:  `Deploys NixOS configuration using NixOps, auto-incrementing the deployment version.`,
	Run: func(cmd *cobra.Command, args []string) {
		n, err := nixops.NewNixOps()
		if err != nil {
			log.Fatal(err)
		}
		n.PrintDeployments()
	},
}

// nixOpsPurgeCmd represents the command to purge NixOps deployments
var nixOpsPurgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Purge all NixOps deployments",
	Long:  `Completely removes all deployment data from the NixOps database.`,
	Run: func(cmd *cobra.Command, args []string) {
		n, err := nixops.NewNixOps()
		if err != nil {
			log.Fatal("Failed to initialize NixOps:", err)
		}
		if err := n.PurgeDatabase(); err != nil {
			log.Fatal("Failed to purge database:", err)
		}
		log.Println("All deployments have been successfully purged from the database.")
	},
}
