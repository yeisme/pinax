package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

func addShareCommands(root *cobra.Command, ctx commandBuildContext) {
	var scope string
	var profile string
	var out string
	var host string
	var port int
	var allowLAN bool
	var readonly bool
	var noAuth bool
	var tokenFile string
	var once bool

	shareCmd := &cobra.Command{
		Use:   "share",
		Short: "Expose local read-only Pinax views",
	}
	shareStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Prepare a read-only local or LAN share endpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.ShareStart(cmd.Context(), app.ShareRequest{VaultPath: *ctx.vaultPath, Profile: profile, Out: out, Scope: scope, Host: host, Port: port, AllowLAN: allowLAN, Readonly: readonly, NoAuth: noAuth, TokenFile: tokenFile, Once: once})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	shareStartCmd.Flags().StringVar(&scope, "scope", "published", "Share scope: published or vault-readonly")
	shareStartCmd.Flags().StringVar(&profile, "profile", "", "Publish profile name for published share")
	shareStartCmd.Flags().StringVar(&out, "out", "", "Publish output directory for published share")
	shareStartCmd.Flags().StringVar(&host, "host", "127.0.0.1", "Host to bind or advertise")
	shareStartCmd.Flags().IntVar(&port, "port", 8787, "Port for share endpoint, or 0 for an available port")
	shareStartCmd.Flags().BoolVar(&allowLAN, "allow-lan", false, "Allow non-loopback LAN share hosts")
	shareStartCmd.Flags().BoolVar(&readonly, "readonly", false, "Require read-only share mode")
	shareStartCmd.Flags().BoolVar(&noAuth, "no-auth", false, "Allow no auth on loopback only")
	shareStartCmd.Flags().StringVar(&tokenFile, "token-file", "", "Token file for authenticated share access")
	shareStartCmd.Flags().BoolVar(&once, "once", false, "Serve one share smoke request set and exit")
	shareCmd.AddCommand(shareStartCmd)
	root.AddCommand(shareCmd)
}
