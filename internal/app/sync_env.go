package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/output"
	pinaxremote "github.com/yeisme/pinax/internal/remote"
	"github.com/yeisme/pinax/internal/vaultignore"
)

// SyncEnvSchemaFact is the stable schema version fact surfaced in env projections.
const SyncEnvSchemaFact = pinaxremote.EnvAssetSchemaVersion

// SyncEnvRequest drives the `pinax sync env` command tree.
type SyncEnvRequest struct {
	VaultPath string
	Action    string // init, set, list, unlock, clean, doctor
	// For set: the env key name and plaintext value (transient, never persisted).
	Key      string
	Value    string
	Provider string
	// For unlock --materialize: keys to allowlist for the materialized file.
	Allowlist []string
	// Materialize requests writing the plaintext to the managed runtime path.
	Materialize bool
}

// SyncEnv is the entry point for the `pinax sync env` command tree. It dispatches
// to the per-action service handlers. Plaintext values are accepted transiently
// and never persisted, logged or emitted in any projection.
func (s *Service) SyncEnv(_ context.Context, req SyncEnvRequest) (domain.Projection, error) {
	root, err := cleanVaultPath(req.VaultPath)
	if err != nil {
		return errorProjection("sync.env", err), err
	}
	if err := ensureVaultAssets(root); err != nil {
		return errorProjection("sync.env", err), err
	}
	switch req.Action {
	case "init":
		return s.syncEnvInit(root, req)
	case "set":
		return s.syncEnvSet(root, req)
	case "list":
		return s.syncEnvList(root)
	case "unlock":
		return s.syncEnvUnlock(root, req)
	case "clean":
		return s.syncEnvClean(root)
	case "doctor":
		return s.syncEnvDoctor(root)
	default:
		err := &domain.CommandError{Code: "unsupported_env_action", Message: fmt.Sprintf("unsupported env action: %q", req.Action), Hint: "Use init, set, list, unlock, clean, or doctor"}
		return domain.NewErrorProjection("sync.env", err), err
	}
}

// envProvider resolves the unlock provider, preferring an explicit override then
// the asset's recorded provider, then the request default.
func envProvider(root, requestProvider string) (pinaxremote.UnlockProvider, string, error) {
	providerName := strings.TrimSpace(requestProvider)
	if providerName == "" {
		if existing, err := pinaxremote.LoadEnvAsset(root); err == nil {
			providerName = existing.Provider
		}
	}
	if providerName == "" {
		providerName = "fake"
	}
	p, err := pinaxremote.ResolveUnlockProvider(providerName)
	if err != nil {
		return nil, providerName, err
	}
	return p, providerName, nil
}

func (s *Service) syncEnvInit(root string, _ SyncEnvRequest) (domain.Projection, error) {
	// Create an empty encrypted asset if none exists. We encrypt an empty
	// document so the asset is valid from the start; plaintext is never created.
	provider, providerName, err := envProvider(root, "")
	if err != nil {
		return domain.NewErrorProjection("sync.env.init", &domain.CommandError{Code: "unsupported_provider", Message: err.Error()}), err
	}
	existing, loadErr := pinaxremote.LoadEnvAsset(root)
	if errors.Is(loadErr, pinaxremote.ErrEnvAssetMissing) || (loadErr == nil && existing.Ciphertext == "") {
		entry, lerr := provider.Lock("env", "", "pinax-sync-env", pinaxremote.SecretKindCredential)
		if lerr != nil {
			return domain.NewErrorProjection("sync.env.init", &domain.CommandError{Code: "env_encrypt_failed", Message: lerr.Error()}), lerr
		}
		asset := pinaxremote.EnvAsset{
			Provider:   providerName,
			Ciphertext: entry.Ciphertext,
			Digest:     pinaxremote.EnvDocumentDigest(nil),
			KeyNames:   []string{},
		}
		if err := pinaxremote.SaveEnvAsset(root, asset); err != nil {
			return domain.NewErrorProjection("sync.env.init", &domain.CommandError{Code: "env_save_failed", Message: err.Error()}), err
		}
	}
	// Ensure the managed env-secrets gitignore block (plaintext protected,
	// encrypted asset + .env.example re-included).
	if err := ensureEnvSecretGitignore(root); err != nil {
		return errorProjection("sync.env.init", err), err
	}
	projection := domain.NewProjection("sync.env.init", "Sync env asset initialized.")
	projection.Facts["schema_version"] = pinaxremote.EnvAssetSchemaVersion
	projection.Facts["provider"] = providerName
	projection.Facts["plaintext_created"] = "false"
	projection.Evidence = []string{filepath.Join(".pinax", pinaxremote.EnvAssetFileName)}
	projection.Actions = []domain.Action{
		{Name: "set", Command: fmt.Sprintf("pinax sync env set KEY --value <value> --vault %s --json", shellQuote(root))},
		{Name: "doctor", Command: fmt.Sprintf("pinax sync env doctor --vault %s --json", shellQuote(root))},
	}
	return projection, nil
}

