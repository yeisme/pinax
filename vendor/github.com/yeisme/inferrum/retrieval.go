package inferrum

import (
	"context"
	"sort"
	"strings"
)

// RetrievalPipeline orchestrates the 7-stage RAG retrieval flow:
//  1. Embed query — shared Provider (gemini/openai/ollama/fake)
//  2. Resolve permission — Domain adapter computes allowed-IDs
//  3. Search backend — shared sidecar returns raw hits
//  4. Hydrate metadata — Domain adapter fills in display fields
//  5. Rerank — optional (off/auto/required), currently identity passthrough
//  6. Assemble context — bounded preview, prompt-ready context string
//  7. Emit manifest — evidence trail (query, filter, scores, skipped IDs)
//
// The manifest is evidence, not a library copy. It must never contain image
// bytes, raw prompts, provider payloads, secrets, or chain-of-thought.
type RetrievalPipeline struct {
	Store  Store
	Domain Domain
}

// NewRetrievalPipeline creates a retrieval pipeline over a store and domain.
func NewRetrievalPipeline(store Store, domain Domain) *RetrievalPipeline {
	return &RetrievalPipeline{Store: store, Domain: domain}
}

// Retrieve runs the full RAG pipeline and returns hits, assembled context, and
// a retrieval manifest.
func (p *RetrievalPipeline) Retrieve(ctx context.Context, req RetrievalRequest) (RetrievalResult, error) {
	stages := make([]string, 0, 7)
	manifest := RetrievalManifest{
		Query:            req.Query,
		PermissionFilter: req.PermissionFilter,
		ScoreSummary:     map[string]float64{},
	}

	// Stage 1: Embed query.
	if req.Provider == nil {
		return RetrievalResult{}, &CommandError{Code: "retrieval_provider_missing", Message: "retrieval request has no embedding provider", Hint: "Set RetrievalRequest.Provider"}
	}
	queryVec, err := req.Provider.Embed(ctx, req.Query)
	if err != nil {
		return RetrievalResult{}, err
	}
	stages = append(stages, "embed_query")
	manifest.Provider = req.Provider.Name()
	manifest.Model = req.Provider.Model()

	// Stage 2: Resolve permission.
	allowedIDs := req.AllowedIDs
	if allowedIDs == nil {
		// In a full pipeline the candidate records come from the domain's source
		// of truth. Here the pipeline delegates entirely to the domain adapter,
		// passing an empty candidate set — the adapter is expected to resolve
		// allowed IDs from its own store using the filter. If the adapter needs
		// candidate records, the caller should pre-resolve and pass AllowedIDs.
		allowedIDs = p.Domain.ResolvePermission(ctx, nil, req.PermissionFilter)
	}
	stages = append(stages, "resolve_permission")
	manifest.AllowedIDs = allowedIDs

	// Stage 3: Search backend.
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	hits, total, err := p.Store.Search(ctx, queryVec, allowedIDs, limit)
	if err != nil {
		return RetrievalResult{}, err
	}
	stages = append(stages, "search_backend")
	manifest.HitCount = total

	// Stage 4: Hydrate metadata — domain redacts/fills display fields.
	for i := range hits {
		hits[i].Metadata = p.Domain.Redact(hits[i].Metadata)
	}
	stages = append(stages, "hydrate_metadata")

	// Stage 5: Rerank — currently identity passthrough (placeholder for future
	// cross-encoder reranking). off/auto/required are accepted but treated
	// uniformly until a reranker backend is wired.
	rerank := strings.ToLower(strings.TrimSpace(req.Rerank))
	if rerank == "" {
		rerank = "auto"
	}
	stages = append(stages, "rerank:"+rerank)

	// Stage 6: Assemble context — bounded preview per hit, joined into a
	// prompt-ready string.
	maxTokens := req.MaxContextTokens
	if maxTokens <= 0 {
		maxTokens = 512
	}
	var contextParts []string
	tokensUsed := 0
	for _, hit := range hits {
		preview := boundedPreview(hitText(hit))
		tokens := tokenCount(preview)
		if tokensUsed+tokens > maxTokens {
			break
		}
		tokensUsed += tokens
		contextParts = append(contextParts, preview)
		manifest.ScoreSummary[hit.ID] = hit.Score
	}
	stages = append(stages, "assemble_context")

	// Stage 7: Emit manifest.
	manifest.Stages = stages
	manifest.ContextTokens = tokensUsed

	// Track skipped IDs (allowed but not returned — permission=unknown items
	// that were filtered out by the backend).
	if len(allowedIDs) > 0 {
		returned := make(map[string]bool, len(hits))
		for _, hit := range hits {
			returned[hit.ID] = true
		}
		for _, id := range allowedIDs {
			if !returned[id] {
				manifest.SkippedIDs = append(manifest.SkippedIDs, id)
			}
		}
	}

	context := strings.Join(contextParts, "\n\n")
	return RetrievalResult{Hits: hits, Context: context, Manifest: manifest}, nil
}

// hitText extracts a text representation of a hit for context assembly. It pulls
// common text-like metadata fields; the domain adapter is responsible for
// ensuring these are safe (redacted) by this stage.
func hitText(hit Hit) string {
	var parts []string
	for _, key := range []string{"title", "heading", "caption", "preview", "text", "body", "summary"} {
		if v, ok := hit.Metadata[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, s)
			}
		}
	}
	if len(parts) == 0 {
		return hit.ID
	}
	return strings.Join(parts, " — ")
}

// boundedPreview trims text to at most 24 words.
func boundedPreview(text string) string {
	fields := strings.Fields(text)
	if len(fields) > 24 {
		fields = fields[:24]
	}
	return strings.Join(fields, " ")
}

// tokenCount returns a word-count approximation of token count.
func tokenCount(text string) int { return len(strings.Fields(text)) }

// sortHitsByScore sorts hits by score descending (stable).
func sortHitsByScore(hits []Hit) {
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
}
