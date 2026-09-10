package cli

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/yeisme/pinax/internal/inputrequests"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/mcpserver"
	"github.com/yeisme/pinax/internal/mcpserver/sdkruntime"
)

func addMCPCommands(root *cobra.Command, ctx commandBuildContext) {
	mcpCmd := &cobra.Command{Use: "mcp", Short: "Start the Pinax MCP surface"}
	var inputListen, inputBase string
	var collaboration, allowBody, allowWrite bool
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP server over stdio (read-only by default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if (allowBody || allowWrite) && !collaboration {
				return errors.New("note permissions require --collaboration")
			}
			if allowWrite && !allowBody {
				return errors.New("--allow-note-write requires --allow-note-body for review")
			}
			var input *inputrequests.Service
			if inputListen != "" || inputBase != "" {
				if !cmd.Flags().Changed("input-listen") || inputBase == "" {
					return errors.New("both --input-listen and --input-base-url are required")
				}
				parsed, err := url.Parse(inputBase)
				if err != nil || parsed.Host == "" {
					return errors.New("invalid input base URL")
				}
				service, close, err := inputrequests.Open(*ctx.vaultPath, inputBase, ctx.svc)
				if err != nil {
					return err
				}
				defer close()
				input = service
				listener, err := net.Listen("tcp", inputListen)
				if err != nil {
					return err
				}
				httpServer := &http.Server{Handler: input, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
				go func() { _ = httpServer.Serve(listener) }()
				defer func() {
					shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = httpServer.Shutdown(shutdown)
				}()
			}
			options := mcpserver.ServerOptions{
				Input:         input,
				Manifest:      TransportManifestProjection,
				Diagnostics:   cmd.ErrOrStderr(),
				Collaboration: collaboration,
				NotePolicy:    app.CollaborationPolicy{AllowBody: allowBody, AllowWrite: allowWrite},
			}
			// 官方 SDK candidate runtime 仅显式开启；默认仍是既有手写实现
			// （pinax-mcp-official-sdk-v1：默认切换前必须完成真实客户端验收）。
			if os.Getenv("PINAX_MCP_RUNTIME") == sdkruntime.RuntimeName {
				return sdkruntime.Serve(cmd.Context(), ctx.svc, *ctx.vaultPath, os.Stdin, cmd.OutOrStdout(), options)
			}
			return mcpserver.ServeWithOptions(cmd.Context(), ctx.svc, *ctx.vaultPath, os.Stdin, cmd.OutOrStdout(), options)
		},
	}
	serve.Flags().StringVar(&inputListen, "input-listen", "", "Explicit address for the restricted input HTTP listener")
	serve.Flags().StringVar(&inputBase, "input-base-url", "", "Reachable owner URL for one-time input links")
	serve.Flags().BoolVar(&collaboration, "collaboration", false, "Enable Markdown note interaction tools")
	serve.Flags().BoolVar(&allowBody, "allow-note-body", false, "Allow explicit single-note body reads through collaboration tools")
	serve.Flags().BoolVar(&allowWrite, "allow-note-write", false, "Allow preview-bound daily note writes through collaboration tools")
	mcpCmd.AddCommand(serve)
	root.AddCommand(mcpCmd)

}
