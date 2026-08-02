package inferrum

import (
	"context"
	"math"
	"time"
)

// VectorStore ties together a Domain adapter, a SidecarClient, and a Provider.
// It implements the Store interface and is the primary entry point that
// consumer CLIs instantiate.
type VectorStore struct {
	Domain   Domain
	Sidecar  *SidecarClient
	StoreURI string
}

// NewVectorStore creates a Store backed by the sidecar for the given domain.
func NewVectorStore(domain Domain, sidecar *SidecarClient, storeURI string) *VectorStore {
	return &VectorStore{Domain: domain, Sidecar: sidecar, StoreURI: storeURI}
}

// Doctor checks sidecar + backend health for this domain's store.
func (s *VectorStore) Doctor(ctx context.Context) (Status, error) {
	return s.Sidecar.Doctor(ctx, s.StoreURI, s.Domain.Name(), s.Domain.TableName())
}

// Rebuild (re)creates the vector index from the given records. Each record's
// metadata is redacted by the domain adapter before being handed to the sidecar.
func (s *VectorStore) Rebuild(ctx context.Context, records []Record, opts RebuildOpts) (int, error) {
	redacted := make([]Record, 0, len(records))
	for _, rec := range records {
		copy := rec
		copy.Metadata = s.Domain.Redact(rec.Metadata)
		if copy.IndexedAt.IsZero() {
			copy.IndexedAt = time.Now().UTC()
		}
		redacted = append(redacted, copy)
	}
	return s.Sidecar.Rebuild(ctx, s.StoreURI, s.Domain.Name(), s.Domain.TableName(), redacted, opts)
}

// Search runs a vector search constrained to allowedIDs. The caller is
// responsible for computing allowedIDs via domain.ResolvePermission (see the
// retrieval pipeline for the canonical flow).
func (s *VectorStore) Search(ctx context.Context, queryVec []float64, allowedIDs []string, limit int) ([]Hit, int, error) {
	return s.Sidecar.Search(ctx, s.StoreURI, s.Domain.Name(), s.Domain.TableName(), queryVec, allowedIDs, limit)
}

// ResolvePermission delegates to the domain adapter to compute allowed IDs.
// This is a convenience method for callers that want to run permission
// resolution + search as separate steps.
func (s *VectorStore) ResolvePermission(ctx context.Context, records []Record, filter PermissionFilter) []string {
	return s.Domain.ResolvePermission(ctx, records, filter)
}

var _ Store = (*VectorStore)(nil)

// FakeStore is an in-memory Store implementation for tests and offline flows.
// It performs no sidecar I/O.
type FakeStore struct {
	Domain  Domain
	records []Record
}

// NewFakeStore creates an in-memory store for the given domain.
func NewFakeStore(domain Domain) *FakeStore {
	return &FakeStore{Domain: domain}
}

// Doctor always reports the fake backend as available.
func (s *FakeStore) Doctor(_ context.Context) (Status, error) {
	return Status{Backend: FakeBackend, Available: true, Dependency: "in-memory fake store"}, nil
}

// Rebuild stores records in memory (metadata redacted) and returns the count.
func (s *FakeStore) Rebuild(_ context.Context, records []Record, _ RebuildOpts) (int, error) {
	redacted := make([]Record, 0, len(records))
	for _, rec := range records {
		copy := rec
		copy.Metadata = s.Domain.Redact(rec.Metadata)
		if copy.IndexedAt.IsZero() {
			copy.IndexedAt = time.Now().UTC()
		}
		redacted = append(redacted, copy)
	}
	s.records = redacted
	return len(redacted), nil
}

// Search performs an in-memory cosine search constrained to allowedIDs.
func (s *FakeStore) Search(_ context.Context, queryVec []float64, allowedIDs []string, limit int) ([]Hit, int, error) {
	if limit <= 0 {
		limit = 10
	}
	allowed := make(map[string]bool, len(allowedIDs))
	for _, id := range allowedIDs {
		allowed[id] = true
	}
	var hits []Hit
	for _, rec := range s.records {
		if len(allowedIDs) > 0 && !allowed[rec.ID] {
			continue
		}
		score := cosineSim(queryVec, rec.Vector)
		hits = append(hits, Hit{ID: rec.ID, Score: score, Metadata: rec.Metadata})
	}
	// Sort by score descending (insertion sort, small N).
	for i := 1; i < len(hits); i++ {
		for j := i; j > 0 && hits[j].Score > hits[j-1].Score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, len(hits), nil
}

// Records returns the stored records (for test inspection).
func (s *FakeStore) Records() []Record { return s.records }

var _ Store = (*FakeStore)(nil)

// cosineSim computes the cosine similarity between two vectors.
func cosineSim(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, aa, bb float64
	for i := 0; i < n; i++ {
		dot += a[i] * b[i]
		aa += a[i] * a[i]
		bb += b[i] * b[i]
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / (math.Sqrt(aa) * math.Sqrt(bb))
}