func (s *Service) syncEnvSet(root string, req SyncEnvRequest) (domain.Projection, error) {
	key := strings.TrimSpace(req.Key)
	if key == "" {
		err := &domain.CommandError{Code: "missing_env_key", Message: "env key is required"}
		return domain.NewErrorProjection("sync.env.set", err), err
	}
	// Validate the key is a valid dotenv identifier before encrypting.
	if _, verr := pinaxremote.ParseDotenv([]byte(key + "=x")); verr != nil {
		return domain.NewErrorProjection("sync.env.set", &domain.CommandError{Code: "invalid_env_key", Message: verr.Error()}), verr
	}
	provider, providerName, err := envProvider(root, req.Provider)
	if err != nil {
		return domain.NewErrorProjection("sync.env.set", &domain.CommandError{Code: "unsupported_provider", Message: err.Error()}), err
	}
	// Load existing values (if any), add the key, re-encrypt the whole document.
	doc := loadExistingEnvDocument(root, provider)
	values, perr := pinaxremote.ParseDotenv([]byte(doc))
	if perr != nil {
		// Existing document is corrupt: fail-closed rather than silently dropping.
		return domain.NewErrorProjection("sync.env.set", &domain.CommandError{Code: "env_parse_failed", Message: perr.Error()}), perr
	}
	values[key] = req.Value
	newDoc := string(pinaxremote.FormatDotenv(values))
	entry, lerr := provider.Lock("env", newDoc, "pinax-sync-env", pinaxremote.SecretKindCredential)
	if lerr != nil {
		return domain.NewErrorProjection("sync.env.set", &domain.CommandError{Code: "env_encrypt_failed", Message: lerr.Error()}), lerr
	}
	keyNames := make([]string, 0, len(values))
	for k := range values {
		keyNames = append(keyNames, k)
	}
	asset := pinaxremote.EnvAsset{
		Provider:   providerName,
		Ciphertext: entry.Ciphertext,
		Digest:     pinaxremote.EnvDocumentDigest([]byte(newDoc)),
		KeyNames:   keyNames,
	}
	if err := pinaxremote.SaveEnvAsset(root, asset); err != nil {
		return domain.NewErrorProjection("sync.env.set", &domain.CommandError{Code: "env_save_failed", Message: err.Error()}), err
	}
	if err := ensureEnvSecretGitignore(root); err != nil {
		return errorProjection("sync.env.set", err), err
	}
	projection := domain.NewProjection("sync.env.set", "Encrypted env value stored.")
	projection.Facts["schema_version"] = pinaxremote.EnvAssetSchemaVersion
	projection.Facts["key"] = key
	projection.Facts["key_count"] = fmt.Sprintf("%d", len(values))
	projection.Facts["provider"] = providerName
	projection.Facts["digest"] = asset.Digest
	projection.Evidence = []string{filepath.Join(".pinax", pinaxremote.EnvAssetFileName)}
	projection.Actions = []domain.Action{{Name: "list", Command: fmt.Sprintf("pinax sync env list --vault %s --json", shellQuote(root))}}
	// Value MUST NOT appear anywhere in the projection.
	output.ApplyProjectionRedaction(&projection)
	return projection, nil
}

