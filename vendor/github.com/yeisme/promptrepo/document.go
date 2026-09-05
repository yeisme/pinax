package promptrepo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	TemplateDocumentSchemaVersion = "promptrepo.template-document.v0.1"

	DocumentCanonicalizationJCS     = "rfc8785"
	DocumentCanonicalizationRawUTF8 = "raw_utf8"

	maxTemplateDocumentDescriptorBytes = 1 << 20
	maxStructuredDocumentBytes         = 2 << 20
	maxStructuredDocumentDepth         = 128
	maxJSONLRecordBytes                = 1 << 20
	maxJSONLRecords                    = 100_000
)

type DocumentFormat string

const (
	DocumentFormatMarkdown DocumentFormat = "markdown"
	DocumentFormatText     DocumentFormat = "text"
	DocumentFormatJSON     DocumentFormat = "json"
	DocumentFormatYAML     DocumentFormat = "yaml"
	DocumentFormatJSONL    DocumentFormat = "jsonl"
)

// DocumentArtifactRef locks a declarative schema or compiler profile. The
// referenced artifact is metadata only; Promptrepo never executes repository
// supplied code.
type DocumentArtifactRef struct {
	Ref    string `json:"ref" yaml:"ref"`
	Digest string `json:"digest" yaml:"digest"`
}

type DocumentLimits struct {
	MaxBytes       int `json:"max_bytes" yaml:"max_bytes"`
	MaxDepth       int `json:"max_depth" yaml:"max_depth"`
	MaxRecordBytes int `json:"max_record_bytes,omitempty" yaml:"max_record_bytes,omitempty"`
	MaxRecords     int `json:"max_records,omitempty" yaml:"max_records,omitempty"`
}

// TemplateDocumentDescriptor is the Registry-authored companion document for
// one exact TemplateRole. Digest is the descriptor digest; SourceDigest keeps
// the existing TemplateRole digest semantics unchanged.
type TemplateDocumentDescriptor struct {
	SchemaVersion        string               `json:"schema_version" yaml:"schema_version"`
	PackageID            string               `json:"package_id" yaml:"package_id"`
	SolutionID           string               `json:"solution_id" yaml:"solution_id"`
	Version              string               `json:"version" yaml:"version"`
	Role                 string               `json:"role" yaml:"role"`
	Locale               string               `json:"locale" yaml:"locale"`
	TemplatePath         string               `json:"template_path" yaml:"template_path"`
	SourceDigest         string               `json:"source_digest" yaml:"source_digest"`
	Format               DocumentFormat       `json:"format" yaml:"format"`
	MediaType            string               `json:"media_type" yaml:"media_type"`
	Canonicalization     string               `json:"canonicalization" yaml:"canonicalization"`
	Schema               *DocumentArtifactRef `json:"schema,omitempty" yaml:"schema,omitempty"`
	CompilerProfile      *DocumentArtifactRef `json:"compiler_profile,omitempty" yaml:"compiler_profile,omitempty"`
	RequiredCapabilities []string             `json:"required_capabilities,omitempty" yaml:"required_capabilities,omitempty"`
	SelectorKinds        []string             `json:"selector_kinds,omitempty" yaml:"selector_kinds,omitempty"`
	RecordIDField        string               `json:"record_id_field,omitempty" yaml:"record_id_field,omitempty"`
	Limits               DocumentLimits       `json:"limits" yaml:"limits"`
	Digest               string               `json:"digest" yaml:"digest"`
}

type ResolveDocumentDescriptorRequest struct {
	Ref    string `json:"ref" yaml:"ref"`
	Locale string `json:"locale,omitempty" yaml:"locale,omitempty"`
	Role   string `json:"role,omitempty" yaml:"role,omitempty"`
}

type ResolvedDocumentDescriptor struct {
	Path        string                     `json:"path" yaml:"path"`
	Consistency string                     `json:"consistency" yaml:"consistency"`
	Document    TemplateDocumentDescriptor `json:"document" yaml:"document"`
	Snapshot    SnapshotMetadata           `json:"snapshot" yaml:"snapshot"`
}

type LoadDocumentRequest struct {
	Ref    string `json:"ref" yaml:"ref"`
	Locale string `json:"locale,omitempty" yaml:"locale,omitempty"`
	Role   string `json:"role,omitempty" yaml:"role,omitempty"`
}

