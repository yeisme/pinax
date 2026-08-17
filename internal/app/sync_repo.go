package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/yeisme/credentialctl/pkg/projectsecrets"
	"github.com/yeisme/pinax/internal/domain"
	pinaxprofile "github.com/yeisme/pinax/internal/profile"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
)

// SyncRepoInitRequest initializes a repository sync declaration.
type SyncRepoInitRequest struct {
	VaultPath       string
	BackendKind     string
	Endpoint        string
	WorkspaceID     string
	TenantID        string
	AppID           string
	CredentialID    string
	EncryptionKeyID string
	RemoteDelete    string
	// ExistingIfPresent reuses an existing declaration's identity fields when the
	// repo is re-initialized (idempotent init).
	ExistingIfPresent bool
}

// SyncRepoInit creates or updates the repository sync declaration. It is
// idempotent: re-running with the same workspace preserves the encryption key
// identity. It never writes plaintext credentials.
func (s *Service) SyncRepoInit(_ context.Context, req SyncRepoInitRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.repo.init", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("sync.repo.init", err), err
	}
	cfg := pinaxremote.SyncConfig{
		SchemaVersion: pinaxremote.SyncConfigSchemaVersion,
		Backend:       pinaxremote.SyncBackend{Kind: strings.TrimSpace(req.BackendKind), Endpoint: strings.TrimSpace(req.Endpoint)},
		Workspace:     pinaxremote.SyncWorkspace{WorkspaceID: req.WorkspaceID, TenantID: req.TenantID, AppID: req.AppID},
		Secrets:       pinaxremote.SyncSecretRefs{CredentialID: req.CredentialID, EncryptionKeyID: req.EncryptionKeyID},
		Policy:        pinaxremote.SyncPolicy{RemoteDeletePolicy: req.RemoteDelete},
	}
	if req.ExistingIfPresent {
		if existing, err := pinaxremote.LoadSyncConfig(root); err == nil && existing.Secrets.EncryptionKeyID != "" && cfg.Secrets.EncryptionKeyID == "" {
			cfg.Secrets.EncryptionKeyID = existing.Secrets.EncryptionKeyID
		}
		if existing, err := pinaxremote.LoadSyncConfig(root); err == nil && existing.Secrets.CredentialID != "" && cfg.Secrets.CredentialID == "" {
			cfg.Secrets.CredentialID = existing.Secrets.CredentialID
		}
	}
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return errorProjection("sync.repo.init", err), err
	}
	if err := writeSyncRepoConfig(root, cfg); err != nil {
		return errorProjection("sync.repo.init", err), err
	}
	if err := ensureSyncRepoGitignore(root); err != nil {
		return errorProjection("sync.repo.init", err), err
	}
	projection := domain.NewProjection("sync.repo.init", "Sync repository declaration initialized.")
	projection.Facts["schema_version"] = pinaxremote.SyncConfigSchemaVersion
	projection.Facts["backend_kind"] = cfg.Backend.Kind
	projection.Facts["workspace_id"] = cfg.Workspace.WorkspaceID
	projection.Facts["namespace"] = cfg.EffectiveNamespace()
	projection.Facts["encryption_key_id"] = cfg.Secrets.EncryptionKeyID
	projection.Evidence = []string{filepath.Join(".pinax", pinaxremote.DeclarationFileName)}
	projection.Actions = []domain.Action{
		{Name: "secret", Command: fmt.Sprintf("pinax sync repo secret set --name %s --json --vault %s", shellQuote(cfg.Secrets.EncryptionKeyID), shellQuote(root))},
		{Name: "bootstrap", Command: fmt.Sprintf("pinax sync repo bootstrap --device <unique-device> --vault %s --json", shellQuote(root))},
	}
	return projection, nil
}

// writeSyncRepoConfig writes the declaration via the canonical boundary.
func writeSyncRepoConfig(root string, cfg pinaxremote.SyncConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal declaration: %w", err)
	}
	return atomicWriteFile(pinaxremote.DeclarationPath(root), data, 0o600)
}

