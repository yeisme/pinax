package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/app"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
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

func resolveBootstrapUnlockSource(unlock, unlockRef, passphraseFile, defaultKeychainRef string) (projectsecrets.UnlockSource, *projectsecrets.KeychainSource, error) {
	mode := strings.TrimSpace(unlock)
	if mode == "" && strings.TrimSpace(passphraseFile) != "" {
		mode = "file"
	}
	switch mode {
	case "":
		return nil, nil, nil
	case "prompt":
		return projectsecrets.NewPromptSource(projectsecrets.WithPromptText("Repository passphrase: ")), nil, nil
	case "env":
		return projectsecrets.EnvSource("PINAX_REPO_PASS"), nil, nil
	case "file":
		if strings.TrimSpace(passphraseFile) == "" {
			return nil, nil, fmt.Errorf("--unlock file requires --passphrase-file")
		}
		source, err := projectsecrets.FileSource(passphraseFile)
		return source, nil, err
	case "keychain":
		ref := strings.TrimSpace(unlockRef)
		if ref == "" {
			ref = strings.TrimSpace(defaultKeychainRef)
		}
		service, account, err := parseKeychainRef(ref)
		if err != nil {
			return nil, nil, err
		}
		source, err := projectsecrets.NewKeychainSource(service, account)
		return source, source, err
	default:
		return nil, nil, fmt.Errorf("unsupported unlock source %q", mode)
	}
}

// resolveSyncUnlockSource is the shared repository unlock resolver for the
// remote-aware sync commands (diff/pull/push). It implements the task 6.7
// precedence: an explicit --unlock source (prompt/keychain/file/env) wins; when
// only --passphrase-file/--env-var is given it resolves the shortcut source;
// otherwise it returns nil so device-profile mode keeps its legacy path and
// repository-encrypted mode fails closed in the transport rather than falling
// back to a device-local AWS profile.
func resolveSyncUnlockSource(vaultPath, unlock, unlockRef, passphraseFile, envVar string) (projectsecrets.UnlockSource, error) {
	if strings.TrimSpace(unlock) != "" {
		defaultKeychainRef := ""
		if strings.TrimSpace(unlock) == "keychain" {
			ref, err := defaultRepositoryKeychainRef(vaultPath)
			if err != nil {
				return nil, err
			}
			defaultKeychainRef = ref
		}
		source, _, err := resolveBootstrapUnlockSource(unlock, unlockRef, passphraseFile, defaultKeychainRef)
		if err != nil {
			return nil, err
		}
		return source, nil
	}
	return resolveProjectUnlockSource(passphraseFile, envVar), nil
}

func parseKeychainRef(ref string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(ref))
	if err != nil || parsed.Scheme != "keychain" || parsed.Host == "" {
		return "", "", fmt.Errorf("invalid keychain reference; expected keychain://<service>/<account>")
	}
	account, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(account) == "" {
		return "", "", fmt.Errorf("invalid keychain reference; expected keychain://<service>/<account>")
	}
	return parsed.Host, account, nil
}

func defaultRepositoryKeychainRef(vaultPath string) (string, error) {
	root, err := filepath.Abs(vaultPath)
	if err != nil {
		return "", err
	}
	resolver, err := projectsecrets.NewResolver(filepath.Join(root, ".pinax", "project-secrets.yaml"), projectsecrets.StaticSource([]byte("metadata-only")), projectsecrets.DenyAllPolicy{})
	if err != nil {
		return "", err
	}
	info, err := resolver.EnvelopeInfo()
	if err != nil {
		state, stateErr := pinaxremote.Load(root)
		if stateErr != nil || strings.TrimSpace(state.Config.WorkspaceID) == "" {
			return "", err
		}
		account := url.PathEscape("pinax:" + state.Config.WorkspaceID)
		return "keychain://pinax/" + account, nil
	}
	account := url.PathEscape(info.Project + ":" + info.Repository)
	return "keychain://pinax/" + account, nil
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
