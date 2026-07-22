package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/app"
)

// resolveProjectUnlockSource builds a credentialctl unlock source from the
// sync pull credential flags. It prefers an explicit passphrase file, then an
// env var; when neither is set it returns nil so the sync run falls back to
// the device-profile credential path.
func resolveProjectUnlockSource(passphraseFile, envVar string) projectsecrets.UnlockSource {
	if strings.TrimSpace(passphraseFile) != "" {
		if src, err := projectsecrets.FileSource(passphraseFile); err == nil {
			return src
		}
	}
	if strings.TrimSpace(envVar) != "" {
		return projectsecrets.EnvSource(envVar)
	}
	return nil
}

// addSyncRepoCredentialCommands wires `pinax sync repo credential
// init|set|list|remove`. These manage the typed repository-encrypted S3/COS
// credential bundle (s3_credentials.v1) through credentialctl's projectsecrets
// API. Plaintext flows only through secure stdin; structured output carries
// metadata only.
func addSyncRepoCredentialCommands(repoCmd *cobra.Command, ctx commandBuildContext) {
	credentialCmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage typed repository-encrypted S3/COS credentials (passphrase/Keychain unlock)",
	}
	var (
		credName, credKind, credFormat, credVersion string
		credStdin                                   bool
		credPassphraseFile, credEnvVar              string
		credProject, credRepository                 string
	)
	// resolveCredentialPassphrase builds the unlock source for credential
	// mutations, preferring file/env so the commands work in CI without a TTY.
	resolveCredentialPassphrase := func() ([]byte, error) {
		if credPassphraseFile != "" {
			src, err := projectsecrets.FileSource(credPassphraseFile)
			if err != nil {
				return nil, err
			}
			return src.Secret(context.TODO())
		}
		if credEnvVar != "" {
			return projectsecrets.EnvSource(credEnvVar).Secret(context.TODO())
		}
		return nil, fmt.Errorf("passphrase source required: use --passphrase-file or --env-var (prompt is not yet wired for credential commands)")
	}

	credentialInitCmd := &cobra.Command{
		Use:   "init",
		Short: "Create the repository-encrypted credential envelope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pass, err := resolveCredentialPassphrase()
			if err != nil {
				return err
			}
			projection, err := ctx.svc.SyncRepoCredential(cmd.Context(), app.SyncRepoCredentialRequest{
				VaultPath:  *ctx.vaultPath,
				Action:     "init",
				Passphrase: pass,
				Project:    credProject,
				Repository: credRepository,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	credentialInitCmd.Flags().StringVar(&credPassphraseFile, "passphrase-file", "", "Read unlock passphrase from a 0600 regular file")
	credentialInitCmd.Flags().StringVar(&credEnvVar, "env-var", "", "Read unlock passphrase from a named environment variable")
	credentialInitCmd.Flags().StringVar(&credProject, "project", "pinax", "Project identity")
	credentialInitCmd.Flags().StringVar(&credRepository, "repository", "", "Repository identity (defaults to vault dir name)")
	credentialCmd.AddCommand(credentialInitCmd)

	credentialSetCmd := &cobra.Command{
		Use:   "set",
		Short: "Encrypt one s3_credentials.v1 bundle (payload via --stdin) into the repository envelope",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !credStdin {
				return fmt.Errorf("credential payload must be provided via --stdin; value flags are rejected")
			}
			payload, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}
			if len(payload) == 0 {
				return fmt.Errorf("empty credential payload on stdin")
			}
			pass, err := resolveCredentialPassphrase()
			if err != nil {
				return err
			}
			projection, err := ctx.svc.SyncRepoCredential(cmd.Context(), app.SyncRepoCredentialRequest{
				VaultPath:  *ctx.vaultPath,
				Action:     "set",
				Name:       credName,
				Kind:       credKind,
				Format:     credFormat,
				Version:    credVersion,
				Payload:    payload,
				Passphrase: pass,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	credentialSetCmd.Flags().StringVar(&credName, "name", "", "Credential logical name (required)")
	credentialSetCmd.Flags().StringVar(&credKind, "kind", "credential", "Credential kind")
	credentialSetCmd.Flags().StringVar(&credFormat, "format", "s3_credentials.v1", "Credential format")
	credentialSetCmd.Flags().StringVar(&credVersion, "version", "1", "Credential version")
	credentialSetCmd.Flags().BoolVar(&credStdin, "stdin", false, "Read the s3_credentials.v1 JSON payload from stdin (required)")
	credentialSetCmd.Flags().StringVar(&credPassphraseFile, "passphrase-file", "", "Read unlock passphrase from a 0600 regular file")
	credentialSetCmd.Flags().StringVar(&credEnvVar, "env-var", "", "Read unlock passphrase from a named environment variable")
	credentialCmd.AddCommand(credentialSetCmd)

	credentialListCmd := &cobra.Command{
		Use:   "list",
		Short: "List repository credential bundle metadata (no plaintext)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			projection, err := ctx.svc.SyncRepoCredential(cmd.Context(), app.SyncRepoCredentialRequest{
				VaultPath: *ctx.vaultPath,
				Action:    "list",
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	credentialCmd.AddCommand(credentialListCmd)

	credentialRemoveCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove one repository credential bundle",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			pass, err := resolveCredentialPassphrase()
			if err != nil {
				return err
			}
			projection, err := ctx.svc.SyncRepoCredential(cmd.Context(), app.SyncRepoCredentialRequest{
				VaultPath:  *ctx.vaultPath,
				Action:     "remove",
				Name:       credName,
				Passphrase: pass,
			})
			return ctx.renderProjection(cmd, projection, err)
		},
	}
	credentialRemoveCmd.Flags().StringVar(&credName, "name", "", "Credential logical name to remove (required)")
	credentialRemoveCmd.Flags().StringVar(&credPassphraseFile, "passphrase-file", "", "Read unlock passphrase from a 0600 regular file")
	credentialRemoveCmd.Flags().StringVar(&credEnvVar, "env-var", "", "Read unlock passphrase from a named environment variable")
	credentialCmd.AddCommand(credentialRemoveCmd)

	repoCmd.AddCommand(credentialCmd)
}