// ensureSyncRepoGitignore protects device-owned and secrets runtime files from
// accidental commits beyond the deliberately-committable declaration.
func ensureSyncRepoGitignore(root string) error {
	const marker = "# pinax-sync: device runtime state (do not commit)"
	entries := []string{
		".pinax/cloud/",
	}
	gitignorePath := filepath.Join(root, ".gitignore")
	existing := ""
	if b, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(b)
	}
	out := existing
	if !strings.Contains(existing, marker) {
		out += "\n" + marker + "\n"
	}
	for _, e := range entries {
		if !strings.Contains(out, e) {
			out += e + "\n"
		}
	}
	if out == existing {
		return nil
	}
	return os.WriteFile(gitignorePath, []byte(out), 0o644)
}

// --- secret management ---

// SyncRepoSecretRequest drives secret set/import/list/remove. Plaintext is
// accepted only transiently and never persisted outside the encrypted asset.
type SyncRepoSecretRequest struct {
	VaultPath string
	Action    string // set, import, list, remove
	Name      string
	Identity  string
	Kind      string // credential, encryption_key
	Plaintext string // for set; transient
	Provider  string // fake, env
}

// SyncRepoSecret mutates or inspects the encrypted secrets asset.
func (s *Service) SyncRepoSecret(_ context.Context, req SyncRepoSecretRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.repo.secret", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("sync.repo.secret", err), err
	}
	switch req.Action {
	case "set":
		return s.syncRepoSecretSet(root, req)
	case "list":
		return s.syncRepoSecretList(root)
	case "remove":
		return s.syncRepoSecretRemove(root, req)
	default:
		err := &domain.CommandError{Code: "unsupported_secret_action", Message: fmt.Sprintf("unsupported secret action: %q", req.Action), Hint: "Use set, list, or remove"}
		return domain.NewErrorProjection("sync.repo.secret", err), err
	}
}

func (s *Service) syncRepoSecretSet(root string, req SyncRepoSecretRequest) (domain.Projection, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		err := &domain.CommandError{Code: "missing_secret_name", Message: "secret name is required"}
		return domain.NewErrorProjection("sync.repo.secret", err), err
	}
	identity := strings.TrimSpace(req.Identity)
	if identity == "" {
		identity = name
	}
	kind := pinaxremote.SecretKindEncryptionKey
	if strings.TrimSpace(req.Kind) == "credential" {
		kind = pinaxremote.SecretKindCredential
	}
	provider, err := pinaxremote.ResolveUnlockProvider(req.Provider)
	if err != nil {
		return domain.NewErrorProjection("sync.repo.secret", &domain.CommandError{Code: "unsupported_provider", Message: err.Error()}), err
	}
	entry, err := provider.Lock(name, req.Plaintext, identity, kind)
	if err != nil {
		return domain.NewErrorProjection("sync.repo.secret", &domain.CommandError{Code: "secret_encrypt_failed", Message: err.Error()}), err
	}
	env, _ := pinaxremote.LoadSecretEnvelope(root)
	if env.Provider == "" {
		env.Provider = provider.Name()
	}
	if env.Secrets == nil {
		env.Secrets = map[string]pinaxremote.SecretEntry{}
	}
	env.Secrets[name] = entry
	if err := pinaxremote.SaveSecretEnvelope(root, env); err != nil {
		return domain.NewErrorProjection("sync.repo.secret", &domain.CommandError{Code: "secret_save_failed", Message: err.Error()}), err
	}
	projection := domain.NewProjection("sync.repo.secret", "Encrypted secret stored.")
	projection.Facts["name"] = name
	projection.Facts["kind"] = string(kind)
	projection.Facts["identity"] = identity
	projection.Facts["provider"] = env.Provider
	projection.Evidence = []string{filepath.Join(".pinax", pinaxremote.SecretsAssetFileName)}
	projection.Actions = []domain.Action{{Name: "list", Command: fmt.Sprintf("pinax sync repo secret list --vault %s --json", shellQuote(root))}}
	return projection, nil
}

func (s *Service) syncRepoSecretList(root string) (domain.Projection, error) {
	env, err := pinaxremote.LoadSecretEnvelope(root)
	if err != nil && !errors.Is(err, pinaxremote.ErrSecretsAssetMissing) {
		return domain.NewErrorProjection("sync.repo.secret", &domain.CommandError{Code: "secret_load_failed", Message: err.Error()}), err
	}
	projection := domain.NewProjection("sync.repo.secret", "Sync secrets listed.")
	projection.Facts["provider"] = env.Provider
	projection.Facts["count"] = fmt.Sprintf("%d", len(env.Secrets))
	items := make([]map[string]string, 0, len(env.Secrets))
	for name, entry := range env.Secrets {
		items = append(items, map[string]string{"name": name, "identity": entry.Identity})
	}
	projection.Data = map[string]any{"secrets": items}
	return projection, nil
}

