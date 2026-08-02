package app

import (
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/yeisme/pinax/internal/domain"
)

const (
	KBGenerationManifestSchema   = "pinax.kb.generation.v1"
	KBActivationDescriptorSchema = "pinax.kb.activation.v1"
)

type KBGenerationStatus string

const (
	KBGenerationStatusPlanned    KBGenerationStatus = "planned"
	KBGenerationStatusEmbedding  KBGenerationStatus = "embedding"
	KBGenerationStatusIndexing   KBGenerationStatus = "indexing"
	KBGenerationStatusValidating KBGenerationStatus = "validating"
	KBGenerationStatusReady      KBGenerationStatus = "ready"
	KBGenerationStatusActive     KBGenerationStatus = "active"
	KBGenerationStatusFailed     KBGenerationStatus = "failed"
	KBGenerationStatusRejected   KBGenerationStatus = "rejected"
)

// KBGenerationManifest is the bounded, immutable identity for one semantic
// projection generation. It intentionally contains no note body, vector,
// permission ID list, absolute path, provider payload, or secret.
type KBGenerationManifest struct {
	SchemaVersion       string             `json:"schema_version"`
	GenerationID        string             `json:"generation_id"`
	Status              KBGenerationStatus `json:"status"`
	Protocol            string             `json:"protocol"`
	Backend             string             `json:"backend"`
	Provider            string             `json:"provider"`
	Model               string             `json:"model"`
	BaseModelDigest     string             `json:"base_model_digest,omitempty"`
	ModelManifestDigest string             `json:"model_manifest_digest"`
	ProfileHash         string             `json:"profile_hash"`
	DaemonVersion       string             `json:"daemon_version,omitempty"`
	SourceSnapshot      string             `json:"source_snapshot"`
	SourceDigest        string             `json:"source_digest"`
	EmbeddingDim        int                `json:"embedding_dim"`
	Documents           int                `json:"documents"`
	Chunks              int                `json:"chunks"`
	RowCount            int                `json:"row_count"`
	CreatedAt           string             `json:"created_at"`
}

// KBActivationRef is the only information the active descriptor needs to
// pin a generation and its matching evaluation receipt.
type KBActivationRef struct {
	GenerationID           string `json:"generation_id"`
	Protocol               string `json:"protocol"`
	Provider               string `json:"provider"`
	Model                  string `json:"model"`
	BaseModelDigest        string `json:"base_model_digest,omitempty"`
	ModelManifestDigest    string `json:"model_manifest_digest"`
	ProfileHash            string `json:"profile_hash"`
	DaemonVersion          string `json:"daemon_version,omitempty"`
	EmbeddingDim           int    `json:"embedding_dim"`
	SourceSnapshot         string `json:"source_snapshot"`
	SourceDigest           string `json:"source_digest"`
	GenerationManifestHash string `json:"generation_manifest_hash"`
	EvaluationReceiptHash  string `json:"evaluation_receipt_hash"`
}

// KBActivationDescriptor is the sole authoritative active/previous pointer.
// Previous is a single inline reference; there is deliberately no
// previous.json or second pointer file.
type KBActivationDescriptor struct {
	SchemaVersion string           `json:"schema_version"`
	Sequence      uint64           `json:"sequence"`
	Active        *KBActivationRef `json:"active,omitempty"`
	Previous      *KBActivationRef `json:"previous,omitempty"`
	ActivatedAt   string           `json:"activated_at,omitempty"`
}

func (m KBGenerationManifest) Validate() error {
	if m.SchemaVersion != KBGenerationManifestSchema || !validKBToken(m.GenerationID) || !validKBStatus(m.Status) || m.Protocol != "inferrum.sidecar.v1" || m.Backend == "" || m.Provider == "" || m.Model == "" {
		return invalidKBGenerationManifest("schema, identity, protocol, backend, provider, model, or status is invalid")
	}
	for field, value := range map[string]string{
		"generation_id":         m.GenerationID,
		"source_snapshot":       m.SourceSnapshot,
		"source_digest":         m.SourceDigest,
		"model_manifest_digest": m.ModelManifestDigest,
		"profile_hash":          m.ProfileHash,
	} {
		if !validKBToken(value) {
			return invalidKBGenerationManifest(field + " is invalid")
		}
	}
	if m.BaseModelDigest != "" && !validKBToken(m.BaseModelDigest) {
		return invalidKBGenerationManifest("base_model_digest is invalid")
	}
	if m.DaemonVersion != "" && !validKBToken(m.DaemonVersion) {
		return invalidKBGenerationManifest("daemon_version is invalid")
	}
	if m.EmbeddingDim <= 0 || m.Documents < 0 || m.Chunks < 0 || m.RowCount < 0 || m.RowCount != m.Chunks {
		return invalidKBGenerationManifest("dimension or row counts are invalid")
	}
	if _, err := time.Parse(time.RFC3339, m.CreatedAt); err != nil {
		return invalidKBGenerationManifest("created_at must be RFC3339")
	}
	return nil
}

func (d KBActivationDescriptor) Validate() error {
	if d.SchemaVersion != KBActivationDescriptorSchema {
		return invalidKBActivation("schema_version is invalid")
	}
	if d.Previous != nil && d.Active == nil {
		return invalidKBActivation("previous requires an active generation")
	}
	if d.Active != nil {
		if err := d.Active.Validate(); err != nil {
			return invalidKBActivation("active reference is invalid")
		}
		if _, err := time.Parse(time.RFC3339, d.ActivatedAt); err != nil {
			return invalidKBActivation("activated_at must be RFC3339 when active is set")
		}
	}
	if d.Previous != nil {
		if err := d.Previous.Validate(); err != nil {
			return invalidKBActivation("previous reference is invalid")
		}
		if d.Previous.GenerationID == d.Active.GenerationID {
			return invalidKBActivation("active and previous generation must differ")
		}
	}
	return nil
}

func (r KBActivationRef) Validate() error {
	if !validKBToken(r.GenerationID) || r.Protocol != "inferrum.sidecar.v1" || r.Provider == "" || r.Model == "" || (r.BaseModelDigest != "" && !validKBToken(r.BaseModelDigest)) || (r.DaemonVersion != "" && !validKBToken(r.DaemonVersion)) || r.EmbeddingDim <= 0 || !validKBToken(r.SourceSnapshot) || !validKBToken(r.SourceDigest) || !validKBToken(r.ModelManifestDigest) || !validKBToken(r.ProfileHash) || !validKBToken(r.GenerationManifestHash) || !validKBToken(r.EvaluationReceiptHash) {
		return invalidKBActivation("generation reference identity is incomplete or unsafe")
	}
	return nil
}

func validKBStatus(status KBGenerationStatus) bool {
	switch status {
	case KBGenerationStatusPlanned, KBGenerationStatusEmbedding, KBGenerationStatusIndexing, KBGenerationStatusValidating, KBGenerationStatusReady, KBGenerationStatusActive, KBGenerationStatusFailed, KBGenerationStatusRejected:
		return true
	default:
		return false
	}
}

func validKBToken(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || filepath.IsAbs(value) || strings.Contains(value, "../") || strings.Contains(value, `\`) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func invalidKBGenerationManifest(message string) *domain.CommandError {
	return &domain.CommandError{Code: "kb_generation_manifest_invalid", Message: "KB generation manifest is invalid", Hint: message}
}

func invalidKBActivation(message string) *domain.CommandError {
	return &domain.CommandError{Code: "kb_activation_invalid", Message: "KB activation descriptor is invalid", Hint: message}
}
