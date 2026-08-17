package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/domain"
	pinaxprofile "github.com/yeisme/pinax/internal/profile"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

// SyncRepoMigrateRequest converts the current device-profile runtime into the
// portable repository-encrypted declaration and envelope without remote writes.
type SyncRepoMigrateRequest struct {
	VaultPath        string
	UnlockSource     projectsecrets.UnlockSource
	RememberKeychain *projectsecrets.KeychainSource
	Yes              bool
}

// syncRepoMigrationPlan is the read-only, plan-first evaluation of a device-
// profile migration (task 6.9). It carries the identity the migration would
// produce plus the already_migrated / migration_conflict outcomes, so the
// orchestrator can decide re-entry or conflict without writing anything.
type syncRepoMigrationPlan struct {
	state           pinaxremote.State
	profileName     string
	credentials     pinaxprofile.AWSStaticCredentials
	contentKey      string
	contentKeyID    string
	credentialID    string
	encryptionID    string
	workspaceID     string
	alreadyMigrated bool
	conflictReason  string
}

// SyncRepoMigrateDeviceProfile is plan-first, atomic and re-entrant
// (pinax-passphrase-s3-bootstrap task 6.9): it evaluates the migration without
// writing, returns already_migrated=true for a same-identity re-run, returns
// migration_conflict when the existing declaration/envelope cannot be verified,
// and only then applies the migration with envelope-before-declaration ordering
// and an atomic declaration rename so any failure leaves the previous runtime,
// declaration and envelope usable.
func (s *Service) SyncRepoMigrateDeviceProfile(ctx context.Context, req SyncRepoMigrateRequest) (domain.Projection, error) {
	const command = "sync.repo.migrate.device-profile"
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	if !req.Yes {
		ce := &domain.CommandError{Code: "approval_required", Message: "device-profile migration requires explicit confirmation", Hint: "Re-run with --yes after reviewing the migration plan."}
		return domain.NewErrorProjection(command, ce), ce
	}
	plan, planErr := planSyncRepoMigration(ctx, root, req)
	if planErr != nil {
		var ce *domain.CommandError
		if errors.As(planErr, &ce) {
			return domain.NewErrorProjection(command, ce), ce
		}
		return errorProjection(command, planErr), planErr
	}
	if plan.conflictReason != "" {
		ce := &domain.CommandError{Code: "migration_conflict", Message: plan.conflictReason, Hint: "Reconcile or rotate the existing repository-encrypted identity before re-running migration."}
		proj := domain.NewErrorProjection(command, ce)
		proj.Facts["remote_write"] = "false"
		proj.Facts["key_rotated"] = "false"
		return proj, ce
	}
	if plan.alreadyMigrated {
		proj := domain.NewProjection(command, "Device-profile sync configuration is already migrated to a repository-encrypted envelope.")
		proj.Facts["already_migrated"] = "true"
		proj.Facts["credential_mode"] = pinaxremote.CredentialModeRepositoryEncrypted
		proj.Facts["credential_id"] = plan.credentialID
		proj.Facts["encryption_key_id"] = plan.encryptionID
		proj.Facts["remote_write"] = "false"
		proj.Facts["key_rotated"] = "false"
		proj.Facts["content_key_id"] = plan.contentKeyID
		return proj, nil
	}
	return applySyncRepoMigration(ctx, command, root, req, plan)
}