func (s *Service) syncEnvList(root string) (domain.Projection, error) {
	asset, err := pinaxremote.LoadEnvAsset(root)
	if err != nil && !errors.Is(err, pinaxremote.ErrEnvAssetMissing) {
		return domain.NewErrorProjection("sync.env.list", &domain.CommandError{Code: "env_load_failed", Message: err.Error()}), err
	}
	projection := domain.NewProjection("sync.env.list", "Sync env keys listed.")
	projection.Facts["schema_version"] = pinaxremote.EnvAssetSchemaVersion
	projection.Facts["provider"] = asset.Provider
	projection.Facts["key_count"] = fmt.Sprintf("%d", len(asset.KeyNames))
	projection.Facts["digest"] = asset.Digest
	projection.Data = map[string]any{"keys": asset.KeyNames}
	return projection, nil
}

func (s *Service) syncEnvUnlock(root string, req SyncEnvRequest) (domain.Projection, error) {
	snapshot, err := pinaxremote.ResolveEnvSnapshot(root, nil)
	if err != nil {
		if errors.Is(err, pinaxremote.ErrEnvAssetMissing) {
			ce := &domain.CommandError{Code: "env_asset_missing", Message: "no encrypted env asset found", Hint: "Run `pinax sync env init` first"}
			return domain.NewErrorProjection("sync.env.unlock", ce), ce
		}
		return domain.NewErrorProjection("sync.env.unlock", &domain.CommandError{Code: "env_unlock_failed", Message: redactedEnvMessage(err)}), err
	}
	projection := domain.NewProjection("sync.env.unlock", "Sync env snapshot resolved in memory.")
	projection.Facts["schema_version"] = pinaxremote.EnvAssetSchemaVersion
	projection.Facts["digest"] = snapshot.Digest()
	projection.Facts["key_count"] = fmt.Sprintf("%d", len(snapshot.AllKeys()))
	projection.Facts["materialized"] = "false"
	if req.Materialize {
		allowlist := req.Allowlist
		if len(allowlist) == 0 {
			allowlist = snapshot.AllKeys()
		}
		path, merr := pinaxremote.MaterializeEnv(root, snapshot, allowlist)
		if merr != nil {
			return domain.NewErrorProjection("sync.env.unlock", &domain.CommandError{Code: "env_materialize_failed", Message: merr.Error()}), merr
		}
		projection.Facts["materialized"] = "true"
		projection.Facts["materialized_path"] = relFromVault(root, path)
		projection.Facts["permissions"] = "0600"
		projection.Evidence = []string{relFromVault(root, path)}
	}
	output.ApplyProjectionRedaction(&projection)
	return projection, nil
}

func (s *Service) syncEnvClean(root string) (domain.Projection, error) {
	removed, err := pinaxremote.CleanMaterializedEnv(root)
	if err != nil {
		return domain.NewErrorProjection("sync.env.clean", &domain.CommandError{Code: "env_clean_failed", Message: err.Error()}), err
	}
	projection := domain.NewProjection("sync.env.clean", "Managed materialized env cleaned.")
	projection.Facts["removed"] = ternaryStr(removed, "true", "false")
	projection.Facts["managed_path"] = filepath.Join(".pinax", pinaxremote.EnvRuntimeDir, pinaxremote.EnvRuntimeFileName)
	return projection, nil
}

