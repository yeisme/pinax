package cli

import (
	"github.com/spf13/cobra"
	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/app"
)

func addSyncRepoMigrateCommands(repoCmd *cobra.Command, ctx commandBuildContext) {
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate an existing device-local sync configuration",
	}
	var unlock, unlockRef, passphraseFile string
	var rememberKeychain, yes bool
	deviceProfileCmd := &cobra.Command{
		Use:   "device-profile",
		Short: "Encrypt the current AWS profile and Capsa content key into the repository envelope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			defaultKeychainRef := ""
			if unlock == "keychain" || rememberKeychain {
				var err error
				defaultKeychainRef, err = defaultRepositoryKeychainRef(*ctx.vaultPath)
				if err != nil {
					// During first migration the envelope does not exist yet, so use
					// the stable runtime workspace identity for remember only after
					// the service creates the envelope.
					defaultKeychainRef = ""
				}
			}
			source, selectedKeychain, err := resolveBootstrapUnlockSource(unlock, unlockRef, passphraseFile, defaultKeychainRef)
			if err != nil {
				return err
			}
			rememberTarget := selectedKeychain
			if rememberKeychain && rememberTarget == nil {
				service, account, err := parseKeychainRef(defaultKeychainRef)
				if err != nil {
					return err
				}
				rememberTarget, err = projectsecrets.NewKeychainSource(service, account)
				if err != nil {
					return err
				}
			}
			projection, err := ctx.svc.SyncRepoMigrateDeviceProfile(cmd.Context(), app.SyncRepoMigrateRequest{
				VaultPath: *ctx.vaultPath, UnlockSource: source, RememberKeychain: rememberTarget, Yes: yes,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	deviceProfileCmd.Flags().StringVar(&unlock, "unlock", "", "Unlock source: prompt|keychain|file|env")
	deviceProfileCmd.Flags().StringVar(&unlockRef, "unlock-ref", "", "Keychain reference keychain://<service>/<account>")
	deviceProfileCmd.Flags().StringVar(&passphraseFile, "passphrase-file", "", "Read unlock passphrase from a 0600 regular file")
	deviceProfileCmd.Flags().BoolVar(&rememberKeychain, "remember-keychain", false, "Persist the verified repository passphrase in macOS Keychain")
	deviceProfileCmd.Flags().BoolVar(&yes, "yes", false, "Confirm migration of the current device profile")
	migrateCmd.AddCommand(deviceProfileCmd)
	repoCmd.AddCommand(migrateCmd)
}
