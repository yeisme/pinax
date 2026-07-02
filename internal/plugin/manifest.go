package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yeisme/pinax/internal/redaction"
	"gopkg.in/yaml.v3"
)

const ManifestSchemaVersion = "pinax.plugin.v1"

type RuntimeKind string

const (
	RuntimeWASM       RuntimeKind = "wasm"
	RuntimeJavaScript RuntimeKind = "javascript"
	RuntimePython     RuntimeKind = "python"
	RuntimeProcess    RuntimeKind = "process"
)

type Manifest struct {
	SchemaVersion string       `json:"schema_version" yaml:"schema_version"`
	ID            string       `json:"id" yaml:"id"`
	Name          string       `json:"name" yaml:"name"`
	Version       string       `json:"version" yaml:"version"`
	Runtime       Runtime      `json:"runtime" yaml:"runtime"`
	Capabilities  []Capability `json:"capabilities" yaml:"capabilities"`
	Permissions   Permissions  `json:"permissions" yaml:"permissions"`
	Budgets       Budgets      `json:"budgets" yaml:"budgets"`
	Hooks         []Hook       `json:"hooks,omitempty" yaml:"hooks"`
	Checksum      string       `json:"checksum,omitempty" yaml:"checksum"`
}

type Runtime struct {
	Kind       RuntimeKind `json:"kind" yaml:"kind"`
	Entrypoint string      `json:"entrypoint" yaml:"entrypoint"`
}

type Capability struct {
	ID           string `json:"id" yaml:"id"`
	Kind         string `json:"kind" yaml:"kind"`
	InputSchema  string `json:"input_schema,omitempty" yaml:"input_schema"`
	OutputSchema string `json:"output_schema,omitempty" yaml:"output_schema"`
}

type Hook struct {
	Event      string `json:"event" yaml:"event"`
	Capability string `json:"capability" yaml:"capability"`
}

type Permissions struct {
	Vault      map[string]string `json:"vault,omitempty" yaml:"vault"`
	Filesystem map[string]string `json:"filesystem,omitempty" yaml:"filesystem"`
	Network    bool              `json:"network" yaml:"network"`
}

type Budgets struct {
	TimeoutMS      int `json:"timeout_ms" yaml:"timeout_ms"`
	MaxInputBytes  int `json:"max_input_bytes" yaml:"max_input_bytes"`
	MaxOutputBytes int `json:"max_output_bytes" yaml:"max_output_bytes"`
	MaxMemoryMB    int `json:"max_memory_mb" yaml:"max_memory_mb"`
}

type ValidationResult struct {
	Manifest          Manifest `json:"manifest"`
	ManifestPath      string   `json:"manifest_path,omitempty"`
	Digest            string   `json:"digest"`
	CapabilityCount   int      `json:"capability_count"`
	PermissionSummary string   `json:"permission_summary"`
	WriteStatus       bool     `json:"write_status"`
}

type ValidationIssue struct {
	Code    string
	Field   string
	Message string
}

type ValidationError struct {
	Code   string
	Issues []ValidationIssue
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Issues) == 0 {
		return e.Code
	}
	return e.Code + ": " + e.Issues[0].Message
}

func ValidateManifestPath(path string) (ValidationResult, error) {
	manifestPath, err := resolveManifestPath(path)
	if err != nil {
		return ValidationResult{}, &ValidationError{Code: "plugin_manifest_not_found", Issues: []ValidationIssue{{Code: "plugin_manifest_not_found", Field: "path", Message: "Plugin manifest was not found"}}}
	}
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return ValidationResult{}, &ValidationError{Code: "plugin_manifest_unreadable", Issues: []ValidationIssue{{Code: "plugin_manifest_unreadable", Field: "path", Message: "Plugin manifest could not be read"}}}
	}
	if classes := redaction.ScanSensitiveClasses(string(body)); len(classes) > 0 {
		return ValidationResult{}, &ValidationError{Code: "plugin_manifest_secret_rejected", Issues: []ValidationIssue{{Code: "plugin_manifest_secret_rejected", Field: "manifest", Message: "Plugin manifest contains secret-like content"}}}
	}
	var manifest Manifest
	if strings.HasSuffix(manifestPath, ".json") {
		err = json.Unmarshal(body, &manifest)
	} else {
		err = yaml.Unmarshal(body, &manifest)
	}
	if err != nil {
		return ValidationResult{}, &ValidationError{Code: "plugin_manifest_invalid", Issues: []ValidationIssue{{Code: "plugin_manifest_invalid", Field: "manifest", Message: "Plugin manifest syntax is invalid"}}}
	}
	issues := validateManifest(manifest)
	if len(issues) > 0 {
		return ValidationResult{}, &ValidationError{Code: issues[0].Code, Issues: issues}
	}
	digestBytes := sha256.Sum256(body)
	return ValidationResult{Manifest: manifest, ManifestPath: manifestPath, Digest: hex.EncodeToString(digestBytes[:]), CapabilityCount: len(manifest.Capabilities), PermissionSummary: permissionSummary(manifest), WriteStatus: false}, nil
}

func resolveManifestPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("empty plugin path")
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return trimmed, nil
	}
	for _, name := range []string{"pinax-plugin.yaml", "pinax-plugin.yml", "pinax-plugin.json"} {
		candidate := filepath.Join(trimmed, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("manifest not found")
}

var pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)
var capabilityIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

func validateManifest(manifest Manifest) []ValidationIssue {
	var issues []ValidationIssue
	if manifest.SchemaVersion != ManifestSchemaVersion {
		issues = append(issues, ValidationIssue{Code: "plugin_manifest_schema_invalid", Field: "schema_version", Message: "Plugin manifest schema_version must be pinax.plugin.v1"})
	}
	if !pluginIDPattern.MatchString(manifest.ID) {
		issues = append(issues, ValidationIssue{Code: "plugin_manifest_id_invalid", Field: "id", Message: "Plugin id must be stable lowercase ASCII"})
	}
	if strings.TrimSpace(manifest.Version) == "" {
		issues = append(issues, ValidationIssue{Code: "plugin_manifest_version_required", Field: "version", Message: "Plugin version is required"})
	}
	if !validRuntimeKind(manifest.Runtime.Kind) {
		issues = append(issues, ValidationIssue{Code: "plugin_manifest_runtime_invalid", Field: "runtime.kind", Message: "Plugin runtime kind is not supported"})
	}
	if strings.TrimSpace(manifest.Runtime.Entrypoint) == "" || unsafeRelativePath(manifest.Runtime.Entrypoint) {
		issues = append(issues, ValidationIssue{Code: "plugin_manifest_entrypoint_invalid", Field: "runtime.entrypoint", Message: "Plugin runtime entrypoint must be a safe relative path"})
	}
	if len(manifest.Capabilities) == 0 {
		issues = append(issues, ValidationIssue{Code: "plugin_manifest_capability_required", Field: "capabilities", Message: "At least one plugin capability is required"})
	}
	for i, capability := range manifest.Capabilities {
		field := fmt.Sprintf("capabilities[%d]", i)
		if !capabilityIDPattern.MatchString(capability.ID) {
			issues = append(issues, ValidationIssue{Code: "plugin_manifest_capability_invalid", Field: field + ".id", Message: "Plugin capability id must be stable lowercase ASCII"})
		}
		if !validCapabilityKind(capability.Kind) {
			issues = append(issues, ValidationIssue{Code: "plugin_manifest_capability_invalid", Field: field + ".kind", Message: "Plugin capability kind is not supported"})
		}
	}
	if manifest.Budgets.TimeoutMS <= 0 || manifest.Budgets.MaxInputBytes <= 0 || manifest.Budgets.MaxOutputBytes <= 0 || manifest.Budgets.MaxMemoryMB <= 0 {
		issues = append(issues, ValidationIssue{Code: "plugin_manifest_budget_invalid", Field: "budgets", Message: "Plugin budgets must have positive upper bounds"})
	}
	return issues
}

func validRuntimeKind(kind RuntimeKind) bool {
	switch kind {
	case RuntimeWASM, RuntimeJavaScript, RuntimePython, RuntimeProcess:
		return true
	default:
		return false
	}
}

func validCapabilityKind(kind string) bool {
	switch kind {
	case "query.source.read", "template.function", "import.transform", "export.render", "publish.render", "note.action_plan", "diagnostic.rule", "view.render":
		return true
	default:
		return false
	}
}

func unsafeRelativePath(path string) bool {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	return filepath.IsAbs(cleaned) || cleaned == "." || strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, string(filepath.Separator)+".."+string(filepath.Separator))
}

func permissionSummary(manifest Manifest) string {
	parts := []string{"vault:deny", "fs:deny", "network:false"}
	if len(manifest.Permissions.Vault) > 0 {
		parts[0] = "vault:" + strings.Join(sortedPermissionValues(manifest.Permissions.Vault), ",")
	}
	if len(manifest.Permissions.Filesystem) > 0 {
		parts[1] = "fs:" + strings.Join(sortedPermissionValues(manifest.Permissions.Filesystem), ",")
	}
	parts[2] = fmt.Sprintf("network:%t", manifest.Permissions.Network)
	return strings.Join(parts, ";")
}

func sortedPermissionValues(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
