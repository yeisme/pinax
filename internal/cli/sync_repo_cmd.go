package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/pinax/internal/app"
)

// addSyncRepoCommands wires the experimental `pinax sync repo` command tree
// (init|secret|bootstrap|plan|apply|doctor). It is additive: it does not rename
// or remove any existing sync command. The declaration layer is marked
// experimental until dogfood evidence supports stable-from.
func addSyncRepoCommands(parent *cobra.Command, ctx commandBuildContext) {
	repoCmd := &cobra.Command{
		Use:   "repo",
		Short: "Manage the declarative sync repository configuration (experimental)",
	}

	// init
	var initBackend, initEndpoint, initWorkspace, initTenant, initApp, initCredID, initKeyID, initRemoteDelete string
	repoInitCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize or update the repository sync declaration",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncRepoInit(cmd.Context(), app.SyncRepoInitRequest{
				VaultPath:         *ctx.vaultPath,
				BackendKind:       initBackend,
				Endpoint:          initEndpoint,
				WorkspaceID:       initWorkspace,
				TenantID:          initTenant,
				AppID:             initApp,
				CredentialID:      initCredID,
				EncryptionKeyID:   initKeyID,
				RemoteDelete:      initRemoteDelete,
				ExistingIfPresent: true,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repoInitCmd.Flags().StringVar(&initBackend, "backend-kind", "s3-direct", "Backend kind: s3-direct, rclone-direct, server, embedded")
	repoInitCmd.Flags().StringVar(&initEndpoint, "endpoint", "", "Backend endpoint topology URI (e.g. s3://bucket/prefix)")
	repoInitCmd.Flags().StringVar(&initWorkspace, "workspace", "", "Workspace id (isolation boundary)")
	repoInitCmd.Flags().StringVar(&initTenant, "tenant", "", "Tenant id (namespace fact; not server RBAC)")
	repoInitCmd.Flags().StringVar(&initApp, "app", "", "App id (namespace fact)")
	repoInitCmd.Flags().StringVar(&initCredID, "credential-id", "", "Logical credential identity (resolved per device)")
	repoInitCmd.Flags().StringVar(&initKeyID, "encryption-key-id", "", "Logical encryption key identity (required)")
	repoInitCmd.Flags().StringVar(&initRemoteDelete, "remote-delete-policy", "deny", "Remote delete policy: deny or require-approval")
	repoCmd.AddCommand(repoInitCmd)

	// secret
	secretCmd := &cobra.Command{Use: "secret", Short: "Manage encrypted repository secrets"}
	var secretName, secretIdentity, secretKind, secretPlaintext, secretProvider string
	secretSetCmd := &cobra.Command{
		Use:   "set",
		Short: "Store an encrypted secret value",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncRepoSecret(cmd.Context(), app.SyncRepoSecretRequest{
				VaultPath: *ctx.vaultPath,
				Action:    "set",
				Name:      secretName,
				Identity:  secretIdentity,
				Kind:      secretKind,
				Plaintext: secretPlaintext,
				Provider:  secretProvider,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	secretSetCmd.Flags().StringVar(&secretName, "name", "", "Secret logical name")
	secretSetCmd.Flags().StringVar(&secretIdentity, "identity", "", "Logical identity (defaults to name)")
	secretSetCmd.Flags().StringVar(&secretKind, "kind", "encryption_key", "Secret kind: encryption_key or credential")
	secretSetCmd.Flags().StringVar(&secretPlaintext, "value", "", "Plaintext value (transient; encrypted before storage)")
	secretSetCmd.Flags().StringVar(&secretProvider, "provider", "fake", "Unlock provider: fake or env")
	secretCmd.AddCommand(secretSetCmd)

	secretListCmd := &cobra.Command{
		Use:   "list",
		Short: "List encrypted secrets (metadata only, no plaintext)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncRepoSecret(cmd.Context(), app.SyncRepoSecretRequest{VaultPath: *ctx.vaultPath, Action: "list"})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	secretCmd.AddCommand(secretListCmd)

	var secretRemoveName string
	secretRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove an encrypted secret",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncRepoSecret(cmd.Context(), app.SyncRepoSecretRequest{VaultPath: *ctx.vaultPath, Action: "remove", Name: secretRemoveName})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	secretRemoveCmd.Flags().StringVar(&secretRemoveName, "name", "", "Secret logical name")
	secretCmd.AddCommand(secretRemoveCmd)
	repoCmd.AddCommand(secretCmd)

	// bootstrap
	var bootstrapDevice string
	repoBootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap a new device from the declaration (pull-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncRepoBootstrap(cmd.Context(), app.SyncRepoRuntimeRequest{VaultPath: *ctx.vaultPath, DeviceID: bootstrapDevice, Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repoBootstrapCmd.Flags().StringVar(&bootstrapDevice, "device", "", "Unique device id for this machine")
	repoBootstrapCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm high-risk changes")
	repoCmd.AddCommand(repoBootstrapCmd)

	// plan
	var planDevice string
	repoPlanCmd := &cobra.Command{
		Use:   "plan",
		Short: "Report intended sync config changes without writing",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncRepoPlan(cmd.Context(), app.SyncRepoRuntimeRequest{VaultPath: *ctx.vaultPath, DeviceID: planDevice})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repoPlanCmd.Flags().StringVar(&planDevice, "device", "", "Device id to plan for")
	repoCmd.AddCommand(repoPlanCmd)

	// apply
	var applyDevice string
	repoApplyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Regenerate the local runtime config from the declaration",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncRepoApply(cmd.Context(), app.SyncRepoRuntimeRequest{VaultPath: *ctx.vaultPath, DeviceID: applyDevice, Yes: *ctx.yes})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repoApplyCmd.Flags().StringVar(&applyDevice, "device", "", "Device id (defaults to existing runtime device)")
	repoApplyCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm high-risk changes (workspace, namespace, key identity, remote delete)")
	repoCmd.AddCommand(repoApplyCmd)

	// doctor
	repoDoctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose declaration/runtime drift, key identity and device state",
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncRepoDoctor(cmd.Context(), app.VaultRequest{VaultPath: *ctx.vaultPath})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repoCmd.AddCommand(repoDoctorCmd)

	parent.AddCommand(repoCmd)
}