type SelectDocumentRequest struct {
	Ref      string `json:"ref" yaml:"ref"`
	Locale   string `json:"locale,omitempty" yaml:"locale,omitempty"`
	Role     string `json:"role,omitempty" yaml:"role,omitempty"`
	Selector string `json:"selector,omitempty" yaml:"selector,omitempty"`
}

type DocumentFinding struct {
	Code    string   `json:"code" yaml:"code"`
	Message string   `json:"message" yaml:"message"`
	Paths   []string `json:"paths,omitempty" yaml:"paths,omitempty"`
}

// LoadedDocument is safe to serialize: body-bearing fields are explicitly
// excluded from JSON and YAML projections.
type LoadedDocument struct {
	Ref             string                     `json:"ref,omitempty" yaml:"ref,omitempty"`
	Address         string                     `json:"address,omitempty" yaml:"address,omitempty"`
	DescriptorPath  string                     `json:"descriptor_path,omitempty" yaml:"descriptor_path,omitempty"`
	Consistency     string                     `json:"consistency,omitempty" yaml:"consistency,omitempty"`
	Descriptor      TemplateDocumentDescriptor `json:"descriptor" yaml:"descriptor"`
	SourceDigest    string                     `json:"source_digest" yaml:"source_digest"`
	CanonicalDigest string                     `json:"canonical_digest" yaml:"canonical_digest"`
	SourceBytes     int                        `json:"source_bytes" yaml:"source_bytes"`
	CanonicalBytes  int                        `json:"canonical_bytes" yaml:"canonical_bytes"`
	RecordCount     int                        `json:"record_count,omitempty" yaml:"record_count,omitempty"`
	Ready           bool                       `json:"ready" yaml:"ready"`
	Findings        []DocumentFinding          `json:"findings,omitempty" yaml:"findings,omitempty"`
	Snapshot        SnapshotMetadata           `json:"snapshot,omitempty" yaml:"snapshot,omitempty"`
	Body            []byte                     `json:"-" yaml:"-"`
	Value           any                        `json:"-" yaml:"-"`
	jsonlRecords    map[string]jsonlRecord
}

// SelectedDocument keeps the selected body/node in memory while exposing only
// its selector and digest lineage to machine projections.
type SelectedDocument struct {
	Document        LoadedDocument    `json:"document" yaml:"document"`
	Selector        string            `json:"selector" yaml:"selector"`
	CanonicalDigest string            `json:"canonical_digest" yaml:"canonical_digest"`
	SelectedBytes   int               `json:"selected_bytes" yaml:"selected_bytes"`
	RecordID        string            `json:"record_id,omitempty" yaml:"record_id,omitempty"`
	Ready           bool              `json:"ready" yaml:"ready"`
	Findings        []DocumentFinding `json:"findings,omitempty" yaml:"findings,omitempty"`
	Body            []byte            `json:"-" yaml:"-"`
	Value           any               `json:"-" yaml:"-"`
}

type DocumentResolver interface {
	ResolveDocumentDescriptor(context.Context, ResolveDocumentDescriptorRequest) (ResolvedDocumentDescriptor, error)
}

type DocumentLoader interface {
	LoadDocument(context.Context, LoadDocumentRequest) (LoadedDocument, error)
}

type DocumentSelector interface {
	SelectDocument(context.Context, SelectDocumentRequest) (SelectedDocument, error)
}

func DecodeTemplateDocumentDescriptor(payload []byte) (TemplateDocumentDescriptor, error) {
	if len(payload) > maxTemplateDocumentDescriptorBytes || !utf8.Valid(payload) {
		return TemplateDocumentDescriptor{}, NewError(CodeDocumentDescriptorInvalid, "document descriptor must be bounded valid UTF-8 JSON", false, nil)
	}
	if _, err := decodeStrictJSON(payload, 32); err != nil {
		if ErrorCode(err) == CodeDocumentDuplicateKey {
			return TemplateDocumentDescriptor{}, err
		}
		return TemplateDocumentDescriptor{}, NewError(CodeDocumentDescriptorInvalid, "document descriptor JSON is invalid", false, err)
	}
	if _, err := canonicalJSON(payload); err != nil {
		return TemplateDocumentDescriptor{}, NewError(CodeDocumentDescriptorInvalid, "document descriptor is not valid canonicalizable JSON", false, err)
	}
	var document TemplateDocumentDescriptor
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return TemplateDocumentDescriptor{}, NewError(CodeDocumentDescriptorInvalid, "document descriptor is invalid", false, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return TemplateDocumentDescriptor{}, NewError(CodeDocumentDescriptorInvalid, "document descriptor has trailing JSON data", false, err)
	}
	if err := ValidateTemplateDocumentDescriptor(document); err != nil {
		return TemplateDocumentDescriptor{}, err
	}
	return document, nil
}

