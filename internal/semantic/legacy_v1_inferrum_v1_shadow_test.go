package semantic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

// This component comparison keeps the legacy reader honest during the
// Inferrum v1 cutover. It compares bounded citation identity, not score bytes:
// the historical reader has a local exact-text bonus while Inferrum stores
// only safe metadata.
func TestLegacyV1InferrumV1ShadowCompareDeterministicCorpus(t *testing.T) {
	notes := []domain.Note{
		{ID: "note-alpha", Path: "notes/alpha.md", Title: "Alpha", Body: "# Alpha\n\nInferrum local retrieval uses a bounded citation."},
		{ID: "note-beta", Path: "notes/beta.md", Title: "Beta", Body: "# Beta\n\nCandidate evaluation precedes activation."},
	}
	provider := FakeProvider{ModelName: FakeProviderModel}
	chunks, err := BuildChunks(context.Background(), notes, provider, DefaultBackend)
	if err != nil {
		t.Fatalf("build deterministic corpus: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(chunks))
	}

	legacyRoot := t.TempDir()
	if err := NewFileStore(legacyRoot, DefaultBackend).Save(chunks); err != nil {
		t.Fatalf("write v1 rows: %v", err)
	}
	legacyMetadata := map[string]any{
		"schema_version": LegacySidecarSchema,
		"backend":        DefaultBackend,
		"provider":       provider.Name(),
		"model":          provider.Model(),
		"embedding_dim":  len(chunks[0].Vector),
		"indexed_at":     "2026-08-02T00:00:00Z",
	}
	metadata, err := json.Marshal(legacyMetadata)
	if err != nil {
		t.Fatalf("marshal v1 metadata: %v", err)
	}
	legacyStore := filepath.Join(legacyRoot, ".pinax", "kb", DefaultBackend)
	if err := os.WriteFile(filepath.Join(legacyStore, "metadata.json"), append(metadata, '\n'), 0o600); err != nil {
		t.Fatalf("write v1 metadata: %v", err)
	}

	inferrumRoot := t.TempDir()
	sidecar := writeLegacyInferrumShadowSidecar(t)
	storeURI := filepath.Join(inferrumRoot, "vector-store")
	if _, err := Save(context.Background(), inferrumRoot, chunks, DefaultBackend, SidecarConfig{Executable: sidecar, StoreURI: storeURI}, len(notes)); err != nil {
		t.Fatalf("write Inferrum v1 projection: %v", err)
	}

	allowedIDs := ChunkIDsForNotes(notes)
	legacyHits, legacyTotal, err := SearchLegacyV1(context.Background(), legacyRoot, "Inferrum local retrieval", provider, 5, allowedIDs)
	if err != nil {
		t.Fatalf("search v1 projection: %v", err)
	}
	inferrumHits, inferrumTotal, err := SearchWithAllowedIDs(context.Background(), inferrumRoot, "Inferrum local retrieval", provider, DefaultBackend, 5, allowedIDs, SidecarConfig{Executable: sidecar, StoreURI: storeURI})
	if err != nil {
		t.Fatalf("search Inferrum v1 projection: %v", err)
	}
	if legacyTotal != len(chunks) || inferrumTotal != len(chunks) || len(legacyHits) != len(inferrumHits) {
		t.Fatalf("document/chunk counts diverged: legacy total=%d hits=%d, Inferrum total=%d hits=%d", legacyTotal, len(legacyHits), inferrumTotal, len(inferrumHits))
	}
	if got, want := citationSet(inferrumHits), citationSet(legacyHits); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("top-k citation set diverged: legacy=%v Inferrum=%v", want, got)
	}
	for _, hit := range inferrumHits {
		if hit.Path == "" || filepath.IsAbs(hit.Path) || strings.Contains(hit.Path, "..") || strings.TrimSpace(hit.Preview) == "" {
			t.Fatalf("Inferrum citation is unsafe: %#v", hit)
		}
	}
	requestPayload, err := os.ReadFile(filepath.Join(inferrumRoot, "shadow-metadata.json"))
	if err != nil {
		t.Fatalf("read Inferrum metadata evidence: %v", err)
	}
	requestText := string(requestPayload)
	// The fake sidecar persists metadata only, so the assertion covers the
	// bounded record surface without retaining vectors or the private store URI.
	for _, forbidden := range []string{"chunk_text", "vault_path", "raw_prompt", "provider_payload", "SECRET"} {
		if strings.Contains(requestText, forbidden) {
			t.Fatalf("Inferrum request leaked %q: %s", forbidden, requestText)
		}
	}
}

func citationSet(hits []SearchHit) []string {
	items := make([]string, 0, len(hits))
	for _, hit := range hits {
		items = append(items, hit.Path+"#"+hit.HeadingPath)
	}
	sort.Strings(items)
	return items
}

func writeLegacyInferrumShadowSidecar(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inferrum-lancedb-sidecar")
	body := `#!/usr/bin/env python3
import json, math, pathlib, sys

op = sys.argv[1]
req = json.load(sys.stdin)
store = pathlib.Path(req["store_uri"])
store.mkdir(parents=True, exist_ok=True)

def cosine(left, right):
    dot = sum(a * b for a, b in zip(left, right))
    left_norm = math.sqrt(sum(a * a for a in left))
    right_norm = math.sqrt(sum(b * b for b in right))
    if left_norm == 0 or right_norm == 0:
        return 0.0
    return dot / (left_norm * right_norm)

if op == "doctor":
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","dependency":"fake-shadow"}))
elif op == "rebuild":
    (store / "records.json").write_text(json.dumps(req.get("records", [])), encoding="utf-8")
    (store.parent / "shadow-metadata.json").write_text(json.dumps([row.get("metadata") or {} for row in req.get("records", [])]), encoding="utf-8")
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","rows":len(req.get("records", []))}))
elif op == "search":
    rows = json.loads((store / "records.json").read_text(encoding="utf-8"))
    allowed = set(req.get("allowed_ids") or [])
    query = req.get("query_vector") or []
    rows = [row for row in rows if not allowed or row.get("id") in allowed]
    rows.sort(key=lambda row: (-cosine(query, row.get("vector") or []), str((row.get("metadata") or {}).get("source_ref") or "")))
    limit = int(req.get("limit") or 20)
    hits = [{"id": row["id"], "score": cosine(query, row.get("vector") or []), "metadata": row.get("metadata") or {}} for row in rows[:limit]]
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"success","backend":"lancedb","total":len(hits),"hits":hits}))
else:
    print(json.dumps({"schema_version":"inferrum.sidecar.v1","status":"failed","error":{"code":"operation_invalid","message":"unknown operation"}}))
    sys.exit(2)
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write shadow sidecar: %v", err)
	}
	return path
}
