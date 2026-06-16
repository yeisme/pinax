package cloudsync

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	EnvelopeSchemaVersion = "pinax.cloud.envelope.v1"
	HeadSchemaVersion     = "pinax.cloud.head.v1"
	RevisionSchemaVersion = "pinax.cloud.revision.v1"
	ManifestSchemaVersion = "pinax.cloud.manifest.v1"
	ConflictSchemaVersion = "pinax.cloud.conflict.v1"
)

var (
	ErrRevisionConflict = errors.New("revision_conflict")
	ErrObjectNotFound   = errors.New("object_not_found")
	ErrLockHeld         = errors.New("lock_held")
	ErrInvalidEnvelope  = errors.New("invalid_envelope")
)

type Envelope struct {
	SchemaVersion string            `json:"schema_version"`
	Alg           string            `json:"alg"`
	KeyID         string            `json:"key_id"`
	Nonce         string            `json:"nonce"`
	Ciphertext    string            `json:"ciphertext"`
	PlainSHA256   string            `json:"plain_sha256"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func (e Envelope) Validate() error {
	if e.SchemaVersion != EnvelopeSchemaVersion || strings.TrimSpace(e.Alg) == "" || strings.TrimSpace(e.KeyID) == "" || strings.TrimSpace(e.Nonce) == "" || strings.TrimSpace(e.Ciphertext) == "" || strings.TrimSpace(e.PlainSHA256) == "" {
		return ErrInvalidEnvelope
	}
	for key, value := range e.Metadata {
		if unsafePlaintextToken(key + "=" + value) {
			return ErrInvalidEnvelope
		}
	}
	return nil
}

type Head struct {
	SchemaVersion   string `json:"schema_version"`
	VaultID         string `json:"vault_id"`
	CurrentRevision string `json:"current_revision"`
	ManifestBlobID  string `json:"manifest_blob_id"`
	UpdatedAt       string `json:"updated_at"`
	UpdatedByDevice string `json:"updated_by_device"`
}

type Revision struct {
	SchemaVersion    string   `json:"schema_version"`
	RevisionID       string   `json:"revision_id"`
	ParentRevisionID string   `json:"parent_revision_id"`
	ManifestBlobID   string   `json:"manifest_blob_id"`
	BlobIDs          []string `json:"blob_ids"`
	CreatedAt        string   `json:"created_at"`
	CreatedByDevice  string   `json:"created_by_device"`
}

type Manifest struct {
	SchemaVersion string          `json:"schema_version"`
	Entries       []ManifestEntry `json:"entries"`
}

type ManifestEntry struct {
	Path        string `json:"path"`
	BlobID      string `json:"blob_id"`
	PlainSHA256 string `json:"plain_sha256"`
	Size        int64  `json:"size"`
	UpdatedAt   string `json:"updated_at"`
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("invalid_manifest")
	}
	for _, entry := range m.Entries {
		if strings.TrimSpace(entry.Path) == "" || unsafePlaintextToken(entry.BlobID) || strings.TrimSpace(entry.BlobID) == "" || strings.TrimSpace(entry.PlainSHA256) == "" {
			return fmt.Errorf("invalid_manifest")
		}
	}
	return nil
}

func (m Manifest) BlobIDs() []string {
	ids := make([]string, 0, len(m.Entries))
	for _, entry := range m.Entries {
		ids = append(ids, entry.BlobID)
	}
	return ids
}

type Conflict struct {
	SchemaVersion    string `json:"schema_version"`
	PathHash         string `json:"path_hash"`
	LocalBlobID      string `json:"local_blob_id"`
	RemoteBlobID     string `json:"remote_blob_id"`
	BaseRevisionID   string `json:"base_revision_id"`
	RemoteRevisionID string `json:"remote_revision_id"`
}

func (c Conflict) Validate() error {
	if c.SchemaVersion != ConflictSchemaVersion || unsafePlaintextToken(c.PathHash) || strings.TrimSpace(c.PathHash) == "" || strings.TrimSpace(c.LocalBlobID) == "" || strings.TrimSpace(c.RemoteBlobID) == "" {
		return fmt.Errorf("invalid_conflict")
	}
	return nil
}

type ObjectRef struct {
	PathHash string
	BlobID   string
	BlobHash string
	Size     int64
	Deleted  bool
}

type CommitRequest struct {
	BaseRevision   string
	RevisionID     string
	ManifestBlobID string
	BlobIDs        []string
	ObjectRefs     []ObjectRef
	DeviceID       string
	RequestID      string
}

type CommitResult struct {
	RevisionID     string
	RemoteWrite    bool
	ManifestBlobID string
}

type BatchCheckResult struct {
	MissingBlobIDs []string
}

type Lock struct {
	DeviceID  string    `json:"device_id"`
	RequestID string    `json:"request_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Layout struct {
	Prefix      string
	WorkspaceID string
	VaultID     string
}

func (l Layout) ProtocolKey() string { return joinKey(l.Prefix, "protocol.json") }
func (l Layout) HeadKey() string     { return joinKey(l.namespace(), "head.json") }
func (l Layout) LockKey() string     { return joinKey(l.namespace(), "locks", "commit.lock") }
func (l Layout) RevisionKey(revisionID string) string {
	return joinKey(l.namespace(), "revisions", safeID(revisionID)+".json")
}
func (l Layout) ManifestKey(blobID string) string {
	return shardedKey(l.namespace(), "manifests", blobID, ".json")
}
func (l Layout) BlobKey(blobID string) string {
	return shardedKey(l.namespace(), "blobs", blobID, ".json")
}

func (l Layout) namespace() string {
	return joinKey(l.Prefix, "workspaces", safeID(l.WorkspaceID), "vaults", safeID(l.VaultID))
}

