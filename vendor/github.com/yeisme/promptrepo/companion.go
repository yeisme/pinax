package promptrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const TemplateContractSchemaVersion = "promptrepo.template-contract.v0.1"
const maxTemplateContractDocumentBytes = 4 << 20

const (
	ContractConsistencySnapshotPinned = "snapshot_pinned"
	ContractConsistencyContentBound   = "content_bound"
)

var companionIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// ContractResolver is an additive, read-only capability for loading the
// companion contract bound to an exact catalog template and snapshot.
type ContractResolver interface {
	ResolveTemplateContract(context.Context, ResolveTemplateContractRequest) (ResolvedTemplateContract, error)
}

type ResolveTemplateContractRequest struct {
	Ref    string `json:"ref" yaml:"ref"`
	Locale string `json:"locale,omitempty" yaml:"locale,omitempty"`
	Role   string `json:"role,omitempty" yaml:"role,omitempty"`
}

// TemplateContractDocument is the canonical wire document authored by the
// Template Registry CLI. It contains metadata and input declarations only;
// prompt template bodies are never embedded.
type TemplateContractDocument struct {
	SchemaVersion  string            `json:"schema_version" yaml:"schema_version"`
	PackageID      string            `json:"package_id" yaml:"package_id"`
	SolutionID     string            `json:"solution_id" yaml:"solution_id"`
	Version        string            `json:"version" yaml:"version"`
	Role           string            `json:"role" yaml:"role"`
	Locale         string            `json:"locale" yaml:"locale"`
	TemplatePath   string            `json:"template_path" yaml:"template_path"`
	TemplateDigest string            `json:"template_digest" yaml:"template_digest"`
	Digest         string            `json:"digest" yaml:"digest"`
	License        string            `json:"license" yaml:"license"`
	Permissions    []string          `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Inputs         []InputDefinition `json:"inputs,omitempty" yaml:"inputs,omitempty"`
}

type ResolvedTemplateContract struct {
	Path        string                   `json:"path" yaml:"path"`
	Consistency string                   `json:"consistency" yaml:"consistency"`
	Document    TemplateContractDocument `json:"document" yaml:"document"`
	Contract    TemplateContract         `json:"contract" yaml:"contract"`
	Snapshot    SnapshotMetadata         `json:"snapshot" yaml:"snapshot"`
}

func DecodeTemplateContractDocument(payload []byte) (TemplateContractDocument, error) {
	if len(payload) > maxTemplateContractDocumentBytes || !utf8.Valid(payload) {
		return TemplateContractDocument{}, NewError(CodeInvalidRequest, "template contract companion must be bounded valid UTF-8 JSON", false, nil)
	}
	var document TemplateContractDocument
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return TemplateContractDocument{}, NewError(CodeInvalidRequest, "template contract companion is invalid", false, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return TemplateContractDocument{}, NewError(CodeInvalidRequest, "template contract companion has trailing JSON data", false, err)
	}
	if err := ValidateTemplateContractDocument(document); err != nil {
		return TemplateContractDocument{}, err
	}
	return document, nil
}

func ValidateTemplateContractDocument(document TemplateContractDocument) error {
	if document.SchemaVersion != TemplateContractSchemaVersion {
		return NewError(CodeInvalidRequest, "template contract companion schema is unsupported", false, nil)
	}
	if !companionIdentifierPattern.MatchString(document.PackageID) || !companionIdentifierPattern.MatchString(document.SolutionID) || !companionIdentifierPattern.MatchString(document.Role) || !validLocale(document.Locale) || strings.TrimSpace(document.Version) == "" {
		return NewError(CodeInvalidRequest, "template contract companion identity is invalid", false, nil)
	}
	if !safeCompanionPath(document.TemplatePath) || !sha256DigestPattern.MatchString(document.TemplateDigest) || !sha256DigestPattern.MatchString(document.Digest) {
		return NewError(CodeInvalidRequest, "template contract companion binding is invalid", false, nil)
	}
	if !validTemplateLicense(document.License) {
		return NewError(CodeInvalidRequest, "template contract companion license is invalid", false, nil)
	}
	permissions := canonicalStrings(document.Permissions)
	if !equalStringSlices(document.Permissions, permissions) {
		return NewError(CodeInvalidRequest, "template contract companion permissions are not canonical", false, nil)
	}
	for _, permission := range permissions {
		if !companionIdentifierPattern.MatchString(permission) {
			return NewError(CodeInvalidRequest, "template contract companion permission is invalid", false, nil)
		}
	}
	for index, input := range document.Inputs {
		if index > 0 && document.Inputs[index-1].Name >= input.Name {
			return NewError(CodeInvalidRequest, "template contract companion inputs are not canonical", false, nil)
		}
	}
	if err := ValidateTemplateInputs(document.Inputs); err != nil {
		return err
	}
	digest, err := CanonicalTemplateContractDigest(document)
	if err != nil {
		return err
	}
	if document.Digest != digest {
		return NewError(CodeDigestMismatch, "template contract companion digest does not match content", false, nil)
	}
	return nil
}

func CanonicalTemplateContractDigest(document TemplateContractDocument) (string, error) {
	clone := document
	clone.Digest = ""
	clone.Permissions = canonicalStrings(clone.Permissions)
	clone.Inputs = append([]InputDefinition(nil), clone.Inputs...)
	sort.Slice(clone.Inputs, func(i, j int) bool { return clone.Inputs[i].Name < clone.Inputs[j].Name })
	payload, err := json.Marshal(clone)
	if err != nil {
		return "", NewError(CodeInvalidRequest, "template contract companion digest failed", false, err)
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

// TemplateContractCompanionPath derives the sidecar location from the
// Registry-owned solution layout: <solution>/prompts/... ->
// <solution>/contracts/<role>.<locale>.json.
func TemplateContractCompanionPath(template TemplateRole) (string, error) {
	if !safeCompanionPath(template.Path) {
		return "", NewError(CodeInvalidRequest, "template path does not support a companion contract", false, nil)
	}
	cleaned := path.Clean(template.Path)
	parts := strings.Split(cleaned, "/")
	promptsIndex := -1
	for index := len(parts) - 2; index >= 0; index-- {
		if parts[index] == "prompts" {
			promptsIndex = index
			break
		}
	}
	if promptsIndex < 0 {
		return "", NewError(CodeInvalidRequest, "template path does not support a companion contract", false, nil)
	}
	if !companionIdentifierPattern.MatchString(template.Role) || !validLocale(template.Locale) {
		return "", NewError(CodeInvalidRequest, "template role or locale is invalid", false, nil)
	}
	solutionDirectory := strings.Join(parts[:promptsIndex], "/")
	return path.Join(solutionDirectory, "contracts", template.Role+"."+template.Locale+".json"), nil
}

func (document TemplateContractDocument) TemplateContract() TemplateContract {
	return TemplateContract{
		Digest: document.Digest, Inputs: append([]InputDefinition(nil), document.Inputs...),
		License: document.License, Permissions: append([]string(nil), document.Permissions...),
	}
}

func safeCompanionPath(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) || strings.ContainsAny(value, "\x00\n\r") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func canonicalStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
