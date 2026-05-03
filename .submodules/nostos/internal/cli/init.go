package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const initTemplate = `# nostos config.yaml
cluster:
  name: talos-default
  endpoint: https://192.168.68.100:6443
  talos_version: v1.10.3
  # Get schematic ID from https://factory.talos.dev
  schematic_id: REPLACE-ME-64-HEX-CHARS

secrets:
  backend: onepassword
  onepassword:
    account: my.1password.com
    vault: kubernetes

nodes: {}
  # dell01:
  #   mac: "d0:94:66:d9:eb:a5"
  #   ip: 192.168.68.100
  #   role: controlplane
  #   arch: amd64
  #   install_disk: /dev/nvme0n1
  #   template: dell01.yaml
`

const stateGitignore = `# everything in here is a cache
*
!.gitignore
`

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init [DIR]",
		Short: "Scaffold config.yaml, templates/, state/ in DIR (default: .)",
		Args:  cobra.MaximumNArgs(1),
		RunE: runEFuncSimple(func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(abs, 0o755); err != nil {
				return err
			}

			cfg := filepath.Join(abs, "config.yaml")
			if _, err := os.Stat(cfg); err == nil && !force {
				return fmt.Errorf("%s already exists. Use --force to overwrite", cfg)
			}
			if err := os.WriteFile(cfg, []byte(initTemplate), 0o600); err != nil {
				return err
			}

			if err := os.MkdirAll(filepath.Join(abs, "templates"), 0o755); err != nil {
				return err
			}
			state := filepath.Join(abs, "state")
			if err := os.MkdirAll(state, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(state, ".gitignore"), []byte(stateGitignore), 0o644); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Initialized nostos project at %s\n", abs)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing config.yaml")
	return cmd
}