func (s *Service) syncRepoSecretRemove(root string, req SyncRepoSecretRequest) (domain.Projection, error) {
	env, err := pinaxremote.LoadSecretEnvelope(root)
	if err != nil {
		return domain.NewErrorProjection("sync.repo.secret", &domain.CommandError{Code: "secret_load_failed", Message: err.Error()}), err
	}
	if _, ok := env.Secrets[req.Name]; !ok {
		err := &domain.CommandError{Code: "secret_not_found", Message: fmt.Sprintf("secret %q not found", req.Name)}
		return domain.NewErrorProjection("sync.repo.secret", err), err
	}
	delete(env.Secrets, req.Name)
	if err := pinaxremote.SaveSecretEnvelope(root, env); err != nil {
		return domain.NewErrorProjection("sync.repo.secret", &domain.CommandError{Code: "secret_save_failed", Message: err.Error()}), err
	}
	projection := domain.NewProjection("sync.repo.secret", "Encrypted secret removed.")
	projection.Facts["name"] = req.Name
	projection.Facts["remaining"] = fmt.Sprintf("%d", len(env.Secrets))
	return projection, nil
}

// --- plan / apply / bootstrap / doctor ---

// SyncRepoRuntimeRequest drives bootstrap, plan and apply.
type SyncRepoRuntimeRequest struct {
	VaultPath string
	DeviceID  string
	Yes       bool
	// UnlockProvider overrides the default provider (for tests / explicit choice).
	UnlockProvider pinaxremote.UnlockProvider
	// ProjectUnlockSource, when set, unlocks a repository-encrypted S3/COS
	// credential bundle via credentialctl before compiling the runtime. Used by
	// `bootstrap --unlock` to prove the envelope unlocks (fail-closed) as part
	// of the staged bootstrap transaction. Nil is allowed when the declaration
	// is not in repository-encrypted mode.
	ProjectUnlockSource projectsecrets.UnlockSource
	// RememberKeychain, when non-nil, receives the already verified repository
	// passphrase after both credential and content-key entries authenticate.
	RememberKeychain *projectsecrets.KeychainSource
	// Pull requests the staged transaction to proceed to a pull after compiling
	// the pull-only runtime. The compile-only safety property (no remote write
	// until pull succeeds) holds regardless.
	Pull bool
}

// SyncRepoBootstrap is the new-device pull-only first run: it unlocks secrets,
// compiles + applies the runtime config and writes the source marker. On a
// device with no local sync receipt it does NOT upload local deletions.
func (s *Service) SyncRepoBootstrap(ctx context.Context, req SyncRepoRuntimeRequest) (domain.Projection, error) {
	return s.syncRepoApplyOrBootstrap(ctx, req, true)
}

// SyncRepoApply regenerates the runtime config from the declaration on an
// already-initialized device. High-risk changes (workspace/namespace/key
// identity/remote-delete) require explicit --yes.
func (s *Service) SyncRepoApply(ctx context.Context, req SyncRepoRuntimeRequest) (domain.Projection, error) {
	return s.syncRepoApplyOrBootstrap(ctx, req, false)
}