func ValidateTemplateDocumentDescriptor(document TemplateDocumentDescriptor) error {
	if document.SchemaVersion != TemplateDocumentSchemaVersion {
		return NewError(CodeDocumentDescriptorInvalid, "document descriptor schema is unsupported", false, nil)
	}
	if !companionIdentifierPattern.MatchString(document.PackageID) || !companionIdentifierPattern.MatchString(document.SolutionID) || !companionIdentifierPattern.MatchString(document.Role) || !validLocale(document.Locale) || !refPartPattern.MatchString(document.Version) {
		return NewError(CodeDocumentDescriptorInvalid, "document descriptor identity is invalid", false, nil)
	}
	if !safeCompanionPath(document.TemplatePath) || !sha256DigestPattern.MatchString(document.SourceDigest) || !sha256DigestPattern.MatchString(document.Digest) {
		return NewError(CodeDocumentDescriptorInvalid, "document descriptor binding is invalid", false, nil)
	}
	expectedMediaType, expectedCanonicalization, allowedSelectors, extensions, ok := documentFormatContract(document.Format)
	if !ok || document.MediaType != expectedMediaType || document.Canonicalization != expectedCanonicalization || !pathHasAnyExtension(document.TemplatePath, extensions) {
		return NewError(CodeDocumentFormatMismatch, "document descriptor format, media type, canonicalization, or path extension does not match", false, nil)
	}
	if document.Limits.MaxBytes <= 0 || document.Limits.MaxBytes > maxStructuredDocumentBytes || document.Limits.MaxDepth <= 0 || document.Limits.MaxDepth > maxStructuredDocumentDepth {
		return NewError(CodeDocumentDescriptorInvalid, "document descriptor limits are invalid", false, nil)
	}
	if document.Format == DocumentFormatJSONL {
		if document.Limits.MaxRecordBytes <= 0 || document.Limits.MaxRecordBytes > maxJSONLRecordBytes || document.Limits.MaxRecordBytes > document.Limits.MaxBytes || document.Limits.MaxRecords <= 0 || document.Limits.MaxRecords > maxJSONLRecords || !companionIdentifierPattern.MatchString(document.RecordIDField) {
			return NewError(CodeDocumentDescriptorInvalid, "JSONL document limits or record id field are invalid", false, nil)
		}
	} else if document.Limits.MaxRecordBytes != 0 || document.Limits.MaxRecords != 0 || document.RecordIDField != "" {
		return NewError(CodeDocumentDescriptorInvalid, "non-JSONL document must not declare JSONL record settings", false, nil)
	}
	selectors := canonicalStrings(document.SelectorKinds)
	if !equalStringSlices(document.SelectorKinds, selectors) {
		return NewError(CodeDocumentDescriptorInvalid, "document selector kinds are not canonical", false, nil)
	}
	for _, selector := range selectors {
		if _, allowed := allowedSelectors[selector]; !allowed {
			return NewError(CodeSelectorUnsupported, "document selector kind is not compatible with the declared format", false, nil)
		}
	}
	if err := validateDocumentArtifactRef(document.Schema); err != nil {
		return err
	}
	if err := validateDocumentArtifactRef(document.CompilerProfile); err != nil {
		return err
	}
	capabilities := canonicalStrings(document.RequiredCapabilities)
	if !equalStringSlices(document.RequiredCapabilities, capabilities) {
		return NewError(CodeDocumentDescriptorInvalid, "document required capabilities are not canonical", false, nil)
	}
	for _, capability := range capabilities {
		if !companionIdentifierPattern.MatchString(capability) {
			return NewError(CodeDocumentDescriptorInvalid, "document required capability is invalid", false, nil)
		}
	}
	digest, err := CanonicalTemplateDocumentDescriptorDigest(document)
	if err != nil {
		return err
	}
	if document.Digest != digest {
		return NewError(CodeDigestMismatch, "document descriptor digest does not match content", false, nil)
	}
	return nil
}