// planSyncRepoMigration evaluates the migration source and the existing
// declaration/envelope WITHOUT writing. It reads the current s3-direct runtime,
// AWS shared profile and content encryption key, then decides:
//   - same identity + envelope verifiable with the supplied passphrase  -> already_migrated
//   - same identity but envelope not verifiable                        -> migration_conflict (unverifiable)
//   - different identity                                               -> migration_conflict (identity mismatch)
//   - no repository-encrypted declaration                              -> fresh migration
func planSyncRepoMigration(ctx context.Context, root string, req SyncRepoMigrateRequest) (syncRepoMigrationPlan, error) {
	state, err := pinaxremote.Load(root)
	if err != nil || state.Config.S3 == nil || state.Config.BackendKind != "s3-direct" {
		return syncRepoMigrationPlan{}, &domain.CommandError{Code: "migration_source_unavailable", Message: "an existing s3-direct runtime is required", Hint: "Configure the current Capsa S3 backend before migration."}
	}
	contentKeyID := pinaxremote.KeyID(pinaxremote.EncryptionSecretRef(state.Config))
	profileName := strings.TrimSpace(state.Config.S3.Profile)
	credentials, err := pinaxprofile.LoadAWSStaticCredentials(profileName)
	if err != nil {
		return syncRepoMigrationPlan{}, &domain.CommandError{Code: "migration_source_unavailable", Message: "the configured AWS profile could not be read", Hint: "Verify the current profile before migration."}
	}
	contentKey, err := pinaxprofile.ResolveSecretRef(pinaxremote.EncryptionSecretRef(state.Config))
	if err != nil || strings.TrimSpace(contentKey) == "" {
		return syncRepoMigrationPlan{}, &domain.CommandError{Code: "migration_source_unavailable", Message: "the current content encryption key could not be resolved", Hint: "Restore the existing content key before migration."}
	}
	credentialID := profileName
	if credentialID == "" {
		credentialID = "default"
	}
	encryptionID := "capsa-content-key-" + state.Config.WorkspaceID
	plan := syncRepoMigrationPlan{
		state:        state,
		profileName:  profileName,
		credentials:  credentials,
		contentKey:   contentKey,
		contentKeyID: contentKeyID,
		credentialID: credentialID,
		encryptionID: encryptionID,
		workspaceID:  state.Config.WorkspaceID,
	}
	if existing, declErr := pinaxremote.LoadSyncConfig(root); declErr == nil {
		if existing.Backend.S3 != nil && existing.Backend.S3.CredentialMode == pinaxremote.CredentialModeRepositoryEncrypted {
			identityMatch := existing.Secrets.CredentialID == credentialID &&
				existing.Secrets.EncryptionKeyID == encryptionID &&
				existing.Workspace.WorkspaceID == plan.workspaceID
			if identityMatch {
				if migrationEnvelopeUnlocks(ctx, root, req.UnlockSource, existing) {
					plan.alreadyMigrated = true
					return plan, nil
				}
				plan.conflictReason = "the existing repository envelope cannot be verified with the provided passphrase"
				return plan, nil
			}
			plan.conflictReason = "the existing repository-encrypted declaration has a different credential/encryption/workspace identity"
			return plan, nil
		}
	}
	return plan, nil
}

// migrationEnvelopeUnlocks reports whether the existing repository-encrypted
// envelope can be unlocked with the supplied source and resolves its credential
// entry. It performs no remote access (the AWS SDK provider is constructed but
// not used) and never writes.
func migrationEnvelopeUnlocks(ctx context.Context, root string, source projectsecrets.UnlockSource, existing pinaxremote.SyncConfig) bool {
	if source == nil {
		return false
	}
	resolver := NewSyncCredentialResolver("pinax", existing.Workspace.WorkspaceID, existing.Secrets.CredentialID)
	if _, _, err := resolver.Resolve(ctx, root, source); err != nil {
		return false
	}
	return true
}

