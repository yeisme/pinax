package cli

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/mcpserver"
)

func addMCPCommands(root *cobra.Command, ctx commandBuildContext) {
	mcpCmd := &cobra.Command{Use: "mcp", Short: "Start the Pinax MCP surface"}
	mcpCmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Start the read-only MCP server over stdio",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpserver.ServeWithOptions(cmd.Context(), ctx.svc, *ctx.vaultPath, os.Stdin, cmd.OutOrStdout(), mcpserver.ServerOptions{
				Manifest:    TransportManifestProjection,
				Diagnostics: cmd.ErrOrStderr(),
			})
		},
	})
	root.AddCommand(mcpCmd)

}
