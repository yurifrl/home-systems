package cli

import (
	"github.com/spf13/cobra"

	"github.com/yurifrl/nostos/internal/mcp"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run a JSON-RPC MCP server over stdio (one tool per cobra command)",
		RunE: runEFunc(func(cmd *cobra.Command, args []string) error {
			s := mcp.NewServer(cmd.Root())
			return s.Serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		}),
	}
}