func shardedKey(prefix, group, id, suffix string) string {
	safe := safeID(id)
	first, second := "00", "00"
	if len(safe) >= 2 {
		first = safe[:2]
	}
	if len(safe) >= 4 {
		second = safe[2:4]
	}
	return joinKey(prefix, group, "sha256", first, second, safe+suffix)
}

func unsafePlaintextToken(value string) bool {
	lowered := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lowered, "path=") || strings.Contains(lowered, "notes/") || strings.Contains(lowered, ".md") || strings.Contains(lowered, "authorization") || strings.Contains(lowered, "token") || strings.Contains(lowered, "cookie")
}

func safeID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "sha256:", "")
	value = strings.ReplaceAll(value, "blob_", "")
	value = strings.ReplaceAll(value, "manifest_", "")
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			return r
		}
		return '_'
	}, value)
	if value == "" {
		return "default"
	}
	return value
}

func joinKey(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.Trim(part, "/")
		if trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	return path.Join(clean...)
}

type Transport interface {
	CurrentHead(ctx context.Context, vaultID string) (Head, error)
	BatchCheck(ctx context.Context, blobIDs []string) (BatchCheckResult, error)
	PutBlob(ctx context.Context, blobID string, envelope Envelope) error
	GetBlob(ctx context.Context, blobID string) (Envelope, error)
	PutManifest(ctx context.Context, blobID string, envelope Envelope) error
	GetManifest(ctx context.Context, blobID string) (Envelope, error)
	CommitRevision(ctx context.Context, req CommitRequest) (CommitResult, error)
}

type MemoryTransport struct {
	layout    Layout
	mu        sync.Mutex
	blobs     map[string]Envelope
	manifests map[string]Envelope
	revisions map[string]Revision
	head      Head
	lock      *Lock
}

func NewMemoryTransport(layout Layout) *MemoryTransport {
	return &MemoryTransport{layout: layout, blobs: make(map[string]Envelope), manifests: make(map[string]Envelope), revisions: make(map[string]Revision)}
}

func (m *MemoryTransport) CurrentHead(_ context.Context, vaultID string) (Head, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.head.VaultID == "" {
		return Head{SchemaVersion: HeadSchemaVersion, VaultID: vaultID}, nil
	}
	return m.head, nil
}

func (m *MemoryTransport) BatchCheck(_ context.Context, blobIDs []string) (BatchCheckResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	missing := make([]string, 0)
	for _, blobID := range blobIDs {
		if _, ok := m.blobs[blobID]; !ok {
			missing = append(missing, blobID)
		}
	}
	return BatchCheckResult{MissingBlobIDs: missing}, nil
}

func (m *MemoryTransport) PutBlob(_ context.Context, blobID string, envelope Envelope) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blobs[blobID] = envelope
	return nil
}

func (m *MemoryTransport) GetBlob(_ context.Context, blobID string) (Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	envelope, ok := m.blobs[blobID]
	if !ok {
		return Envelope{}, ErrObjectNotFound
	}
	return envelope, nil
}

func (m *MemoryTransport) PutManifest(_ context.Context, blobID string, envelope Envelope) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manifests[blobID] = envelope
	return nil
}

func (m *MemoryTransport) GetManifest(_ context.Context, blobID string) (Envelope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	envelope, ok := m.manifests[blobID]
	if !ok {
		return Envelope{}, ErrObjectNotFound
	}
	return envelope, nil
}

func (m *MemoryTransport) CommitRevision(_ context.Context, req CommitRequest) (CommitResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.head.CurrentRevision != strings.TrimSpace(req.BaseRevision) {
		return CommitResult{}, ErrRevisionConflict
	}
	if _, ok := m.manifests[req.ManifestBlobID]; !ok {
		return CommitResult{}, fmt.Errorf("%w: manifest", ErrObjectNotFound)
	}
	blobIDs := req.BlobIDs
	if len(req.ObjectRefs) > 0 {
		blobIDs = make([]string, 0, len(req.ObjectRefs))
		for _, ref := range req.ObjectRefs {
			if !ref.Deleted {
				blobIDs = append(blobIDs, ref.BlobID)
			}
		}
	}
	for _, blobID := range blobIDs {
		if _, ok := m.blobs[blobID]; !ok {
			return CommitResult{}, fmt.Errorf("%w: %s", ErrObjectNotFound, blobID)
		}
	}
	revisionID := strings.TrimSpace(req.RevisionID)
	if revisionID == "" {
		revisionID = "rev_" + time.Now().UTC().Format("20060102150405")
	}
	m.revisions[revisionID] = Revision{SchemaVersion: RevisionSchemaVersion, RevisionID: revisionID, ParentRevisionID: req.BaseRevision, ManifestBlobID: req.ManifestBlobID, BlobIDs: append([]string(nil), blobIDs...), CreatedAt: time.Now().UTC().Format(time.RFC3339), CreatedByDevice: req.DeviceID}
	m.head = Head{SchemaVersion: HeadSchemaVersion, VaultID: m.layout.VaultID, CurrentRevision: revisionID, ManifestBlobID: req.ManifestBlobID, UpdatedAt: time.Now().UTC().Format(time.RFC3339), UpdatedByDevice: req.DeviceID}
	return CommitResult{RevisionID: revisionID, ManifestBlobID: req.ManifestBlobID, RemoteWrite: true}, nil
}

func (m *MemoryTransport) AcquireLock(_ context.Context, lock Lock, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lock != nil && now.Before(m.lock.ExpiresAt) && m.lock.RequestID != lock.RequestID {
		return ErrLockHeld
	}
	copy := lock
	m.lock = &copy
	return nil
}
