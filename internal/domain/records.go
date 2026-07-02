package domain

import (
	"errors"
)

type NoteLifecycle string

const (
	NoteLifecycleActive   NoteLifecycle = "active"
	NoteLifecycleArchived NoteLifecycle = "archived"
	NoteLifecycleTrashed  NoteLifecycle = "trashed"
	NoteLifecycleDeleted  NoteLifecycle = "deleted"
)

type RecordEventKind string

const (
	RecordEventNoteCreated         RecordEventKind = "note.created"
	RecordEventNoteRenamed         RecordEventKind = "note.renamed"
	RecordEventNoteMoved           RecordEventKind = "note.moved"
	RecordEventNoteArchived        RecordEventKind = "note.archived"
	RecordEventNoteTrashed         RecordEventKind = "note.trashed"
	RecordEventNoteDeleted         RecordEventKind = "note.deleted"
	RecordEventNoteRestored        RecordEventKind = "note.restored"
	RecordEventNoteMetadataUpdated RecordEventKind = "note.metadata_updated"
)

type ContentRevision struct {
	Hash         string `json:"hash,omitempty"`
	Size         int64  `json:"size,omitempty"`
	ModifiedUnix int64  `json:"modified_unix,omitempty"`
}

type VersionEvidence struct {
	Backend       string `json:"backend"`
	RevisionID    string `json:"revision_id,omitempty"`
	WorktreeState string `json:"worktree_state,omitempty"`
	FileBlobID    string `json:"file_blob_id,omitempty"`
	DiffHash      string `json:"diff_hash,omitempty"`
}

type RecordEvent struct {
	SchemaVersion   string          `json:"schema_version"`
	EventID         string          `json:"event_id"`
	Seq             uint64          `json:"seq"`
	IdempotencyKey  string          `json:"idempotency_key"`
	Kind            RecordEventKind `json:"kind"`
	NoteID          string          `json:"note_id"`
	Path            string          `json:"path,omitempty"`
	OldPath         string          `json:"old_path,omitempty"`
	Title           string          `json:"title,omitempty"`
	Lifecycle       NoteLifecycle   `json:"lifecycle,omitempty"`
	ContentRevision ContentRevision `json:"content_revision,omitempty"`
	VersionEvidence VersionEvidence `json:"version_evidence,omitempty"`
	Evidence        []string        `json:"evidence,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

type NoteRecord struct {
	NoteID          string          `json:"note_id"`
	Path            string          `json:"path"`
	Title           string          `json:"title,omitempty"`
	Lifecycle       NoteLifecycle   `json:"lifecycle"`
	RecordVersion   uint64          `json:"record_version"`
	LedgerSeq       uint64          `json:"ledger_seq"`
	ContentRevision ContentRevision `json:"content_revision,omitempty"`
	VersionEvidence VersionEvidence `json:"version_evidence,omitempty"`
}

type Tombstone struct {
	NoteID        string         `json:"note_id"`
	ObjectKind    string         `json:"object_kind,omitempty"`
	ObjectID      string         `json:"object_id,omitempty"`
	TombstoneID   string         `json:"tombstone_id,omitempty"`
	OldPath       string         `json:"old_path"`
	OldHash       string         `json:"old_hash,omitempty"`
	Title         string         `json:"title,omitempty"`
	TrashPath     string         `json:"trash_path,omitempty"`
	RegistryPath  string         `json:"registry_path,omitempty"`
	RegistryFacts map[string]any `json:"registry_facts,omitempty"`
	DeletedAt     string         `json:"deleted_at"`
	RestoredAt    string         `json:"restored_at,omitempty"`
	Source        string         `json:"source,omitempty"`
	Evidence      []string       `json:"evidence,omitempty"`
	ExpiresAt     string         `json:"expires_at,omitempty"`
}

type RecordIssue struct {
	Code     string `json:"code"`
	NoteID   string `json:"note_id,omitempty"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type RecordRepairOperation struct {
	OperationID string `json:"operation_id"`
	Kind        string `json:"kind"`
	NoteID      string `json:"note_id,omitempty"`
	Path        string `json:"path,omitempty"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
}

type IndexSnapshot struct {
	SchemaVersion string `json:"schema_version"`
	SnapshotID    string `json:"snapshot_id"`
	LedgerSeq     uint64 `json:"ledger_seq"`
	IndexEpoch    uint64 `json:"index_epoch"`
	CreatedAt     string `json:"created_at"`
}

type LedgerVersion struct {
	SchemaVersion string `json:"schema_version"`
	LastSeq       uint64 `json:"last_seq"`
	UpdatedAt     string `json:"updated_at"`
}

type LedgerState struct {
	SchemaVersion string                `json:"schema_version"`
	Records       map[string]NoteRecord `json:"records"`
	Tombstones    map[string]Tombstone  `json:"tombstones"`
	Version       LedgerVersion         `json:"version"`
	Issues        []RecordIssue         `json:"issues,omitempty"`
}

func ErrorCode(err error) string {
	var commandErr *CommandError
	if errors.As(err, &commandErr) {
		return commandErr.Code
	}
	return ""
}
