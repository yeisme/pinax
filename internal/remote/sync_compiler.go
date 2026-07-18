package remote

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// CompileRequest bundles the inputs the declaration→runtime compiler needs: the
// portable declaration, the device-local resolved secret references, the unique
// device id and a clock for receipts/marker timestamps.
type CompileRequest struct {
	Declaration     SyncConfig
	ResolvedSecret  string // device-local secret ref resolved from credential identity
	ResolvedEncrypt string // device-local encryption secret ref resolved from key identity
	DeviceID        string
	Now             time.Time
}

// CompileResult reports what the compiler would write, without touching disk.
// Apply uses Apply(); plan/doctor read Result fields directly.
type CompileResult struct {
	RuntimeConfig     Config
	SourceMarker      SourceMarker
	DeclarationDigest string
}

// Compile produces the device runtime Config and source marker from a
// declaration and resolved secrets. It does NOT write to disk; callers use
// ApplyCompiled or inspect the result for plan output. The compiler reuses the
// existing Capsa config normalization so generated state is identical to a
// manually-logged-in device.
func Compile(req CompileRequest) (CompileResult, error) {
	decl := req.Declaration.Normalized()
	if err := decl.Validate(); err != nil {
		return CompileResult{}, err
	}
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		return CompileResult{}, fmt.Errorf("device id is required")
	}
	now := req.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	endpoint := strings.TrimSpace(decl.Backend.Endpoint)
	// For s3-direct the endpoint may be derived from the S3 topology block.
	if endpoint == "" && decl.Backend.S3 != nil {
		endpoint = endpointFromS3Config(*decl.Backend.S3)
	}
	if endpoint == "" {
		return CompileResult{}, fmt.Errorf("backend endpoint is required")
	}
	createdAt := now.Format(time.RFC3339)
	config := Config{
		SchemaVersion:       ConfigSchemaVersion,
		BackendKind:         decl.Backend.Kind,
		Endpoint:            endpoint,
		WorkspaceID:         decl.Workspace.WorkspaceID,
		DeviceID:            deviceID,
		SecretRef:           strings.TrimSpace(req.ResolvedSecret),
		EncryptionSecretRef: strings.TrimSpace(req.ResolvedEncrypt),
		S3:                  normalizeS3Config(decl.Backend.S3),
		CreatedAt:           createdAt,
		UpdatedAt:           now.Format(time.RFC3339),
	}
	config = normalizeConfig(config)
	digest := HexDigest(MarshalForDigest(decl))
	return CompileResult{
		RuntimeConfig:     config,
		DeclarationDigest: digest,
		SourceMarker: SourceMarker{
			SchemaVersion:     SourceMarkerSchemaVersion,
			DeclarationDigest: digest,
			GeneratedAt:       now.Format(time.RFC3339),
			DeviceID:          deviceID,
		},
	}, nil
}

// ApplyCompiled writes the runtime config and source marker atomically through
// the canonical authoring boundary, preserving the existing CreatedAt when
// regenerating the same workspace. It backs up the prior config before writing
// so apply is restorable.
func ApplyCompiled(root string, result CompileResult, preserveCreatedAt string) error {
	if strings.TrimSpace(preserveCreatedAt) != "" {
		cfg := result.RuntimeConfig
		cfg.CreatedAt = preserveCreatedAt
		result.RuntimeConfig = cfg
	}
	// Backup prior runtime config before overwriting, for rollback.
	if _, err := os.Stat(configPath(root)); err == nil {
		if err := backupConfig(root); err != nil {
			return fmt.Errorf("backup runtime config: %w", err)
		}
	}
	if err := WriteConfig(root, result.RuntimeConfig); err != nil {
		return err
	}
	return writeSourceMarker(root, result.SourceMarker)
}

func backupConfig(root string) error {
	src := configPath(root)
	dst := configPath(root) + ".bak"
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

func writeSourceMarker(root string, marker SourceMarker) error {
	path := SourceMarkerPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(marker)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadSourceMarker reads the device-local source marker. Missing marker is not
// an error — it means the runtime config predates the declaration layer.
func LoadSourceMarker(root string) (SourceMarker, error) {
	b, err := os.ReadFile(SourceMarkerPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SourceMarker{}, nil
		}
		return SourceMarker{}, err
	}
	var marker SourceMarker
	if err := yaml.Unmarshal(b, &marker); err != nil {
		return SourceMarker{}, fmt.Errorf("parse source marker: %w", err)
	}
	return marker, nil
}

// DriftReport describes the difference between a declaration and the local
// generated runtime config. Field is a redacted path; Value is never included.
type DriftReport struct {
	InDrift bool        `json:"in_drift" yaml:"in_drift"`
	Reasons []DriftItem `json:"reasons,omitempty" yaml:"reasons,omitempty"`
}

// DriftItem is one redacted divergence between declaration and runtime state.
type DriftItem struct {
	Field  string `json:"field" yaml:"field"`
	Reason string `json:"reason" yaml:"reason"`
}

// DetectDrift compares a declaration against the loaded runtime state and the
// recorded source marker. It returns redacted field names only.
func DetectDrift(declaration SyncConfig, runtime Config, marker SourceMarker) DriftReport {
	decl := declaration.Normalized()
	report := DriftReport{}
	if marker.DeclarationDigest != "" {
		current := HexDigest(MarshalForDigest(decl))
		if current != marker.DeclarationDigest {
			report.Reasons = append(report.Reasons, DriftItem{Field: "declaration", Reason: "declaration changed since last apply (digest mismatch)"})
		}
	}
	if decl.Workspace.WorkspaceID != strings.TrimSpace(runtime.WorkspaceID) {
		report.Reasons = append(report.Reasons, DriftItem{Field: "workspace.workspace_id", Reason: "workspace id differs from generated runtime config"})
	}
	if decl.Backend.Kind != strings.TrimSpace(runtime.BackendKind) {
		report.Reasons = append(report.Reasons, DriftItem{Field: "backend.kind", Reason: "backend kind differs from generated runtime config"})
	}
	expectedEP := strings.TrimSpace(decl.Backend.Endpoint)
	if expectedEP == "" && decl.Backend.S3 != nil {
		expectedEP = endpointFromS3Config(*decl.Backend.S3)
	}
	if expectedEP != strings.TrimSpace(runtime.Endpoint) {
		report.Reasons = append(report.Reasons, DriftItem{Field: "backend.endpoint", Reason: "backend endpoint differs from generated runtime config"})
	}
	report.InDrift = len(report.Reasons) > 0
	return report
}
