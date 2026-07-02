package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestAgentBrainContextBundleUsesBoundedRefsAndRedactsBodies(t *testing.T) {
	contexts := []domain.AgentContext{
		{
			SchemaVersion: domain.AgentContextSchemaVersion,
			ContextID:     "memory:alice",
			SourceKind:    "memory_context",
			DisplayTitle:  "Alice roadmap",
			Refs:          []domain.AgentContextRef{{Kind: "memory", ID: "mem-1", Title: "Alice roadmap"}},
			Snippets:      []domain.AgentContextSnippet{{Kind: "body", Text: "SECRET_BODY_SENTINEL", Source: "notes/alice.md"}},
			Evidence:      []string{"SECRET_EVIDENCE_SENTINEL"},
			BodyExposure:  "snippet",
			Actions:       []domain.Action{{Name: "memory_context", Command: "pinax memory context Alice --vault <vault> --agent"}},
		},
		{
			SchemaVersion: domain.AgentContextSchemaVersion,
			ContextID:     "kb:project",
			SourceKind:    "semantic_kb_hit",
			DisplayTitle:  "Project Atlas",
			Refs:          []domain.AgentContextRef{{Kind: "note", ID: "note-1", Path: "notes/atlas.md", Title: "Project Atlas"}},
			BodyExposure:  "snippet",
		},
		{
			SchemaVersion: domain.AgentContextSchemaVersion,
			ContextID:     "graph:alice",
			SourceKind:    "graph_entity",
			DisplayTitle:  "Alice",
			Refs:          []domain.AgentContextRef{{Kind: "note", ID: "note-2", Path: "notes/alice.md", Title: "Alice"}},
			BodyExposure:  "context",
		},
		{
			SchemaVersion: domain.AgentContextSchemaVersion,
			ContextID:     "query:active",
			SourceKind:    "query_row",
			DisplayTitle:  "Active task",
			Refs:          []domain.AgentContextRef{{Kind: "query_row", ID: "row-1", Title: "Active task"}},
			BodyExposure:  "none",
		},
	}

	bundle := BuildAgentBrainContextBundle(AgentBrainContextBundleRequest{
		Task:     "prepare Alice update",
		Contexts: contexts,
		Receipts: []domain.AgentBrainReceiptRef{{Kind: "proof_receipt", ID: "receipt-1", Status: "passed"}},
	})

	if bundle.SchemaVersion != domain.AgentBrainContextBundleSchemaVersion || bundle.Task != "prepare Alice update" {
		t.Fatalf("bundle identity = %#v", bundle)
	}
	if bundle.BodyExposure != "bounded_projection" {
		t.Fatalf("body exposure = %q", bundle.BodyExposure)
	}
	if len(bundle.MemoryRefs) != 1 || len(bundle.SemanticRefs) != 1 || len(bundle.GraphRefs) != 1 || len(bundle.QueryRefs) != 1 {
		t.Fatalf("classified refs = %#v", bundle)
	}
	if len(bundle.Receipts) != 1 || bundle.Receipts[0].ID != "receipt-1" {
		t.Fatalf("receipts = %#v", bundle.Receipts)
	}
	if len(bundle.NextActions) == 0 || bundle.NextActions[0].Name != "memory_context" {
		t.Fatalf("next actions = %#v", bundle.NextActions)
	}
	if !containsString(bundle.Entities, "Alice roadmap") || !containsString(bundle.Entities, "Project Atlas") {
		t.Fatalf("entities = %#v", bundle.Entities)
	}

	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	text := string(raw)
	for _, forbidden := range []string{"SECRET_BODY_SENTINEL", "SECRET_EVIDENCE_SENTINEL"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("context bundle leaked %s: %s", forbidden, text)
		}
	}
}
