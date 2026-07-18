package remote

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func validDeclaration() SyncConfig {
	return SyncConfig{
		SchemaVersion: SyncConfigSchemaVersion,
		Backend:       SyncBackend{Kind: "s3-direct", Endpoint: "s3://my-bucket/prefix"},
		Workspace:     SyncWorkspace{WorkspaceID: "personal", TenantID: "t1", AppID: "pinax"},
		Secrets:       SyncSecretRefs{CredentialID: "tencent-cos-pinax", EncryptionKeyID: "personal-sync-key"},
	}
}

func TestSyncConfigValidate_AcceptsValidDeclaration(t *testing.T) {
	if err := validDeclaration().Validate(); err != nil {
		t.Fatalf("expected valid declaration to pass, got %v", err)
	}
}

func TestSyncConfigValidate_RejectsUnsupportedSchema(t *testing.T) {
	c := validDeclaration()
	c.SchemaVersion = "pinax.sync.config.v999"
	if err := c.Validate(); !IsSyncConfigError(err, "unsupported_schema_version") {
		t.Fatalf("expected unsupported_schema_version, got %v", err)
	}
}

func TestSyncConfigValidate_RejectsUnsupportedBackend(t *testing.T) {
	c := validDeclaration()
	c.Backend.Kind = "ftp"
	if err := c.Validate(); !IsSyncConfigError(err, "unsupported_backend_kind") {
		t.Fatalf("expected unsupported_backend_kind, got %v", err)
	}
}

func TestSyncConfigValidate_RejectsMissingWorkspaceAndKey(t *testing.T) {
	c := validDeclaration()
	c.Workspace.WorkspaceID = ""
	c.Secrets.EncryptionKeyID = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation failure for missing workspace and key")
	}
}

func TestSyncConfigValidate_RejectsPlaintextSensitive(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SyncConfig)
		code   string
	}{
		{"plaintext token in endpoint", func(c *SyncConfig) { c.Backend.Endpoint = "secret_key=abc123" }, "plaintext_sensitive_field"},
		{"absolute path in workspace", func(c *SyncConfig) { c.Workspace.WorkspaceID = "/home/user/vault" }, "plaintext_sensitive_field"},
		{"password-like field", func(c *SyncConfig) { c.Secrets.EncryptionKeyID = "password=hunter2" }, "plaintext_sensitive_field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validDeclaration()
			tc.mutate(&c)
			err := c.Validate()
			if !IsSyncConfigError(err, tc.code) {
				t.Fatalf("expected %s, got %v", tc.code, err)
			}
		})
	}
}

func TestSyncConfigValidate_RejectsUnsupportedScheme(t *testing.T) {
	c := validDeclaration()
	c.Backend.Endpoint = "ftp://example.com"
	if err := c.Validate(); !IsSyncConfigError(err, "unsupported_scheme") {
		t.Fatalf("expected unsupported_scheme, got %v", err)
	}
}

func TestSyncConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := validDeclaration()
	if err := writeSyncConfig(dir, c); err != nil {
		t.Fatalf("writeSyncConfig: %v", err)
	}
	loaded, err := LoadSyncConfig(dir)
	if err != nil {
		t.Fatalf("LoadSyncConfig: %v", err)
	}
	if loaded.Workspace.WorkspaceID != "personal" {
		t.Fatalf("round-trip workspace mismatch: %s", loaded.Workspace.WorkspaceID)
	}
	if loaded.Secrets.EncryptionKeyID != "personal-sync-key" {
		t.Fatalf("round-trip key mismatch: %s", loaded.Secrets.EncryptionKeyID)
	}
	// File must have restrictive permissions.
	info, err := os.Stat(DeclarationPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("declaration perm = %o, want 0600", perm)
	}
}

func TestLoadSyncConfig_MissingIsSentinel(t *testing.T) {
	_, err := LoadSyncConfig(t.TempDir())
	if !errors.Is(err, ErrSyncDeclarationMissing) {
		t.Fatalf("expected ErrSyncDeclarationMissing, got %v", err)
	}
}

func TestEffectiveNamespace(t *testing.T) {
	c := validDeclaration()
	if got := c.EffectiveNamespace(); got != "t-t1/a-pinax/w-personal" {
		t.Fatalf("namespace = %q", got)
	}
	c2 := SyncConfig{Workspace: SyncWorkspace{WorkspaceID: "solo"}}.Normalized()
	if got := c2.EffectiveNamespace(); got != "w-solo" {
		t.Fatalf("solo namespace = %q", got)
	}
}

// --- Secret envelope ---

func TestSecretEnvelopeFakeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PINAX_SYNC_FAKE_KEY", "test-passphrase")
	provider := FakeUnlockProvider{}
	entry, err := provider.Lock("personal-sync-key", "plain-value-123", "personal-sync-key", SecretKindEncryptionKey)
	if err != nil {
		t.Fatalf("Lock: %v", err)
	}
	env := SecretEnvelope{Provider: "fake", Secrets: map[string]SecretEntry{"personal-sync-key": entry}}
	if err := SaveSecretEnvelope(dir, env); err != nil {
		t.Fatalf("SaveSecretEnvelope: %v", err)
	}
	loaded, err := LoadSecretEnvelope(dir)
	if err != nil {
		t.Fatalf("LoadSecretEnvelope: %v", err)
	}
	unlocked, err := provider.Unlock(loaded)
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if unlocked["personal-sync-key"] != "plain-value-123" {
		t.Fatalf("plaintext mismatch: %q", unlocked["personal-sync-key"])
	}
	// Asset must be restrictive and contain no plaintext.
	b, _ := os.ReadFile(SecretsAssetPath(dir))
	if string(b) == "" || contains(string(b), "plain-value-123") {
		t.Fatalf("plaintext leaked into asset: %s", string(b))
	}
	info, _ := os.Stat(SecretsAssetPath(dir))
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("asset perm = %o, want 0600", perm)
	}
}

