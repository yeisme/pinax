package remote

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Declaration schema versions. These are the repository-tracked, portable
// declaration layer — distinct from the device-owned pinax.cloud.config.v1
// runtime state that the compiler emits.
const (
	SyncConfigSchemaVersion   = "pinax.sync.config.v1"
	SyncSecretsSchemaVersion  = "pinax.sync.secrets.v1"
	SourceMarkerSchemaVersion = "pinax.sync.source-marker.v1"
)

// DeclarationFilePaths are the CLI-authored structured assets inside the vault.
// Business code and agents MUST NOT assemble these by hand.
const (
	DeclarationFileName  = "pinax-sync.yaml"
	SecretsAssetFileName = "pinax-sync.secrets.yaml"
	SourceMarkerFileName = "pinax-sync.source.yaml"
)

// SyncConfig is the versioned, portable repository sync declaration. It holds
// backend topology, logical credential/encryption identities and sync policy,
// but never plaintext credentials, tokens, absolute device paths or device
// runtime state.
type SyncConfig struct {
	SchemaVersion string         `json:"schema_version" yaml:"schema_version"`
	Backend       SyncBackend    `json:"backend" yaml:"backend"`
	Workspace     SyncWorkspace  `json:"workspace" yaml:"workspace"`
	Secrets       SyncSecretRefs `json:"secrets" yaml:"secrets"`
	Policy        SyncPolicy     `json:"policy,omitempty" yaml:"policy,omitempty"`
}

