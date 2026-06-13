package remote

import (
	"context"
	"errors"
	"time"
)

const CreateIfAbsentRevision = "__pinax_create_if_absent__"

var (
	ErrObjectNotFound = errors.New("object not found")
	ErrConflict       = errors.New("revision conflict")
)

// BlobStore abstracts the underlying blind storage system (S3, File, etc.).
type BlobStore interface {
	// Get retrieves the object. If not found, returns ErrObjectNotFound.
	Get(ctx context.Context, key string) (data []byte, rev string, err error)

	// Put uploads the object. baseRev is the expected current revision.
	// If baseRev is CreateIfAbsentRevision, the object must not exist.
	// If baseRev is not empty and doesn't match, returns ErrConflict.
	// Returns the new revision string.
	Put(ctx context.Context, key string, data []byte, baseRev string) (newRev string, err error)

	// Stat retrieves the revision of the object. If not found, returns ErrObjectNotFound.
	Stat(ctx context.Context, key string) (rev string, err error)

	// Delete removes the object.
	Delete(ctx context.Context, key string) error
}

// ConditionalWriteCapability reports whether Put enforces baseRev preconditions durably.
type ConditionalWriteCapability interface {
	SupportsConditionalWrites() bool
}

// ObjectInfo describes a remote object.
type ObjectInfo struct {
	Key          string
	Size         int64
	Revision     string
	LastModified time.Time
}

// ExtendedBlobStore extends BlobStore with list and batch operations.
type ExtendedBlobStore interface {
	BlobStore
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
	Exists(ctx context.Context, key string) (bool, error)
	BatchStat(ctx context.Context, keys []string) (map[string]string, error)
}
