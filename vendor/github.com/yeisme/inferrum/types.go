package inferrum

import "time"

// Public types for the Inferrum vector + RAG platform. These match the contract
// section of docs/architecture/inferrum-vector-rag-platform.md.

// Record is a domain-agnostic vector record envelope. Metadata is opaque to the
// sidecar; the caller (which knows the domain) populates permission, collection,
// tags, note_id, etc. The sidecar stores and returns it verbatim.
type Record struct {
	ID             string         `json:"id"`
	Vector         []float64      `json:"vector"`
	EmbeddingModel string         `json:"embedding_model"`
	EmbeddingDim   int            `json:"embedding_dim"`
	Metadata       map[string]any `json:"metadata"`
	IndexedAt      time.Time      `json:"indexed_at"`
}

// Hit is a single search result returned by the sidecar / store.
type Hit struct {
	ID       string         `json:"id"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata"`
}

// Status reports backend/sidecar health for doctor output.
type Status struct {
	Backend    string `json:"backend"`
	Available  bool   `json:"available"`
	Dependency string `json:"dependency,omitempty"`
}

// DoctorStatus carries additive readiness details without changing the stable
// Status struct layout used by existing unkeyed literals.
type DoctorStatus struct { Status; StoreState string `json:"store_state,omitempty"` }

// RebuildOpts controls a store rebuild.
type RebuildOpts struct {
	DistanceMetric string `json:"distance_metric,omitempty"`
	EmbeddingDim   int    `json:"embedding_dim,omitempty"`
}

// SearchOpts controls a store search.
type SearchOpts struct {
	DistanceMetric string `json:"distance_metric,omitempty"`
	Limit          int    `json:"limit"`
}

// RetrievalRequest is the input to RetrievalPipeline.Retrieve.
type RetrievalRequest struct {
	Query            string         // user query text
	Provider         Provider       // embedding provider for the query
	AllowedIDs       []string       // optional pre-computed allowed IDs; if nil, domain resolves permission
	PermissionFilter map[string]any // opaque filter handed to domain.ResolvePermission
	Limit            int            // max hits to return
	Rerank           string         // "off" | "auto" | "required"
	MaxContextTokens int            // bound on assembled context (token budget)
}

// RetrievalResult is the output of the retrieval pipeline.
type RetrievalResult struct {
	Hits     []Hit
	Context  string
	Manifest RetrievalManifest
}

// RetrievalManifest is the evidence trail for a retrieval run. It is evidence,
// not a library copy — it must never contain image bytes, raw prompts, provider
// payloads, secrets, or chain-of-thought (per the eikona retrieval_manifest.json
// contract).
type RetrievalManifest struct {
	Query            string             `json:"query"`
	Provider         string             `json:"provider"`
	Model            string             `json:"model"`
	PermissionFilter map[string]any     `json:"permission_filter,omitempty"`
	AllowedIDs       []string           `json:"allowed_ids,omitempty"`
	HitCount         int                `json:"hit_count"`
	SkippedIDs       []string           `json:"skipped_ids,omitempty"`
	ScoreSummary     map[string]float64 `json:"score_summary,omitempty"`
	Stages           []string           `json:"stages"`
	ContextTokens    int                `json:"context_tokens"`
}