func CanonicalTemplateDocumentDescriptorDigest(document TemplateDocumentDescriptor) (string, error) {
	clone := document
	clone.Digest = ""
	clone.RequiredCapabilities = canonicalStrings(clone.RequiredCapabilities)
	clone.SelectorKinds = canonicalStrings(clone.SelectorKinds)
	payload, err := json.Marshal(clone)
	if err != nil {
		return "", NewError(CodeDocumentCanonicalizeFailed, "document descriptor canonicalization failed", false, err)
	}
	canonical, err := canonicalJSON(payload)
	if err != nil {
		return "", err
	}
	return digestBytes(canonical), nil
}

func ValidateTemplateDocumentDescriptorBinding(document TemplateDocumentDescriptor, solution Solution, template TemplateRole) error {
	if document.PackageID != solution.PackageID || document.SolutionID != solution.ID || document.Version != solution.Version || document.Role != template.Role || document.Locale != template.Locale {
		return NewError(CodeAddressMismatch, "document descriptor identity does not match the resolved template", false, nil)
	}
	if document.TemplatePath != template.Path || document.SourceDigest != template.Digest {
		return NewError(CodeDigestMismatch, "document descriptor does not match the resolved template body", false, nil)
	}
	return nil
}

// TemplateDocumentDescriptorPath derives
// <solution>/contracts/documents/<role>.<locale>.document.json from the
// existing Registry solution layout.
func TemplateDocumentDescriptorPath(template TemplateRole) (string, error) {
	if !safeCompanionPath(template.Path) {
		return "", NewError(CodeInvalidRequest, "template path does not support a document descriptor", false, nil)
	}
	parts := strings.Split(path.Clean(template.Path), "/")
	promptsIndex := -1
	for index := len(parts) - 2; index >= 0; index-- {
		if parts[index] == "prompts" {
			promptsIndex = index
			break
		}
	}
	if promptsIndex < 0 || !companionIdentifierPattern.MatchString(template.Role) || !validLocale(template.Locale) {
		return "", NewError(CodeInvalidRequest, "template path, role, or locale does not support a document descriptor", false, nil)
	}
	return path.Join(strings.Join(parts[:promptsIndex], "/"), "contracts", "documents", template.Role+"."+template.Locale+".document.json"), nil
}

func validateDocumentArtifactRef(reference *DocumentArtifactRef) error {
	if reference == nil {
		return nil
	}
	if strings.TrimSpace(reference.Ref) != reference.Ref || reference.Ref == "" || len(reference.Ref) > 2048 || strings.ContainsAny(reference.Ref, "\x00\t\n\r ") || !sha256DigestPattern.MatchString(reference.Digest) {
		return NewError(CodeDocumentSchemaInvalid, "document schema or compiler profile reference is invalid", false, nil)
	}
	return nil
}

func documentFormatContract(format DocumentFormat) (string, string, map[string]struct{}, []string, bool) {
	switch format {
	case DocumentFormatMarkdown:
		return "text/markdown", DocumentCanonicalizationRawUTF8, map[string]struct{}{"heading": {}}, []string{".md", ".markdown"}, true
	case DocumentFormatText:
		return "text/plain", DocumentCanonicalizationRawUTF8, map[string]struct{}{}, []string{".txt"}, true
	case DocumentFormatJSON:
		return "application/json", DocumentCanonicalizationJCS, map[string]struct{}{"json-pointer": {}}, []string{".json"}, true
	case DocumentFormatYAML:
		return "application/yaml", DocumentCanonicalizationJCS, map[string]struct{}{"yaml-pointer": {}}, []string{".yaml", ".yml"}, true
	case DocumentFormatJSONL:
		return "application/x-ndjson", DocumentCanonicalizationJCS, map[string]struct{}{"jsonl-id": {}}, []string{".jsonl", ".ndjson"}, true
	default:
		return "", "", nil, nil, false
	}
}

func pathHasAnyExtension(value string, extensions []string) bool {
	lower := strings.ToLower(value)
	for _, extension := range extensions {
		if strings.HasSuffix(lower, extension) {
			return true
		}
	}
	return false
}

func digestBytes(payload []byte) string {
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}