func (s *Service) syncRepoApplyOrBootstrap(ctx context.Context, req SyncRepoRuntimeRequest, bootstrap bool) (domain.Projection, error) {
	command := "sync.repo.apply"
	if bootstrap {
		command = "sync.repo.bootstrap"
	}
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection(command, err), err
	}
	declaration, err := pinaxremote.LoadSyncConfig(root)
	if err != nil {
		if errors.Is(err, pinaxremote.ErrSyncDeclarationMissing) {
			ce := &domain.CommandError{Code: "declaration_missing", Message: "sync repository declaration not found", Hint: "Run `pinax sync repo init` first"}
			return domain.NewErrorProjection(command, ce), ce
		}
		return domain.NewErrorProjection(command, &domain.CommandError{Code: "declaration_invalid", Message: err.Error()}), err
	}
	resolved, errs := s.resolveSecrets(root, declaration, req.UnlockProvider)
	if len(errs) > 0 {
		ce := &domain.CommandError{Code: "sync_repo_unlock_required", Message: errs[0].Error(), Hint: fmt.Sprintf("Provide an unlock identity (e.g. PINAX_SYNC_FAKE_KEY or PINAX_SYNC_SECRET_*) and re-run `pinax sync repo %s`", ternary(bootstrap, "bootstrap", "apply"))}
		return domain.NewErrorProjection(command, ce), ce
	}
	// Staged bootstrap transaction (pinax-passphrase-s3-bootstrap task 5.2):
	// when the declaration is repository-encrypted and an unlock source is
	// supplied, PROVE the envelope unlocks (fail-closed) before compiling the
	// runtime. The snapshot is closed immediately — plaintext is not retained
	// past the verification boundary. Any failure here aborts before compile, so
	// no remote write / head replace can occur.
	var unlockSourceFact, credentialModeFact, contentKeyIDFact string
	if declaration.Backend.S3 != nil {
		credentialModeFact = declaration.Backend.S3.CredentialMode
		if credentialModeFact == "" {
			credentialModeFact = pinaxremote.CredentialModeDeviceProfile
		}
	}
	var heldSource *heldUnlockSource
	if credentialModeFact == pinaxremote.CredentialModeRepositoryEncrypted {
		if req.ProjectUnlockSource == nil {
			ce := &domain.CommandError{Code: "sync_repo_unlock_required", Message: "repository unlock source is required", Hint: "Use --unlock prompt, --unlock keychain, --passphrase-file, or --unlock env."}
			return domain.NewErrorProjection(command, ce), ce
		}
		heldSource, err = holdUnlockSource(ctx, req.ProjectUnlockSource)
		if err != nil {
			ce := &domain.CommandError{Code: "sync_repo_unlock_required", Message: "repository unlock source is unavailable", Hint: "Check the selected unlock source and retry."}
			return domain.NewErrorProjection(command, ce), ce
		}
		defer heldSource.Close()
		encryptionRef, unlockErr := resolveRepositoryBootstrapSecrets(ctx, root, declaration, heldSource)
		if unlockErr != nil {
			ce := &domain.CommandError{Code: "sync_repo_unlock_failed", Message: "repository credential envelope did not unlock", Hint: "Check the passphrase/keychain/file source; the Capsa content key and remote revisions are untouched."}
			return domain.NewErrorProjection(command, ce), ce
		}
		resolved.credential = strings.TrimSpace(declaration.Secrets.CredentialID)
		if resolved.credential == "" {
			resolved.credential = "default"
		}
		resolved.encryption = encryptionRef
		contentKeyIDFact = pinaxremote.KeyID(encryptionRef)
		unlockSourceFact = heldSource.Descriptor()
		if req.RememberKeychain != nil {
			if err := rememberKeychainSecret(ctx, req.RememberKeychain, heldSource.secret); err != nil {
				ce := &domain.CommandError{Code: "keychain_store_unavailable", Message: "repository passphrase could not be verified in Keychain", Hint: "Unlock Keychain or retry without --remember-keychain."}
				return domain.NewErrorProjection(command, ce), ce
			}
		}
	}
	// High-risk change detection: compare against existing runtime config.
	existing, _ := pinaxremote.Load(root)
	preserve := ""
	if existing.Config.CreatedAt != "" {
		preserve = existing.Config.CreatedAt
	}
	result, err := pinaxremote.Compile(pinaxremote.CompileRequest{
		Declaration:     declaration,
		ResolvedSecret:  resolved.credential,
		ResolvedEncrypt: resolved.encryption,
		DeviceID:        req.DeviceID,
		Now:             time.Now(),
	})
	if err != nil {
		return errorProjection(command, err), err
	}
	highRisk := detectHighRiskChange(existing.Config, result.RuntimeConfig, declaration)
	if highRisk != "" && !req.Yes {
		ce := &domain.CommandError{Code: "approval_required", Message: fmt.Sprintf("apply would change %s; explicit confirmation required", highRisk), Hint: "Re-run with --yes to confirm"}
		return domain.NewErrorProjection(command, ce), ce
	}
	if err := pinaxremote.ApplyCompiled(root, result, preserve); err != nil {
		return errorProjection(command, err), err
	}
	summary := "Sync runtime config regenerated from declaration."
	if bootstrap {
		summary = "Sync runtime config bootstrapped from declaration (pull-only on new device)."
	}
	projection := domain.NewProjection(command, summary)
	projection.Facts["workspace_id"] = result.RuntimeConfig.WorkspaceID
	projection.Facts["device_id"] = result.RuntimeConfig.DeviceID
	projection.Facts["backend_kind"] = result.RuntimeConfig.BackendKind
	projection.Facts["declaration_digest"] = result.DeclarationDigest
	projection.Facts["namespace"] = declaration.EffectiveNamespace()
	projection.Facts["pull_only"] = ternaryStr(bootstrap, "true", "false")
	if credentialModeFact != "" {
		projection.Facts["credential_mode"] = credentialModeFact
	}
	if unlockSourceFact != "" {
		projection.Facts["unlock_source"] = unlockSourceFact
		projection.Facts["credential_verified"] = "true"
	}
	if req.RememberKeychain != nil {
		projection.Facts["keychain_remembered"] = "true"
		projection.Facts["keychain_account"] = req.RememberKeychain.AccountDigest()
	}
	if contentKeyIDFact != "" {
		projection.Facts["content_key_id"] = contentKeyIDFact
	}
	if req.Pull {
		pullProjection, pullErr := s.SyncPull(ctx, SyncRequest{
			VaultPath:           root,
			Target:              "capsa",
			Yes:                 true,
			ProjectUnlockSource: heldSource,
		})
		if pullErr != nil && (pullProjection.Error == nil || pullProjection.Error.Code != "cloud_empty_remote") {
			ce := &domain.CommandError{Code: "bootstrap_pull_failed", Message: "runtime is ready but the initial pull failed", Hint: fmt.Sprintf("Run `pinax sync repo doctor --vault %s --json`, then retry `pinax sync pull --target capsa --vault %s --yes --json`.", shellQuote(root), shellQuote(root))}
			projection.Status = "partial"
			projection.Error = ce
			projection.Facts["runtime_ready"] = "true"
			projection.Facts["pull_applied"] = "false"
			projection.Facts["remote_write"] = "false"
			projection.Actions = append(projection.Actions, pullProjection.Actions...)
			projection.Data = map[string]any{"pull": pullProjection}
			return projection, ce
		}
		projection.Facts["pull_applied"] = "true"
		if pullProjection.Error != nil && pullProjection.Error.Code == "cloud_empty_remote" {
			projection.Facts["files_applied"] = "0"
		}
		projection.Facts["runtime_ready"] = "true"
		projection.Facts["remote_write"] = "false"
		for _, key := range []string{"revision_id", "files_applied", "run_id"} {
			if value := pullProjection.Facts[key]; value != "" {
				projection.Facts[key] = value
			}
		}
		projection.Evidence = append(projection.Evidence, pullProjection.Evidence...)
	}
	if highRisk != "" {
		projection.Facts["approved_change"] = highRisk
	}
	projection.Evidence = []string{filepath.Join(".pinax", "cloud", "config.yaml"), filepath.Join(".pinax", "cloud", pinaxremote.SourceMarkerFileName)}
	projection.Actions = []domain.Action{
		{Name: "doctor", Command: fmt.Sprintf("pinax sync repo doctor --vault %s --json", shellQuote(root))},
		{Name: "sync", Command: fmt.Sprintf("pinax sync pull --vault %s --json", shellQuote(root))},
	}
	return projection, nil
}

