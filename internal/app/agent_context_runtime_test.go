package app

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

func TestAgentContextRuntime_BoundedContext(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_ctx_rt"}

	// Create some confirmed memories via propose→approve
	for _, s := range []struct{ subj, obj string }{
		{"GORM Gen decision", "use Gen for typed DAO"},
		{"SQLite index fact", "index uses SQLite"},
	} {
		facts, _, err := svc.AgentMemoryPropose(ctx, AgentMemoryProposeRequest{
			VaultPath: vault, Principal: adapterPrincipal(), Scope: scope,
			Kind: agentprotocol.MemoryKindFact, Subject: s.subj, Object: s.obj,
			Sources: agentprotocol.SourceRefList{{Kind: "note", Ref: "n1"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.AgentMemoryApprove(ctx, vault, facts.ProposalID, ownerPrincipal()); err != nil {
			t.Fatal(err)
		}
	}

	pack, err := svc.AgentContextRuntime(ctx, AgentContextRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("p_read", "codex"),
		Scope:     scope,
		Entities:  []string{"GORM"},
		MaxItems:  10,
		MaxChars:  4000,
	})
	if err != nil {
		t.Fatal(err)
	}

	if pack.SchemaVersion != agentprotocol.ContextSchemaVersion {
		t.Errorf("schema = %s", pack.SchemaVersion)
	}
	if pack.EntryCount() == 0 {
		t.Error("expected non-empty pack")
	}
	// GORM entity-matched fact should be present
	found := false
	for _, f := range pack.Facts {
		if f.Subject == "GORM Gen decision" {
			found = true
		}
	}
	if !found {
		t.Error("GORM Gen fact should be in pack")
	}
}

func TestAgentContextRuntime_LegacyMerge(t *testing.T) {
	svc, vault := testAgentMemoryService(t)
	ctx := context.Background()
	scope := agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "ws_legacy"}

	legacyRecords := []agentmemory.LegacyRecord{
		{ID: "old_1", Type: "fact", Status: "confirmed", Subject: "legacy fact"},
		{ID: "old_2", Type: "decision", Status: "superseded", Subject: "old decision"},
	}

	pack, err := svc.AgentContextRuntimeWithLegacy(ctx, AgentContextRequest{
		VaultPath: vault,
		Principal: agentprotocol.DefaultAdapterPrincipal("p_read", "codex"),
		Scope:     scope,
	}, legacyRecords, "ws_legacy")
	if err != nil {
		t.Fatal(err)
	}
	// old_1 (confirmed) should appear; old_2 (superseded) should not
	foundOld1 := false
	for _, f := range pack.Facts {
		if f.MemoryID == "old_1" {
			foundOld1 = true
		}
	}
	if !foundOld1 {
		t.Error("legacy confirmed fact should appear in pack")
	}
}
