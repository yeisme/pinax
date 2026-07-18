package agentprotocol

import (
	"strings"
	"testing"
	"time"
)

func TestScope_Validate(t *testing.T) {
	tests := []struct {
		name    string
		scope   Scope
		wantErr string
	}{
		{"valid owner", Scope{Kind: ScopeKindOwner, ID: "user_1"}, ""},
		{"valid task", Scope{Kind: ScopeKindTask, ID: "task_abc"}, ""},
		{"invalid kind", Scope{Kind: "galaxy", ID: "x"}, ErrCodeInvalidScope},
		{"empty id", Scope{Kind: ScopeKindProject, ID: ""}, ErrCodeInvalidScope},
		{"whitespace id", Scope{Kind: ScopeKindProject, ID: "  "}, ErrCodeInvalidScope},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.scope.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error code %s, got nil", tt.wantErr)
			}
			se, ok := err.(*StableError)
			if !ok {
				t.Fatalf("expected *StableError, got %T", err)
			}
			if se.Code != tt.wantErr {
				t.Fatalf("error code = %q, want %q", se.Code, tt.wantErr)
			}
		})
	}
}

func TestScope_Contains(t *testing.T) {
	owner := Scope{Kind: ScopeKindOwner, ID: "user_1"}
	ws := Scope{Kind: ScopeKindWorkspace, ID: "ws_1"}
	proj := Scope{Kind: ScopeKindProject, ID: "proj_1"}
	task := Scope{Kind: ScopeKindTask, ID: "task_1"}

	if !owner.Contains(ws) {
		t.Error("owner should contain workspace")
	}
	if !owner.Contains(proj) {
		t.Error("owner should contain project")
	}
	if !ws.Contains(task) {
		t.Error("workspace should contain task")
	}
	if task.Contains(owner) {
		t.Error("task should NOT contain owner")
	}
	if !owner.Contains(Scope{Kind: ScopeKindOwner, ID: "user_1"}) {
		t.Error("owner should contain same owner")
	}
	if owner.Contains(Scope{Kind: ScopeKindOwner, ID: "user_2"}) {
		t.Error("owner should NOT contain different owner")
	}
}

func TestPrincipal_Validate(t *testing.T) {
	valid := Principal{
		SchemaVersion: SchemaVersion,
		PrincipalID:   "p_1",
		Trust:         TrustLevelAdapter,
		Capabilities:  []Capability{CapabilityRead, CapabilityPropose},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid principal: %v", err)
	}

	if err := (Principal{}).Validate(); err == nil {
		t.Error("empty principal should fail")
	}

	badTrust := valid
	badTrust.Trust = "superuser"
	if err := badTrust.Validate(); err == nil {
		t.Error("invalid trust should fail")
	}

	badCap := valid
	badCap.Capabilities = []Capability{"fly"}
	if err := badCap.Validate(); err == nil {
		t.Error("invalid capability should fail")
	}
}

func TestPrincipal_CanConfirm(t *testing.T) {
	adapter := DefaultAdapterPrincipal("p_adapter", "codex")
	if adapter.CanConfirm() {
		t.Error("adapter principal should not confirm by default")
	}
	owner := Principal{PrincipalID: "p_owner", Trust: TrustLevelOwner}
	if !owner.CanConfirm() {
		t.Error("owner principal should confirm")
	}
	withCap := Principal{PrincipalID: "p_explicit", Trust: TrustLevelAdapter, Capabilities: []Capability{CapabilityConfirm}}
	if !withCap.CanConfirm() {
		t.Error("explicit confirm capability should allow confirm")
	}
}

