// Package syncwire owns the on-wire types shared by the cloud sync stack:
// the encrypted envelope (pinax.cloud.envelope.v1) and the content manifest
// (pinax.cloud.manifest.v1/v2). remote and cloudsync previously declared
// parallel copies of these schemas with drifting field sets — this package is
// the single source of truth; the other packages alias these types so the
// wire format can no longer diverge silently.
package syncwire

import (
	"fmt"
	"strings"
)

const (
	// EnvelopeSchemaVersion is the encrypted-envelope wire schema.
	EnvelopeSchemaVersion = "pinax.cloud.envelope.v1"
	// ManifestSchemaVersionV1 is the path-keyed manifest wire schema.
	ManifestSchemaVersionV1 = "pinax.cloud.manifest.v1"
	// ManifestSchemaVersionV2 is the identity-first manifest wire schema.
	ManifestSchemaVersionV2 = "pinax.cloud.manifest.v2"
	// ManifestSchemaVersion is the schema written by new manifests.
	ManifestSchemaVersion = ManifestSchemaVersionV1
)

// Envelope is the encrypted payload wrapper for blobs and manifests.
type Envelope struct {
	SchemaVersion string            `json:"schema_version"`
	Alg           string            `json:"alg"`
	KeyID         string            `json:"key_id"`
	Nonce         string            `json:"nonce"`
	Ciphertext    string            `json:"ciphertext"`
	PlainSHA256   string            `json:"plain_sha256"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// Validate enforces the envelope schema and rejects metadata that could leak
// plaintext tokens.
func (e Envelope) Validate() error {
	if e.SchemaVersion != EnvelopeSchemaVersion || strings.TrimSpace(e.Alg) == "" || strings.TrimSpace(e.KeyID) == "" || strings.TrimSpace(e.Nonce) == "" || strings.TrimSpace(e.Ciphertext) == "" || strings.TrimSpace(e.PlainSHA256) == "" {
		return fmt.Errorf("invalid_envelope")
	}
	for key, value := range e.Metadata {
		if IsUnsafePlaintextToken(key + "=" + value) {
			return fmt.Errorf("invalid_envelope")
		}
	}
	return nil
}

// Manifest is the content manifest serialized inside manifest envelopes.
type Manifest struct {
	SchemaVersion string           `json:"schema_version"`
	GeneratedAt   string           `json:"generated_at"`
	EntryCount    int              `json:"entry_count"`
	Entries       []ManifestEntry  `json:"entries"`
	Deletes       []ManifestDelete `json:"deletes,omitempty"`
}

// ManifestEntry is one object in a manifest.
type ManifestEntry struct {
	ObjectID   string `json:"object_id,omitempty"`
	RevisionID string `json:"revision_id,omitempty"`
	DeviceID   string `json:"device_id,omitempty"`
	Path       string `json:"path"`
	PathHash   string `json:"path_hash"`
	BlobID     string `json:"blob_id"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	ObjectKind string `json:"object_kind,omitempty"`
	Mode       uint32 `json:"mode,omitempty"`
	MediaType  string `json:"media_type,omitempty"`
	UpdatedAt  string `json:"updated_at"`
}

// ManifestDelete is one delete-marker tombstone in a manifest.
type ManifestDelete struct {
	PathHash    string `json:"path_hash"`
	ObjectKind  string `json:"object_kind"`
	ObjectID    string `json:"object_id,omitempty"`
	TombstoneID string `json:"tombstone_id"`
	DeletedAt   string `json:"deleted_at,omitempty"`
	TrashBlobID string `json:"trash_blob_id,omitempty"`
	RevisionID  string `json:"revision_id,omitempty"`
	DeviceID    string `json:"device_id,omitempty"`
}