// applySyncRepoMigration writes the portable envelope and declaration. Ordering
// is envelope-first then atomic-declaration-rename: a fresh migration has no
// prior envelope to corrupt, and the declaration is the authoritative pointer,
// so a declaration write failure leaves the previous declaration intact (the
// staged envelope is inert ciphertext without a matching declaration). The
// receipt keeps remote_write=false and key_rotated=false.
func applySyncRepoMigration(ctx context.Context, command, root string, req SyncRepoMigrateRequest, plan syncRepoMigrationPlan) (domain.Projection, error) {
	held, err := holdUnlockSource(ctx, req.UnlockSource)
	if err != nil {
		ce := &domain.CommandError{Code: "sync_repo_unlock_required", Message: "a repository passphrase is required", Hint: "Use --unlock prompt, keychain, file, or env."}
		return domain.NewErrorProjection(command, ce), ce
	}
	defer held.Close()
	store := projectsecrets.NewStore()
	if _, err := store.Init(root, projectSecretAsset, "pinax", plan.workspaceID, "passphrase-v1", held.secret); err != nil {
		return mapMigrationError(command, err)
	}
	payload, err := json.Marshal(pinaxremote.S3Credentials{
		AccessKeyID: plan.credentials.AccessKeyID, SecretAccessKey: plan.credentials.SecretAccessKey, SessionToken: plan.credentials.SessionToken,
	})
	if err != nil {
		return errorProjection(command, err), err
	}
	defer func() {
		for i := range payload {
			payload[i] = 0
		}
	}()
	if _, err := store.SetEntry(root, projectSecretAsset, plan.credentialID, "credential", pinaxremote.S3CredentialFormat, "1", payload, held.secret); err != nil {
		return mapMigrationError(command, err)
	}
	if _, err := store.SetEntry(root, projectSecretAsset, plan.encryptionID, "encryption_key", capsaEncryptionKeyFormat, "1", []byte(plan.contentKey), held.secret); err != nil {
		return mapMigrationError(command, err)
	}
	if req.RememberKeychain != nil {
		if err := rememberKeychainSecret(ctx, req.RememberKeychain, held.secret); err != nil {
			return mapMigrationError(command, err)
		}
	}
	s3 := *plan.state.Config.S3
	s3.Profile = ""
	s3.CredentialMode = pinaxremote.CredentialModeRepositoryEncrypted
	declaration := pinaxremote.SyncConfig{
		SchemaVersion: pinaxremote.SyncConfigSchemaVersion,
		Backend:       pinaxremote.SyncBackend{Kind: "s3-direct", Endpoint: plan.state.Config.Endpoint, S3: &s3},
		Workspace:     pinaxremote.SyncWorkspace{WorkspaceID: plan.workspaceID},
		Secrets:       pinaxremote.SyncSecretRefs{CredentialID: plan.credentialID, EncryptionKeyID: plan.encryptionID},
		Policy:        pinaxremote.SyncPolicy{RemoteDeletePolicy: "deny", NewDeviceMode: "pull-only"},
		Requires: pinaxremote.SyncRequires{Capabilities: []string{
			"repository-encrypted-s3-v1",
			"capsa-remote-commit-v1",
			"pull-only-bootstrap-v1",
		}},
	}.Normalized()
	if err := writeSyncRepoConfig(root, declaration); err != nil {
		return errorProjection(command, err), err
	}
	if err := ensureSyncRepoGitignore(root); err != nil {
		return errorProjection(command, err), err
	}
	projection := domain.NewProjection(command, "Device-profile sync configuration migrated to a repository-encrypted envelope.")
	projection.Facts["credential_mode"] = pinaxremote.CredentialModeRepositoryEncrypted
	projection.Facts["credential_id"] = plan.credentialID
	projection.Facts["encryption_key_id"] = plan.encryptionID
	projection.Facts["remote_write"] = "false"
	projection.Facts["key_rotated"] = "false"
	projection.Facts["content_key_id"] = plan.contentKeyID
	projection.Evidence = []string{".pinax/pinax-sync.yaml", projectSecretAsset, ".gitignore"}
	projection.Actions = []domain.Action{{Name: "bootstrap", Command: fmt.Sprintf("pinax sync repo bootstrap --device <device> --unlock prompt --pull --yes --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func mapMigrationError(command string, err error) (domain.Projection, error) {
	code := "migration_failed"
	if projectErr, ok := err.(*projectsecrets.Error); ok {
		switch projectErr.Code {
		case projectsecrets.CodeKeychainUnavailable:
			code = "keychain_store_unavailable"
		case projectsecrets.CodeUnlockFailed, projectsecrets.CodeUnlockRequired:
			code = "sync_repo_unlock_failed"
		}
	}
	commandErr := &domain.CommandError{Code: code, Message: code}
	return domain.NewErrorProjection(command, commandErr), commandErr
}