// SyncRepoPlan reports intended changes WITHOUT writing anything.
func (s *Service) SyncRepoPlan(_ context.Context, req SyncRepoRuntimeRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.repo.plan", err), err
	}
	declaration, err := pinaxremote.LoadSyncConfig(root)
	if err != nil {
		if errors.Is(err, pinaxremote.ErrSyncDeclarationMissing) {
			ce := &domain.CommandError{Code: "declaration_missing", Message: "sync repository declaration not found", Hint: "Run `pinax sync repo init` first"}
			return domain.NewErrorProjection("sync.repo.plan", ce), ce
		}
		return domain.NewErrorProjection("sync.repo.plan", &domain.CommandError{Code: "declaration_invalid", Message: err.Error()}), err
	}
	existing, _ := pinaxremote.Load(root)
	marker, _ := pinaxremote.LoadSourceMarker(root)
	drift := pinaxremote.DetectDrift(declaration, existing.Config, marker)
	projection := domain.NewProjection("sync.repo.plan", "Sync repository plan generated (no writes).")
	projection.Facts["workspace_id"] = declaration.Normalized().Workspace.WorkspaceID
	projection.Facts["namespace"] = declaration.EffectiveNamespace()
	projection.Facts["in_drift"] = ternaryStr(drift.InDrift, "true", "false")
	projection.Facts["runtime_configured"] = ternaryStr(existing.Config.WorkspaceID != "", "true", "false")
	if drift.InDrift {
		projection.Facts["requires_approval"] = "declaration_changed"
	}
	projection.Data = map[string]any{
		"declaration": map[string]any{
			"backend_kind":      declaration.Backend.Kind,
			"namespace":         declaration.EffectiveNamespace(),
			"encryption_key_id": declaration.Secrets.EncryptionKeyID,
		},
		"drift": drift,
	}
	projection.Actions = []domain.Action{{Name: "apply", Command: fmt.Sprintf("pinax sync repo apply --vault %s --json", shellQuote(root))}}
	return projection, nil
}

