package agentmemory

import (
	"context"
	"testing"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func TestLegacyKindMap(t *testing.T) {
	tests := []struct {
		in   string
		want agentprotocol.MemoryKind
	}{
		{"fact", agentprotocol.MemoryKindFact},
		{"decision", agentprotocol.MemoryKindDecision},
		{"event", agentprotocol.MemoryKindEvent},
		{"task", agentprotocol.MemoryKindTask},
		{"unknown_type", agentprotocol.MemoryKindFact}, // forward-compat: unknown → fact
		{"", agentprotocol.MemoryKindFact},
	}
	for _, tt := range tests {
		got := LegacyKindMap(tt.in)
		if got != tt.want {
			t.Errorf("LegacyKindMap(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLegacyStateMap(t *testing.T) {
	tests := []struct {
		in   string
		want agentprotocol.LifecycleState
	}{
		{"confirmed", agentprotocol.LifecycleConfirmed},
		{"superseded", agentprotocol.LifecycleSuperseded},
		{"expired", agentprotocol.LifecycleExpired},
		{"rejected", agentprotocol.LifecycleRejected},
		{"draft", agentprotocol.LifecycleProposed},
		{"unknown", agentprotocol.LifecycleProposed}, // forward-compat
	}
	for _, tt := range tests {
		got := LegacyStateMap(tt.in)
		if got != tt.want {
			t.Errorf("LegacyStateMap(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMapLegacyRecord(t *testing.T) {
	rec := LegacyRecord{
		ID:           "mem_old_1",
		Type:         "decision",
		Status:       "confirmed",
		Subject:      "Use SQLite for local index",
		SourceURI:    "notes/architecture.md",
		SupersedesID: "mem_old_0",
	}
	m := MapLegacyRecord(rec, "my-workspace")

	if m.ID != "mem_old_1" {
		t.Errorf("ID = %q, want mem_old_1", m.ID)
	}
	if m.Kind != agentprotocol.MemoryKindDecision {
		t.Errorf("Kind = %q", m.Kind)
	}
	if m.State != agentprotocol.LifecycleConfirmed {
		t.Errorf("State = %q", m.State)
	}
	if m.CreatorID != DefaultLocalPrincipalID {
		t.Errorf("CreatorID = %q", m.CreatorID)
	}
	if m.Scope.Kind != agentprotocol.ScopeKindWorkspace || m.Scope.ID != "my-workspace" {
		t.Errorf("Scope = %+v", m.Scope)
	}
	if len(m.Sources) != 1 || m.Sources[0].Ref != "notes/architecture.md" {
		t.Errorf("Sources = %+v", m.Sources)
	}
	if m.SupersedesID != "mem_old_0" {
		t.Errorf("SupersedesID = %q", m.SupersedesID)
	}
}

func TestMapLegacyRecord_EmptyWorkspace(t *testing.T) {
	rec := LegacyRecord{ID: "x", Type: "fact", Status: "confirmed"}
	m := MapLegacyRecord(rec, "")
	if m.Scope.ID != "default" {
		t.Errorf("empty workspace should default to 'default', got %q", m.Scope.ID)
	}
}

func TestEnsureLegacyViewed_NoBackfill(t *testing.T) {
	s := testStore(t)
	defer func() { _ = s.Close() }()
	records := []LegacyRecord{
		{ID: "old_1", Type: "fact", Status: "confirmed", Subject: "s1"},
		{ID: "old_2", Type: "decision", Status: "superseded", Subject: "s2"},
	}
	result, err := EnsureLegacyViewed(context.Background(), s, records, "ws_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 mapped records, got %d", len(result))
	}
	// EnsureLegacyViewed 不写回 agent_memory_records 表
	rows, err := s.ListMemories(context.Background(), agentprotocol.Scope{Kind: agentprotocol.ScopeKindWorkspace, ID: "ws_1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("EnsureLegacyViewed should NOT backfill; got %d rows in store", len(rows))
	}
}