func TestMemoryRecord_Validate(t *testing.T) {
	valid := MemoryRecord{
		SchemaVersion: SchemaVersion,
		ID:            "mem_1",
		Kind:          MemoryKindFact,
		Scope:         Scope{Kind: ScopeKindProject, ID: "proj_1"},
		State:         LifecycleConfirmed,
		CreatorID:     "p_1",
		Sources:       SourceRefList{{Kind: "note", Ref: "note_abc"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid record: %v", err)
	}

	noID := valid
	noID.ID = ""
	if err := noID.Validate(); err == nil {
		t.Error("missing id should fail")
	}

	badKind := valid
	badKind.Kind = "rumor"
	if err := badKind.Validate(); err == nil {
		t.Error("invalid kind should fail")
	}

	badState := valid
	badState.State = "frozen"
	if err := badState.Validate(); err == nil {
		t.Error("invalid state should fail")
	}

	noCreator := valid
	noCreator.CreatorID = ""
	if err := noCreator.Validate(); err == nil {
		t.Error("missing creator should fail")
	}

	badSource := valid
	badSource.Sources = SourceRefList{{Kind: "", Ref: "x"}}
	if err := badSource.Validate(); err == nil {
		t.Error("invalid source should fail")
	}
}

func TestMemoryRecord_IsRecallable(t *testing.T) {
	if !(MemoryRecord{State: LifecycleConfirmed}).IsRecallable() {
		t.Error("confirmed should be recallable")
	}
	if !(MemoryRecord{State: LifecycleConflicted}).IsRecallable() {
		t.Error("conflicted should be recallable (visible, not hidden)")
	}
	if (MemoryRecord{State: LifecycleProposed}).IsRecallable() {
		t.Error("proposed should not be recallable")
	}
	if (MemoryRecord{State: LifecycleRejected}).IsRecallable() {
		t.Error("rejected should not be recallable")
	}
}

func TestMemoryRecord_IsExpired(t *testing.T) {
	now := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	if !(MemoryRecord{ExpiresAt: &past}).IsExpired(now) {
		t.Error("past expiry should be expired")
	}
	if (MemoryRecord{ExpiresAt: &future}).IsExpired(now) {
		t.Error("future expiry should not be expired")
	}
	if (MemoryRecord{ExpiresAt: nil}).IsExpired(now) {
		t.Error("nil expiry should not be expired")
	}
}

func TestValidTransition(t *testing.T) {
	tests := []struct {
		from, to LifecycleState
		want     bool
	}{
		{LifecycleProposed, LifecycleConfirmed, true},
		{LifecycleProposed, LifecycleRejected, true},
		{LifecycleProposed, LifecycleExpired, false},
		{LifecycleConfirmed, LifecycleSuperseded, true},
		{LifecycleConfirmed, LifecycleConflicted, true},
		{LifecycleConfirmed, LifecycleRejected, false},
		{LifecycleConflicted, LifecycleConfirmed, true},
		{LifecycleRejected, LifecycleConfirmed, false},
		{LifecycleSuperseded, LifecycleConfirmed, true},
		{LifecycleExpired, LifecycleConfirmed, true},
	}
	for _, tt := range tests {
		got := ValidTransition(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("ValidTransition(%s→%s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestContextRequest_Validate(t *testing.T) {
	valid := ContextRequest{
		SchemaVersion: ContextSchemaVersion,
		Principal:     DefaultAdapterPrincipal("p_1", "codex"),
		Scope:         Scope{Kind: ScopeKindProject, ID: "proj_1"},
		KindFilter:    []MemoryKind{MemoryKindFact, MemoryKindDecision},
		Budget:        ContextBudget{MaxItems: 10, MaxChars: 4000},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request: %v", err)
	}

	badFilter := valid
	badFilter.KindFilter = []MemoryKind{"rumor"}
	if err := badFilter.Validate(); err == nil {
		t.Error("invalid kind filter should fail")
	}

	negBudget := valid
	negBudget.Budget = ContextBudget{MaxItems: -1}
	if err := negBudget.Validate(); err == nil {
		t.Error("negative budget should fail")
	}
}

func TestContextPack_AssertNoBody(t *testing.T) {
	pack := ContextPack{
		Facts: []ContextEntry{
			{MemoryID: "m1", Kind: MemoryKindFact, Preview: "short preview"},
		},
	}
	if err := pack.AssertNoBody(100); err != nil {
		t.Fatalf("short preview should pass: %v", err)
	}

	longPack := ContextPack{
		Facts: []ContextEntry{
			{MemoryID: "m1", Kind: MemoryKindFact, Preview: strings.Repeat("x", 200)},
		},
	}
	if err := longPack.AssertNoBody(100); err == nil {
		t.Error("preview exceeding max should fail")
	}

	bodyReason := ContextPack{
		Facts: []ContextEntry{
			{MemoryID: "m1", Kind: MemoryKindFact, ScoreReason: "matched raw body content"},
		},
	}
	if err := bodyReason.AssertNoBody(1000); err == nil {
		t.Error("score_reason referencing body should fail")
	}
}

func TestProposal_Validate(t *testing.T) {
	valid := Proposal{
		SchemaVersion:  SchemaVersion,
		ProposalID:     "prop_1",
		Principal:      DefaultAdapterPrincipal("p_1", "codex"),
		Scope:          Scope{Kind: ScopeKindProject, ID: "proj_1"},
		Kind:           MemoryKindDecision,
		RequestedState: LifecycleConfirmed,
		Sources:        SourceRefList{{Kind: "note", Ref: "note_1"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid proposal: %v", err)
	}

	badKind := valid
	badKind.Kind = "guess"
	if err := badKind.Validate(); err == nil {
		t.Error("invalid kind should fail")
	}
}

func TestHandoff_Validate(t *testing.T) {
	valid := Handoff{
		SchemaVersion: HandoffSchemaVersion,
		HandoffID:     "h_1",
		FromPrincipal: DefaultAdapterPrincipal("p_cohors", "cohors"),
		ToPrincipal:   DefaultAdapterPrincipal("p_codex", "codex"),
		Scope:         Scope{Kind: ScopeKindProject, ID: "proj_1"},
		Objective:     "Review implementation slice",
		Decisions:     []string{"chose GORM Gen for typed DAO"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid handoff: %v", err)
	}

	noObjective := valid
	noObjective.Objective = ""
	if err := noObjective.Validate(); err == nil {
		t.Error("missing objective should fail")
	}
}

func TestFeedback_Validate(t *testing.T) {
	valid := Feedback{
		SchemaVersion: SchemaVersion,
		FeedbackID:    "fb_1",
		Principal:     DefaultAdapterPrincipal("p_1", "codex"),
		Scope:         Scope{Kind: ScopeKindProject, ID: "proj_1"},
		Kind:          FeedbackUseful,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid feedback: %v", err)
	}

	badKind := valid
	badKind.Kind = "amazing"
	if err := badKind.Validate(); err == nil {
		t.Error("invalid feedback kind should fail")
	}
}

func TestAdapterDescriptor_Validate(t *testing.T) {
	valid := AdapterDescriptor{
		SchemaVersion:     AdapterSchemaVersion,
		AdapterID:         "codex-ref",
		Runtime:           "codex",
		SupportedVersions: []string{SchemaVersion},
		Capabilities:      []Capability{CapabilityRead, CapabilityPropose},
		SupportsProposal:  true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid descriptor: %v", err)
	}

	noVersions := valid
	noVersions.SupportedVersions = nil
	if err := noVersions.Validate(); err == nil {
		t.Error("missing versions should fail")
	}
}

func TestNegotiate(t *testing.T) {
	desc := AdapterDescriptor{
		AdapterID:         "codex-ref",
		Runtime:           "codex",
		SupportedVersions: []string{SchemaVersion},
		Capabilities:      []Capability{CapabilityRead, CapabilityPropose, CapabilityFeedback},
	}

	// Full compatibility
	res := Negotiate(desc, []string{SchemaVersion}, []Capability{CapabilityRead, CapabilityPropose})
	if !res.Compatible || res.Degraded {
		t.Errorf("expected full compatibility, got %+v", res)
	}
	if res.NegotiatedVersion != SchemaVersion {
		t.Errorf("negotiated version = %q, want %q", res.NegotiatedVersion, SchemaVersion)
	}

	// Version mismatch
	res = Negotiate(desc, []string{"yeisme.agent_memory.v999"}, []Capability{CapabilityRead})
	if res.Compatible {
		t.Error("version mismatch should not be compatible")
	}

	// Degraded (subset of capabilities)
	res = Negotiate(desc, []string{SchemaVersion}, []Capability{CapabilityRead, CapabilityConfirm})
	if res.Compatible {
		t.Error("missing capability should not be fully compatible")
	}
	if !res.Degraded {
		t.Error("partial capability match should be degraded")
	}
}

func TestStableError(t *testing.T) {
	e := NewStableError(ErrCodeInvalidScope, "bad scope").WithDetail("field", "kind")
	if e.Code != ErrCodeInvalidScope {
		t.Errorf("code = %q", e.Code)
	}
	if e.Details["field"] != "kind" {
		t.Errorf("detail not set: %+v", e.Details)
	}
	if !strings.Contains(e.Error(), ErrCodeInvalidScope) {
		t.Errorf("Error() should contain code: %s", e.Error())
	}
}

func TestSourceRefList(t *testing.T) {
	empty := SourceRefList{}
	if !empty.IsEmpty() {
		t.Error("empty list should be empty")
	}
	if err := empty.Validate(); err != nil {
		t.Errorf("empty list should validate: %v", err)
	}

	list := SourceRefList{
		{Kind: "note", Ref: "n1"},
		{Kind: "receipt", Ref: "r1"},
	}
	if err := list.Validate(); err != nil {
		t.Fatalf("valid list: %v", err)
	}

	badList := SourceRefList{
		{Kind: "note", Ref: "n1"},
		{Kind: "", Ref: "r1"},
	}
	if err := badList.Validate(); err == nil {
		t.Error("list with invalid ref should fail")
	}
}