func (s *Service) syncEnvDoctor(root string) (domain.Projection, error) {
	projection := domain.NewProjection("sync.env.doctor", "Sync env doctor complete.")
	asset, loadErr := pinaxremote.LoadEnvAsset(root)
	status := "success"
	code := "healthy"
	if errors.Is(loadErr, pinaxremote.ErrEnvAssetMissing) {
		status = "failed"
		code = "env_asset_missing"
	} else if loadErr != nil {
		status = "failed"
		code = "env_asset_corrupt"
	}
	projection.Status = status
	projection.Facts["code"] = code
	projection.Facts["schema_version"] = pinaxremote.EnvAssetSchemaVersion
	projection.Facts["provider"] = asset.Provider
	projection.Facts["digest"] = asset.Digest
	projection.Facts["key_count"] = fmt.Sprintf("%d", len(asset.KeyNames))

	// Permission check on the encrypted asset.
	if loadErr == nil {
		info, statErr := os.Stat(pinaxremote.EnvAssetPath(root))
		if statErr == nil {
			projection.Facts["asset_permissions"] = fmt.Sprintf("%04o", info.Mode().Perm())
			if info.Mode().Perm() != 0o600 {
				projection.Facts["asset_permissions_warning"] = "expected_0600"
			}
		}
	}

	// Git tracked-secret detection (warn only, never delete). A tracked plaintext
	// env file is an active secret leak and takes priority over the asset-state
	// code so the user sees the most actionable finding first.
	tracked := detectTrackedSecretEnv(root)
	if len(tracked) > 0 {
		projection.Facts["tracked_secret_env"] = fmt.Sprintf("%d", len(tracked))
		projection.Data = map[string]any{"tracked_paths": tracked}
		status = "failed"
		code = "tracked_secret_env"
		projection.Status = status
		projection.Facts["code"] = code
		projection.Actions = []domain.Action{{Name: "remediate", Command: fmt.Sprintf("git rm --cached -- %s", strings.Join(tracked, " "))}}
	}

	// Materialized file state check.
	materializedPath := pinaxremote.EnvRuntimePath(root)
	if info, err := os.Stat(materializedPath); err == nil {
		projection.Facts["materialized_present"] = "true"
		projection.Facts["materialized_permissions"] = fmt.Sprintf("%04o", info.Mode().Perm())
		if info.Mode().Perm() != 0o600 {
			projection.Facts["materialized_permissions_warning"] = "expected_0600"
		}
	} else {
		projection.Facts["materialized_present"] = "false"
	}

	projection.Summary = fmt.Sprintf("sync env doctor: %s", code)
	output.ApplyProjectionRedaction(&projection)
	return projection, nil
}

// detectTrackedSecretEnv returns plaintext env files tracked by the Git index.
// It warns only — it never modifies the working tree. Git unavailability is
// treated as "no tracked secrets detected" (not an error) so doctor works in
// non-Git vaults.
func detectTrackedSecretEnv(root string) []string {
	gitDir := filepath.Join(root, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return nil
	}
	cmd := exec.Command("git", "-C", root, "ls-files", "--", ".env", ".env.*", "*.env")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var tracked []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Only flag plaintext env files; never the encrypted asset or templates.
		if strings.HasSuffix(line, ".env.example") {
			continue
		}
		if strings.HasSuffix(line, ".age") {
			continue
		}
		tracked = append(tracked, line)
	}
	return tracked
}

// ensureEnvSecretGitignore applies the managed env-secrets gitignore block to the
// vault's .gitignore, preserving user rules.
func ensureEnvSecretGitignore(root string) error {
	gitignorePath := filepath.Join(root, ".gitignore")
	existing := ""
	if b, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(b)
	}
	updated := vaultignore.ApplyEnvSecretGitignore(existing)
	if updated == existing {
		return nil
	}
	return os.WriteFile(gitignorePath, []byte(updated), 0o644)
}

// loadExistingEnvDocument unlocks the current env asset and returns its plaintext
// dotenv document (empty string when no asset exists). Plaintext lives only in
// the caller's transient scope.
func loadExistingEnvDocument(root string, provider pinaxremote.UnlockProvider) string {
	snapshot, err := pinaxremote.ResolveEnvSnapshot(root, provider)
	if err != nil || snapshot == nil {
		return ""
	}
	var b strings.Builder
	keys := snapshot.AllKeys()
	values := make(map[string]string, len(keys))
	for _, k := range keys {
		v, _ := snapshot.Lookup(k)
		values[k] = v
	}
	b.Write(pinaxremote.FormatDotenv(values))
	return b.String()
}

// relFromVault returns a slash path relative to the vault root for evidence,
// falling back to the base name on error.
func relFromVault(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return filepath.Base(abs)
	}
	return filepath.ToSlash(rel)
}

// redactedEnvMessage surfaces an env unlock error message. Env errors already
// avoid attaching plaintext values; this wrapper exists so the unlock path has a
// single, auditable choke point if future provider errors need scrubbing.
func redactedEnvMessage(err error) string {
	return err.Error()
}