// SyncBackend describes the portable transport topology. Endpoint and S3 are
// topology only; provider credential values resolve at device-local unlock.
type SyncBackend struct {
	Kind     string    `json:"kind" yaml:"kind"` // s3-direct, rclone-direct, server, embedded
	Endpoint string    `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	S3       *S3Config `json:"s3,omitempty" yaml:"s3,omitempty"`
}

// SyncWorkspace carries stable namespace fields. The effective remote namespace
// is derived deterministically; full multi-tenant authorization is a separate
// server capability and is NOT provided by direct transport.
type SyncWorkspace struct {
	TenantID    string `json:"tenant_id,omitempty" yaml:"tenant_id,omitempty"`
	AppID       string `json:"app_id,omitempty" yaml:"app_id,omitempty"`
	WorkspaceID string `json:"workspace_id" yaml:"workspace_id"`
}

// SyncSecretRefs holds logical credential and encryption key identities. Each
// device resolves these to its local profile, keychain or secret manager; raw
// values never enter the declaration.
type SyncSecretRefs struct {
	CredentialID    string `json:"credential_id,omitempty" yaml:"credential_id,omitempty"`
	EncryptionKeyID string `json:"encryption_key_id" yaml:"encryption_key_id"`
}

// SyncPolicy captures approval-gated, safety-critical knobs. The default is the
// most conservative posture so an accidental apply cannot widen the blast radius.
type SyncPolicy struct {
	// RemoteDeletePolicy controls whether local deletions may propagate to the
	// remote. "deny" (default) refuses; "require-approval" needs explicit --yes.
	RemoteDeletePolicy string `json:"remote_delete_policy,omitempty" yaml:"remote_delete_policy,omitempty"`
	// NewDeviceMode defaults to "pull-only": a device with no local sync receipt
	// must not upload local deletions or replace remote state on first bootstrap.
	NewDeviceMode string `json:"new_device_mode,omitempty" yaml:"new_device_mode,omitempty"`
}

// SourceMarker is the device-local record that a runtime config was generated
// from a declaration. It stores the declaration digest so doctor can detect
// drift without re-reading the (possibly absent) original declaration.
type SourceMarker struct {
	SchemaVersion     string `json:"schema_version" yaml:"schema_version"`
	DeclarationDigest string `json:"declaration_digest,omitempty" yaml:"declaration_digest,omitempty"`
	GeneratedAt       string `json:"generated_at" yaml:"generated_at"`
	DeviceID          string `json:"device_id" yaml:"device_id"`
}

// SupportedSyncBackendKinds is the closed set of backend kinds the declaration
// layer accepts. Unknown kinds are rejected so an unsupported backend cannot
// silently compile to a half-usable runtime config.
var SupportedSyncBackendKinds = map[string]bool{
	"s3-direct":     true,
	"rclone-direct": true,
	"server":        true,
	"embedded":      true,
}

// ValidRemoteDeletePolicies enumerates the conservative policy values.
var ValidRemoteDeletePolicies = map[string]bool{
	"":                 true, // default → deny
	"deny":             true,
	"require-approval": true,
}

// DeclarationPath returns the path to the repository sync declaration.
func DeclarationPath(root string) string {
	return filepath.Join(root, ".pinax", DeclarationFileName)
}

// SecretsAssetPath returns the path to the encrypted secrets asset.
func SecretsAssetPath(root string) string {
	return filepath.Join(root, ".pinax", SecretsAssetFileName)
}

// SourceMarkerPath returns the device-local source marker path (under cloud/ so
// it is treated as device runtime state, never shared).
func SourceMarkerPath(root string) string {
	return filepath.Join(root, ".pinax", "cloud", SourceMarkerFileName)
}

// LoadSyncConfig reads and validates the repository declaration. A missing file
// is reported via ErrSyncDeclarationMissing so callers can distinguish "not
// initialized" from "corrupt".
func LoadSyncConfig(root string) (SyncConfig, error) {
	cfg, err := readSyncConfigFile(DeclarationPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SyncConfig{}, ErrSyncDeclarationMissing
		}
		return SyncConfig{}, err
	}
	if err := cfg.Validate(); err != nil {
		return SyncConfig{}, err
	}
	return cfg, nil
}

// ErrSyncDeclarationMissing is returned when no pinax-sync.yaml exists.
var ErrSyncDeclarationMissing = errors.New("sync repository declaration not found")

func readSyncConfigFile(path string) (SyncConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SyncConfig{}, err
	}
	var cfg SyncConfig
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return SyncConfig{}, fmt.Errorf("parse sync declaration: %w", err)
	}
	return cfg, nil
}

// writeSyncConfig persists the declaration through the canonical authoring
// boundary with restrictive permissions. It rejects plaintext-sensitive fields
// before touching disk.
func writeSyncConfig(root string, cfg SyncConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	path := DeclarationPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create declaration directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal sync declaration: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// Validate enforces the declaration contract: schema version, supported backend,
// required workspace/encryption identity and absence of plaintext-sensitive or
// absolute-path fields. It returns stable English error codes for every failure.
func (c SyncConfig) Validate() error {
	if strings.TrimSpace(c.SchemaVersion) != "" && c.SchemaVersion != SyncConfigSchemaVersion {
		return &SyncConfigError{Code: "unsupported_schema_version", Field: "schema_version", Message: fmt.Sprintf("unsupported schema version: %s", c.SchemaVersion)}
	}
	c.SchemaVersion = SyncConfigSchemaVersion
	if !SupportedSyncBackendKind(c.Backend.Kind) {
		return &SyncConfigError{Code: "unsupported_backend_kind", Field: "backend.kind", Message: fmt.Sprintf("unsupported backend kind: %q", c.Backend.Kind)}
	}
	if strings.TrimSpace(c.Workspace.WorkspaceID) == "" {
		return &SyncConfigError{Code: "missing_workspace_id", Field: "workspace.workspace_id", Message: "workspace_id is required"}
	}
	if strings.TrimSpace(c.Secrets.EncryptionKeyID) == "" {
		return &SyncConfigError{Code: "missing_encryption_key_id", Field: "secrets.encryption_key_id", Message: "encryption_key_id is required"}
	}
	if !ValidRemoteDeletePolicies[c.Policy.RemoteDeletePolicy] {
		return &SyncConfigError{Code: "invalid_remote_delete_policy", Field: "policy.remote_delete_policy", Message: fmt.Sprintf("invalid remote_delete_policy: %q", c.Policy.RemoteDeletePolicy)}
	}
	if field, hit := scanForPlaintextSensitive(c); hit {
		return &SyncConfigError{Code: "plaintext_sensitive_field", Field: field, Message: fmt.Sprintf("declaration field %s contains a plaintext-sensitive value; use `pinax sync repo secret set` or an external credential reference", field)}
	}
	if ep := strings.TrimSpace(c.Backend.Endpoint); ep != "" {
		u, err := url.Parse(ep)
		if err != nil || u.Scheme == "" {
			return &SyncConfigError{Code: "invalid_endpoint", Field: "backend.endpoint", Message: fmt.Sprintf("invalid endpoint URI: %s", ep)}
		}
		if !IsSupportedScheme(u.Scheme) {
			return &SyncConfigError{Code: "unsupported_scheme", Field: "backend.endpoint", Message: fmt.Sprintf("unsupported remote scheme: %s", u.Scheme)}
		}
	}
	return nil
}

// SupportedSyncBackendKind reports whether kind is a supported declaration backend.
func SupportedSyncBackendKind(kind string) bool {
	return SupportedSyncBackendKinds[strings.TrimSpace(kind)]
}

// Normalized returns a copy with trimmed/derived fields, suitable for stable
// comparison (drift detection) and compilation.
func (c SyncConfig) Normalized() SyncConfig {
	out := c
	out.SchemaVersion = SyncConfigSchemaVersion
	out.Backend.Kind = strings.TrimSpace(out.Backend.Kind)
	out.Backend.Endpoint = strings.TrimRight(strings.TrimSpace(out.Backend.Endpoint), "/")
	out.Workspace.TenantID = strings.TrimSpace(out.Workspace.TenantID)
	out.Workspace.AppID = strings.TrimSpace(out.Workspace.AppID)
	out.Workspace.WorkspaceID = strings.TrimSpace(out.Workspace.WorkspaceID)
	out.Secrets.CredentialID = strings.TrimSpace(out.Secrets.CredentialID)
	out.Secrets.EncryptionKeyID = strings.TrimSpace(out.Secrets.EncryptionKeyID)
	if out.Policy.RemoteDeletePolicy == "" {
		out.Policy.RemoteDeletePolicy = "deny"
	}
	if out.Policy.NewDeviceMode == "" {
		out.Policy.NewDeviceMode = "pull-only"
	}
	out.Backend.S3 = normalizeS3Config(out.Backend.S3)
	return out
}

// EffectiveNamespace derives the deterministic remote namespace prefix from the
// stable workspace fields. It does NOT replace server-side authorization; it
// only makes the on-wire object layout collision-resistant.
func (c SyncConfig) EffectiveNamespace() string {
	c = c.Normalized()
	parts := []string{}
	if c.Workspace.TenantID != "" {
		parts = append(parts, "t-"+c.Workspace.TenantID)
	}
	if c.Workspace.AppID != "" {
		parts = append(parts, "a-"+c.Workspace.AppID)
	}
	parts = append(parts, "w-"+c.Workspace.WorkspaceID)
	return strings.Join(parts, "/")
}

// SyncConfigError is the stable validation error returned by Validate. Code is
// a stable English identifier; Field is the redacted field path (never the
// offending value).
type SyncConfigError struct {
	Code    string
	Field   string
	Message string
}

func (e *SyncConfigError) Error() string { return e.Message }

// IsSyncConfigError reports whether err is a *SyncConfigError with the given code.
func IsSyncConfigError(err error, code string) bool {
	var sce *SyncConfigError
	if errors.As(err, &sce) {
		return code == "" || sce.Code == code
	}
	return false
}

func SupportedSyncBackendKings(kind string) bool {
	return SupportedSyncBackendKinds[strings.TrimSpace(kind)]
}

// plaintextSensitivePattern matches literal token/password/secret values,
// absolute machine paths and pinax-internal references that must never appear
// in a portable declaration. Mirrors internal/redaction semantics so the
// declaration layer rejects at authoring time what the output layer redacts.
var plaintextSensitivePattern = regexp.MustCompile(`(?i)(authorization\s*[:=]?\s*bearer\s+\S+|(^|[/._-])(token|api[_-]?key|access[_-]?key|secret[_-]?key|password|secret_token)([/._=-]|$)|secret\s*[:=]\s*\S+|(/Users/|/home/|[a-z]:\\))`)

// scanForPlaintextSensitive walks the user-supplied declaration string fields
// and reports the first field path that carries a plaintext-sensitive value.
func scanForPlaintextSensitive(c SyncConfig) (string, bool) {
	fields := map[string]string{
		"backend.endpoint":            c.Backend.Endpoint,
		"backend.s3.profile":          s3Profile(c.Backend.S3),
		"workspace.workspace_id":      c.Workspace.WorkspaceID,
		"workspace.tenant_id":         c.Workspace.TenantID,
		"workspace.app_id":            c.Workspace.AppID,
		"secrets.credential_id":       c.Secrets.CredentialID,
		"secrets.encryption_key_id":   c.Secrets.EncryptionKeyID,
		"policy.remote_delete_policy": c.Policy.RemoteDeletePolicy,
	}
	for field, value := range fields {
		if plaintextSensitivePattern.MatchString(value) {
			return field, true
		}
	}
	return "", false
}

func s3Profile(s3 *S3Config) string {
	if s3 == nil {
		return ""
	}
	return s3.Profile
}