// Validate enforces manifest structure per schema version.
func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersionV1 && m.SchemaVersion != ManifestSchemaVersionV2 {
		return fmt.Errorf("invalid_manifest")
	}
	objectIDs := map[string]bool{}
	paths := map[string]bool{}
	for _, entry := range m.Entries {
		if strings.TrimSpace(entry.Path) == "" || IsUnsafePlaintextToken(entry.BlobID) || strings.TrimSpace(entry.BlobID) == "" || strings.TrimSpace(entry.SHA256) == "" {
			return fmt.Errorf("invalid_manifest")
		}
		if m.SchemaVersion == ManifestSchemaVersionV2 {
			if strings.TrimSpace(entry.ObjectID) == "" || strings.TrimSpace(entry.ObjectKind) == "" || strings.TrimSpace(entry.RevisionID) == "" || strings.TrimSpace(entry.DeviceID) == "" || objectIDs[entry.ObjectID] || paths[entry.Path] {
				return fmt.Errorf("invalid_manifest")
			}
			objectIDs[entry.ObjectID] = true
			paths[entry.Path] = true
		}
	}
	for _, deleteMarker := range m.Deletes {
		if strings.TrimSpace(deleteMarker.PathHash) == "" || IsUnsafePlaintextToken(deleteMarker.PathHash) || strings.TrimSpace(deleteMarker.ObjectKind) == "" || strings.TrimSpace(deleteMarker.TombstoneID) == "" || IsUnsafePlaintextToken(deleteMarker.TombstoneID) {
			return fmt.Errorf("invalid_manifest")
		}
		if strings.TrimSpace(deleteMarker.TrashBlobID) != "" && IsUnsafePlaintextToken(deleteMarker.TrashBlobID) {
			return fmt.Errorf("invalid_manifest")
		}
		if m.SchemaVersion == ManifestSchemaVersionV2 && (strings.TrimSpace(deleteMarker.ObjectID) == "" || strings.TrimSpace(deleteMarker.RevisionID) == "" || strings.TrimSpace(deleteMarker.DeviceID) == "") {
			return fmt.Errorf("invalid_manifest")
		}
	}
	return nil
}

// BlobIDs lists every blob referenced by the manifest entries, including
// trash backup blobs held by delete markers.
func (m Manifest) BlobIDs() []string {
	ids := make([]string, 0, len(m.Entries)+len(m.Deletes))
	for _, entry := range m.Entries {
		ids = append(ids, entry.BlobID)
	}
	for _, deleteMarker := range m.Deletes {
		if strings.TrimSpace(deleteMarker.TrashBlobID) != "" {
			ids = append(ids, deleteMarker.TrashBlobID)
		}
	}
	return ids
}

// IsUnsafePlaintextToken reports whether a value may carry plaintext paths or
// credentials that must never appear in wire metadata.
func IsUnsafePlaintextToken(value string) bool {
	lowered := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lowered, "path=") || strings.Contains(lowered, "notes/") || strings.Contains(lowered, ".pinax/trash") || strings.Contains(lowered, ".md") || strings.Contains(lowered, "authorization") || strings.Contains(lowered, "token") || strings.Contains(lowered, "cookie")
}

func (m Manifest) ValidateV2() error {
	if m.SchemaVersion != ManifestSchemaVersionV2 {
		return fmt.Errorf("invalid_manifest_version")
	}
	objectIDs := make(map[string]struct{}, len(m.Entries))
	paths := make(map[string]struct{}, len(m.Entries))
	for _, entry := range m.Entries {
		if strings.TrimSpace(entry.ObjectID) == "" || strings.TrimSpace(entry.ObjectKind) == "" || strings.TrimSpace(entry.Path) == "" || strings.TrimSpace(entry.RevisionID) == "" || strings.TrimSpace(entry.DeviceID) == "" || strings.TrimSpace(entry.UpdatedAt) == "" {
			return fmt.Errorf("invalid_manifest_v2_entry")
		}
		if _, exists := objectIDs[entry.ObjectID]; exists {
			return fmt.Errorf("duplicate_manifest_object_id")
		}
		if _, exists := paths[entry.Path]; exists {
			return fmt.Errorf("duplicate_manifest_path")
		}
		objectIDs[entry.ObjectID] = struct{}{}
		paths[entry.Path] = struct{}{}
	}
	for _, deleteMarker := range m.Deletes {
		if strings.TrimSpace(deleteMarker.ObjectID) == "" || strings.TrimSpace(deleteMarker.ObjectKind) == "" || strings.TrimSpace(deleteMarker.RevisionID) == "" || strings.TrimSpace(deleteMarker.DeviceID) == "" {
			return fmt.Errorf("invalid_manifest_v2_tombstone")
		}
	}
	return nil
}