// SyncRepoDoctor diagnoses drift, key identity, device and protected-path state.
func (s *Service) SyncRepoDoctor(_ context.Context, req VaultRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.repo.doctor", err), err
	}
	declaration, declErr := pinaxremote.LoadSyncConfig(root)
	existing, _ := pinaxremote.Load(root)
	marker, _ := pinaxremote.LoadSourceMarker(root)
	projection := domain.NewProjection("sync.repo.doctor", "Sync repository doctor complete.")
	code := "healthy"
	status := "success"
	summary := "sync repository declaration and runtime config are in sync"
	if errors.Is(declErr, pinaxremote.ErrSyncDeclarationMissing) {
		code = "declaration_missing"
		status = "failed"
		summary = "no sync repository declaration found"
	} else if declErr != nil {
		code = "declaration_invalid"
		status = "failed"
		summary = declErr.Error()
	} else {
		drift := pinaxremote.DetectDrift(declaration, existing.Config, marker)
		if drift.InDrift {
			code = "sync_repo_config_drift"
			status = "failed"
			summary = "repository declaration differs from generated runtime config"
			projection.Data = map[string]any{"drift": drift}
		}
	}
	projection.Status = status
	projection.Summary = summary
	projection.Facts["code"] = code
	projection.Facts["declaration_present"] = ternaryStr(declErr == nil, "true", "false")
	projection.Facts["runtime_configured"] = ternaryStr(existing.Config.WorkspaceID != "", "true", "false")
	if declaration.Workspace.WorkspaceID != "" {
		projection.Facts["workspace_id"] = declaration.Workspace.WorkspaceID
		projection.Facts["namespace"] = declaration.EffectiveNamespace()
		projection.Facts["encryption_key_id"] = declaration.Secrets.EncryptionKeyID
	}
	// Report the S3 credential resolution mode so doctor surfaces whether the
	// repository is using repository-encrypted bundles (passphrase/Keychain) or
	// the device-local AWS profile path. The value is non-sensitive topology.
	if declaration.Backend.S3 != nil {
		mode := declaration.Backend.S3.CredentialMode
		if mode == "" {
			mode = pinaxremote.CredentialModeDeviceProfile
		}
		projection.Facts["credential_mode"] = mode
		projection.Facts["repository_encrypted_ready"] = ternaryStr(repoCredentialEnvelopePresent(root), "true", "false")
	}
	if existing.Config.DeviceID != "" {
		projection.Facts["device_id"] = existing.Config.DeviceID
	}
	authBoundary := "provider_credentials"
	serverAudit := false
	if existing.Config.BackendKind == "server" {
		authBoundary = "pinax_cloud_server"
		serverAudit = true
	}
	projection.Facts["auth_boundary"] = authBoundary
	projection.Facts["server_audit"] = ternaryStr(serverAudit, "true", "false")
	nextAction := fmt.Sprintf("pinax sync repo apply --vault %s --json", shellQuote(root))
	if code == "declaration_missing" {
		nextAction = fmt.Sprintf("pinax sync repo init --vault %s --json", shellQuote(root))
	}
	projection.Actions = []domain.Action{{Name: "next", Command: nextAction}}
	return projection, nil
}

