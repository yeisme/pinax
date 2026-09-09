package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/credentialctl/pkg/projectsecrets"
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
	var secretStdin, secretValueUsed bool
	secretSetCmd := &cobra.Command{
		Use:   "set",
		Short: "Store an encrypted secret value",
		RunE: func(cmd *cobra.Command, args []string) error {
			plaintext := secretPlaintext
			if secretStdin {
				data, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				plaintext = strings.TrimRight(string(data), "\n")
			}
			// Deprecation warning for --value: prefer --stdin (or, for S3/COS
			// credentials, `sync repo credential set`). The warning goes to the
			// command's stderr so it never pollutes machine-readable stdout. The
			// flag is kept for at least two minor releases (earliest removal v0.4.0).
			if secretValueUsed && !secretStdin {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning: --value is deprecated; use --stdin (or `pinax sync repo credential set` for S3/COS bundles). --value is retained for compatibility and will be removed no earlier than v0.4.0.")
			}
			projection, err := ctx.svc.SyncRepoSecret(cmd.Context(), app.SyncRepoSecretRequest{
				VaultPath: *ctx.vaultPath,
				Action:    "set",
				Name:      secretName,
				Identity:  secretIdentity,
				Kind:      secretKind,
				Plaintext: plaintext,
				Provider:  secretProvider,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	secretSetCmd.Flags().StringVar(&secretName, "name", "", "Secret logical name")
	secretSetCmd.Flags().StringVar(&secretIdentity, "identity", "", "Logical identity (defaults to name)")
	secretSetCmd.Flags().StringVar(&secretKind, "kind", "encryption_key", "Secret kind: encryption_key or credential")
	secretSetCmd.Flags().StringVar(&secretPlaintext, "value", "", "Plaintext value (deprecated; prefer --stdin)")
	secretSetCmd.Flags().BoolVar(&secretStdin, "stdin", false, "Read plaintext from stdin (preferred)")
	secretSetCmd.Flags().StringVar(&secretProvider, "provider", "fake", "Unlock provider: fake or env")
	secretSetCmd.PreRun = func(cmd *cobra.Command, args []string) { secretValueUsed = cmd.Flags().Changed("value") }
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

	// credential (typed repository-encrypted S3/COS bundle via credentialctl)
	addSyncRepoCredentialCommands(repoCmd, ctx)
	addSyncRepoMigrateCommands(repoCmd, ctx)

	// bootstrap
	var bootstrapDevice, bootstrapUnlock, bootstrapUnlockRef, bootstrapPassphraseFile string
	var bootstrapRememberKeychain, bootstrapPull bool
	repoBootstrapCmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Bootstrap a new device from the declaration (pull-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			defaultKeychainRef := ""
			if bootstrapUnlock == "keychain" || bootstrapRememberKeychain {
				var err error
				defaultKeychainRef, err = defaultRepositoryKeychainRef(*ctx.vaultPath)
				if err != nil {
					return err
				}
			}
			projectSource, selectedKeychain, err := resolveBootstrapUnlockSource(bootstrapUnlock, bootstrapUnlockRef, bootstrapPassphraseFile, defaultKeychainRef)
			if err != nil {
				return err
			}
			rememberTarget, err := resolveRememberKeychain(bootstrapRememberKeychain, selectedKeychain, defaultKeychainRef)
			if err != nil {
				return err
			}
			projection, err := ctx.svc.SyncRepoBootstrap(cmd.Context(), app.SyncRepoRuntimeRequest{
				VaultPath:           *ctx.vaultPath,
				DeviceID:            bootstrapDevice,
				Yes:                 *ctx.yes,
				ProjectUnlockSource: projectSource,
				RememberKeychain:    rememberTarget,
				Pull:                bootstrapPull,
			})
			if err == nil {
				if cmd.Flags().Changed("unlock") {
					projection.Facts["unlock"] = bootstrapUnlock
				}
				if cmd.Flags().Changed("unlock-ref") {
					projection.Facts["unlock_ref"] = "configured"
				}
				if cmd.Flags().Changed("passphrase-file") {
					projection.Facts["passphrase_file"] = "configured"
				}
			}
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	repoBootstrapCmd.Flags().StringVar(&bootstrapDevice, "device", "", "Unique device id for this machine")
	repoBootstrapCmd.Flags().BoolVar(ctx.yes, "yes", false, "Confirm high-risk changes")
	repoBootstrapCmd.Flags().StringVar(&bootstrapUnlock, "unlock", "", "Unlock source: prompt|keychain|file|env (delegated to credentialctl)")
	repoBootstrapCmd.Flags().StringVar(&bootstrapUnlockRef, "unlock-ref", "", "Keychain unlock reference keychain://<service>/<account>")
	repoBootstrapCmd.Flags().StringVar(&bootstrapPassphraseFile, "passphrase-file", "", "Read unlock passphrase from a 0600 regular file")
	repoBootstrapCmd.Flags().BoolVar(&bootstrapRememberKeychain, "remember-keychain", false, "Persist the unlock secret in a repository-scoped macOS Keychain item (requires explicit approval)")
	repoBootstrapCmd.Flags().BoolVar(&bootstrapPull, "pull", false, "After compiling the pull-only runtime, pull and decrypt the remote revision (default: compile-only)")
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

// resolveRememberKeychain returns a write target only when the caller
// explicitly requested persistence. A keychain unlock source is read-only
// unless --remember-keychain is set.
func resolveRememberKeychain(remember bool, selected *projectsecrets.KeychainSource, defaultKeychainRef string) (*projectsecrets.KeychainSource, error) {
	if !remember {
		return nil, nil
	}
	if selected != nil {
		return selected, nil
	}
	service, account, err := parseKeychainRef(defaultKeychainRef)
	if err != nil {
		return nil, err
	}
	return projectsecrets.NewKeychainSource(service, account)
}
