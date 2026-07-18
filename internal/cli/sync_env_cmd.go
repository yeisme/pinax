package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

// addSyncEnvCommands wires the experimental `pinax sync env` command tree
// (init|set|list|unlock|clean|doctor). It is additive: it does not rename or
// remove any existing sync command. The runtime env loader capability is marked
// experimental until dogfood evidence supports stable-from.
func addSyncEnvCommands(parent *cobra.Command, ctx commandBuildContext) {
	envCmd := &cobra.Command{
		Use:   "env",
		Short: "Manage the encrypted runtime dotenv asset (experimental)",
	}

	// init
	envInitCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the encrypted env asset and managed Git ignore",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncEnv(cmd.Context(), app.SyncEnvRequest{VaultPath: *ctx.vaultPath, Action: "init"})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	envCmd.AddCommand(envInitCmd)

	// set
	var envKey, envValue, envProvider string
	envSetCmd := &cobra.Command{
		Use:   "set KEY",
		Short: "Store an encrypted env value (plaintext is transient)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := envKey
			if len(args) == 1 {
				key = args[0]
			}
			projection, err := ctx.svc.SyncEnv(cmd.Context(), app.SyncEnvRequest{VaultPath: *ctx.vaultPath, Action: "set", Key: key, Value: envValue, Provider: envProvider})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	envSetCmd.Flags().StringVar(&envKey, "key", "", "Env key name")
	envSetCmd.Flags().StringVar(&envValue, "value", "", "Plaintext value (transient; encrypted before storage)")
	envSetCmd.Flags().StringVar(&envProvider, "provider", "fake", "Unlock provider: fake or env")
	envCmd.AddCommand(envSetCmd)

	// list
	envListCmd := &cobra.Command{
		Use:   "list",
		Short: "List encrypted env key names (metadata only, no plaintext)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncEnv(cmd.Context(), app.SyncEnvRequest{VaultPath: *ctx.vaultPath, Action: "list"})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	envCmd.AddCommand(envListCmd)

	// unlock
	var envMaterialize bool
	var envAllowlist string
	envUnlockCmd := &cobra.Command{
		Use:   "unlock",
		Short: "Resolve the env snapshot in memory (or materialize with --materialize)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var allowlist []string
			if strings.TrimSpace(envAllowlist) != "" {
				for _, k := range strings.Split(envAllowlist, ",") {
					if k = strings.TrimSpace(k); k != "" {
						allowlist = append(allowlist, k)
					}
				}
			}
			projection, err := ctx.svc.SyncEnv(cmd.Context(), app.SyncEnvRequest{VaultPath: *ctx.vaultPath, Action: "unlock", Materialize: envMaterialize, Allowlist: allowlist})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	envUnlockCmd.Flags().BoolVar(&envMaterialize, "materialize", false, "Write the plaintext env to the managed runtime path (0600)")
	envUnlockCmd.Flags().StringVar(&envAllowlist, "allowlist", "", "Comma-separated keys to materialize (default: all declared)")
	envCmd.AddCommand(envUnlockCmd)

	// clean
	envCleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove only the managed materialized env file",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncEnv(cmd.Context(), app.SyncEnvRequest{VaultPath: *ctx.vaultPath, Action: "clean"})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	envCmd.AddCommand(envCleanCmd)

	// doctor
	envDoctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose env asset, permissions, Git tracked-secret and reload state",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncEnv(cmd.Context(), app.SyncEnvRequest{VaultPath: *ctx.vaultPath, Action: "doctor"})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	envCmd.AddCommand(envDoctorCmd)

	parent.AddCommand(envCmd)
}
