package agentcontext

import (
	"context"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func TestCompilePersonalAssistantSourcesFollowIncludedBudget(t *testing.T) {
	t.Parallel()
	compiler, store := testCompiler(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "personal-assistant"}
	now := time.Now().UTC()

	for _, memory := range []agentprotocol.MemoryRecord{
		{
			SchemaVersion: agentprotocol.SchemaVersion,
			ID:            "high-priority",
			Kind:          agentprotocol.MemoryKindDecision,
			Scope:         scope,
			State:         agentprotocol.LifecycleConfirmed,
			Subject:       "Selected decision",
			Confidence:    agentprotocol.ConfidenceVerified,
			Sources: agentprotocol.SourceRefList{
				{Kind: agentprotocol.SourceKindNote, Ref: "note-selected"},
				{Kind: agentprotocol.SourceKindNote, Ref: "note-shared"},
			},
			CreatorID: "owner",
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			SchemaVersion: agentprotocol.SchemaVersion,
			ID:            "lower-priority",
			Kind:          agentprotocol.MemoryKindFact,
			Scope:         scope,
			State:         agentprotocol.LifecycleConfirmed,
			Subject:       "Truncated fact",
			Confidence:    agentprotocol.ConfidenceLow,
			Sources: agentprotocol.SourceRefList{
				{Kind: agentprotocol.SourceKindNote, Ref: "note-truncated"},
				{Kind: agentprotocol.SourceKindNote, Ref: "note-shared"},
			},
			CreatorID: "owner",
			CreatedAt: now,
			UpdatedAt: now,
		},
	} {
		if err := store.SaveMemory(ctx, memory); err != nil {
			t.Fatal(err)
		}
	}

	pack, err := compiler.Compile(ctx, agentprotocol.ContextRequest{
		SchemaVersion: agentprotocol.ContextSchemaVersion,
		Principal:     agentprotocol.DefaultAdapterPrincipal("personal-assistant", "test"),
		Scope:         scope,
		Budget:        agentprotocol.ContextBudget{MaxItems: 1, MaxChars: 2000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pack.Truncated || pack.EntryCount() != 1 {
		t.Fatalf("pack budget = entries:%d truncated:%t", pack.EntryCount(), pack.Truncated)
	}
	if len(pack.Sources) != 2 {
		t.Fatalf("sources = %#v, want selected and shared only", pack.Sources)
	}
	got := map[string]bool{}
	for _, source := range pack.Sources {
		got[source.Ref] = true
	}
	if !got["note-selected"] || !got["note-shared"] || got["note-truncated"] {
		t.Fatalf("budgeted sources = %#v", pack.Sources)
	}
}

func TestCompilePersonalAssistantSourcesDeduplicateByKindAndRef(t *testing.T) {
	t.Parallel()
	compiler, store := testCompiler(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "personal-assistant-dedupe"}
	now := time.Now().UTC()
	for _, id := range []string{"memory-a", "memory-b"} {
		if err := store.SaveMemory(ctx, agentprotocol.MemoryRecord{
			SchemaVersion: agentprotocol.SchemaVersion,
			ID:            id,
			Kind:          agentprotocol.MemoryKindFact,
			Scope:         scope,
			State:         agentprotocol.LifecycleConfirmed,
			Subject:       id,
			Confidence:    agentprotocol.ConfidenceHigh,
			Sources:       agentprotocol.SourceRefList{{Kind: agentprotocol.SourceKindNote, Ref: "note-shared"}},
			CreatorID:     "owner",
			CreatedAt:     now,
			UpdatedAt:     now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	pack, err := compiler.Compile(ctx, agentprotocol.ContextRequest{
		SchemaVersion: agentprotocol.ContextSchemaVersion,
		Principal:     agentprotocol.DefaultAdapterPrincipal("personal-assistant", "test"),
		Scope:         scope,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Sources) != 1 || pack.Sources[0].Ref != "note-shared" {
		t.Fatalf("deduplicated sources = %#v", pack.Sources)
	}
}