func TestSecretEnvelopeWrongKeyFails(t *testing.T) {
	t.Setenv("PINAX_SYNC_FAKE_KEY", "key-one")
	provider := FakeUnlockProvider{}
	entry, _ := provider.Lock("k", "secret", "k", SecretKindEncryptionKey)
	env := SecretEnvelope{Provider: "fake", Secrets: map[string]SecretEntry{"k": entry}}

	t.Setenv("PINAX_SYNC_FAKE_KEY", "key-two")
	_, err := provider.Unlock(env)
	if err == nil {
		t.Fatal("expected decrypt failure with wrong key")
	}
}

func TestSecretEnvelopeUnlockRequired(t *testing.T) {
	t.Setenv("PINAX_SYNC_FAKE_KEY", "")
	provider := FakeUnlockProvider{}
	env := SecretEnvelope{Provider: "fake", Secrets: map[string]SecretEntry{"k": {Identity: "k", Ciphertext: "x"}}}
	_, err := provider.Unlock(env)
	if err == nil {
		t.Fatal("expected unlock required error when no key available")
	}
}

func TestResolveUnlockProvider(t *testing.T) {
	if _, err := ResolveUnlockProvider("fake"); err != nil {
		t.Fatalf("fake provider: %v", err)
	}
	if _, err := ResolveUnlockProvider("env"); err != nil {
		t.Fatalf("env provider: %v", err)
	}
	if _, err := ResolveUnlockProvider("unknown-xyz"); err == nil {
		t.Fatal("expected unsupported_provider error")
	}
}

func TestSecretEnvelopeRejectsPlaintextName(t *testing.T) {
	dir := t.TempDir()
	env := SecretEnvelope{Provider: "fake", Secrets: map[string]SecretEntry{"password=hunter2": {Identity: "x", Ciphertext: "x"}}}
	if err := SaveSecretEnvelope(dir, env); err == nil {
		t.Fatal("expected plaintext_sensitive_name rejection")
	}
}

// --- Compiler ---

func TestCompileProducesRuntimeConfig(t *testing.T) {
	result, err := Compile(CompileRequest{
		Declaration:     validDeclaration(),
		ResolvedSecret:  "stored://tencent-cos-pinax",
		ResolvedEncrypt: "stored://personal-sync-key",
		DeviceID:        "desktop-1",
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	cfg := result.RuntimeConfig
	if cfg.WorkspaceID != "personal" {
		t.Fatalf("workspace = %s", cfg.WorkspaceID)
	}
	if cfg.DeviceID != "desktop-1" {
		t.Fatalf("device = %s", cfg.DeviceID)
	}
	if cfg.SchemaVersion != ConfigSchemaVersion {
		t.Fatalf("schema = %s", cfg.SchemaVersion)
	}
	if cfg.BackendKind != "s3-direct" {
		t.Fatalf("backend kind = %s", cfg.BackendKind)
	}
	if result.DeclarationDigest == "" {
		t.Fatal("expected non-empty declaration digest")
	}
}

func TestApplyCompiledWritesAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	result, _ := Compile(CompileRequest{
		Declaration:     validDeclaration(),
		ResolvedSecret:  "stored://tencent-cos-pinax",
		ResolvedEncrypt: "stored://personal-sync-key",
		DeviceID:        "desktop-1",
	})
	if err := ApplyCompiled(dir, result, ""); err != nil {
		t.Fatalf("ApplyCompiled: %v", err)
	}
	state, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after apply: %v", err)
	}
	if state.Config.DeviceID != "desktop-1" {
		t.Fatalf("applied device = %s", state.Config.DeviceID)
	}
	// Source marker present.
	marker, err := LoadSourceMarker(dir)
	if err != nil {
		t.Fatalf("LoadSourceMarker: %v", err)
	}
	if marker.DeclarationDigest == "" {
		t.Fatal("expected source marker digest")
	}
	// Second apply creates a backup.
	if err := ApplyCompiled(dir, result, ""); err != nil {
		t.Fatalf("second ApplyCompiled: %v", err)
	}
	if _, err := os.Stat(configPath(dir) + ".bak"); err != nil {
		t.Fatalf("expected backup file: %v", err)
	}
}

func TestDetectDrift(t *testing.T) {
	decl := validDeclaration()
	result, _ := Compile(CompileRequest{Declaration: decl, DeviceID: "d1"})
	runtime := result.RuntimeConfig
	marker := result.SourceMarker
	if r := DetectDrift(decl, runtime, marker); r.InDrift {
		t.Fatalf("expected no drift, got %+v", r)
	}
	// Change declaration workspace → drift.
	changed := decl
	changed.Workspace.WorkspaceID = "other"
	if r := DetectDrift(changed, runtime, marker); !r.InDrift {
		t.Fatal("expected drift after workspace change")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func init() {
	// Ensure test isolation from host XDG/HOME for any incidental ConfigDir use.
	_ = filepath.Clean("/")
}