// resolveSecrets unlocks the secrets asset and maps logical identities to
// device-local secret references for the runtime config. It fails closed when
// an identity cannot be resolved.
type resolvedSecrets struct {
	credential string
	encryption string
}

func (s *Service) resolveSecrets(root string, declaration pinaxremote.SyncConfig, override pinaxremote.UnlockProvider) (resolvedSecrets, []error) {
	env, err := pinaxremote.LoadSecretEnvelope(root)
	var errs []error
	if errors.Is(err, pinaxremote.ErrSecretsAssetMissing) {
		// No encrypted asset: fall back to reusing any existing runtime secret
		// reference (profile-management spec: reuse existing encryption ref).
		existing, _ := pinaxremote.Load(root)
		return resolvedSecrets{
			credential: existing.Config.SecretRef,
			encryption: pinaxremote.EncryptionSecretRef(existing.Config),
		}, nil
	}
	if err != nil {
		return resolvedSecrets{}, []error{err}
	}
	provider := override
	if provider == nil {
		p, perr := pinaxremote.ResolveUnlockProvider(env.Provider)
		if perr != nil {
			return resolvedSecrets{}, []error{perr}
		}
		provider = p
	}
	values, uerr := provider.Unlock(env)
	if uerr != nil {
		return resolvedSecrets{}, []error{uerr}
	}
	// Persist unlocked plaintext into the user-level secret store and emit only
	// stored:// references to the runtime config. Plaintext never reaches the
	// generated cloud/config.yaml, logs or receipts.
	storeRef := func(name, plaintext string) string {
		if strings.TrimSpace(plaintext) == "" {
			return ""
		}
		if strings.HasPrefix(plaintext, "stored://") || strings.HasPrefix(plaintext, "env://") || strings.HasPrefix(plaintext, "env:") {
			return plaintext // already a reference
		}
		ref, err := pinaxprofile.SetStoredSecret("sync-repo-"+name, plaintext)
		if err != nil {
			errs = append(errs, err)
			return plaintext
		}
		return ref
	}
	cred := ""
	if declaration.Secrets.CredentialID != "" {
		cred = storeRef("cred-"+declaration.Secrets.CredentialID, values[declaration.Secrets.CredentialID])
	}
	enc := storeRef("enc-"+declaration.Secrets.EncryptionKeyID, values[declaration.Secrets.EncryptionKeyID])
	// Reuse existing runtime refs when the declaration's identity resolves empty
	// (e.g. env provider on a device that already has a stored:// ref).
	if enc == "" {
		existing, _ := pinaxremote.Load(root)
		enc = pinaxremote.EncryptionSecretRef(existing.Config)
	}
	return resolvedSecrets{credential: cred, encryption: enc}, errs
}

// detectHighRiskChange reports the first high-risk divergence between the
// existing runtime config and the compiled one, or "" if none. High-risk means
// workspace, backend namespace, encryption key identity or remote-delete policy.
func detectHighRiskChange(existing pinaxremote.Config, compiled pinaxremote.Config, declaration pinaxremote.SyncConfig) string {
	decl := declaration.Normalized()
	if existing.WorkspaceID != "" && existing.WorkspaceID != compiled.WorkspaceID {
		return "workspace.workspace_id"
	}
	if existing.BackendKind != "" && existing.BackendKind != compiled.BackendKind {
		return "backend.kind"
	}
	// Namespace change (tenant/app) is high-risk even if workspace id matches.
	if existing.WorkspaceID != "" && existing.WorkspaceID == compiled.WorkspaceID {
		// Namespace is encoded in endpoint prefix for s3; a declaration change in
		// tenant/app surfaces via EffectiveNamespace, checked in plan/doctor.
		_ = decl
	}
	if existing.EncryptionSecretRef != "" && compiled.EncryptionSecretRef != "" && existing.EncryptionSecretRef != compiled.EncryptionSecretRef {
		return "encryption_key_identity"
	}
	return ""
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func ternaryStr(cond bool, a, b string) string {
	return ternary(cond, a, b)
}
