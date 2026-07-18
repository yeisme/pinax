package memoryinbox

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentmemory"
	"github.com/yeisme/pinax/internal/agentprotocol"
)

// validProposalRow returns an AgentProposalRow with valid defaults for inbox
// classification. The caller customises id and status per scenario.
func validProposalRow(id, status string) agentmemory.AgentProposalRow {
	return agentmemory.AgentProposalRow{
		ProposalID:  id,
		PrincipalID: "p_1",
		ScopeKind:   string(agentprotocol.ScopeKindProject),
		ScopeID:     "proj_1",
		Kind:        string(agentprotocol.MemoryKindFact),
		Subject:     "Test proposal " + id,
		Status:      status,
		CreatedAt:   time.Now().UTC(),
	}
}

func TestAggregate_PendingProposals(t *testing.T) {
	agg := NewAggregator()
	input := AggregateInput{
		Proposals: []agentmemory.AgentProposalRow{
			validProposalRow("prop-1", string(agentprotocol.ProposalStatusApprovalRequired)),
			validProposalRow("prop-2", string(agentprotocol.ProposalStatusApprovalRequired)),
		},
	}

	pack, err := agg.Aggregate(context.Background(), input, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pack.TotalItems != 2 {
		t.Fatalf("TotalItems = %d, want 2", pack.TotalItems)
	}
	if !pack.Experimental {
		t.Error("pack should be marked experimental")
	}

	for _, item := range pack.Items {
		if item.Category != CategoryNewFact {
			t.Errorf("item %s: category = %s, want %s", item.ItemID, item.Category, CategoryNewFact)
		}
		if item.Risk != RiskLow {
			t.Errorf("item %s: risk = %s, want %s", item.ItemID, item.Risk, RiskLow)
		}
		if item.SuggestedAction != ActionReview {
			t.Errorf("item %s: action = %s, want %s", item.ItemID, item.SuggestedAction, ActionReview)
		}
		if item.ProposalID == "" {
			t.Errorf("item %s: proposal_id should be set", item.ItemID)
		}
		if !item.Experimental {
			t.Errorf("item %s: should be marked experimental", item.ItemID)
		}
	}
}

func TestAggregate_ConflictProposal(t *testing.T) {
	agg := NewAggregator()
	input := AggregateInput{
		Proposals: []agentmemory.AgentProposalRow{
			validProposalRow("prop-conflict", string(agentprotocol.ProposalStatusConflictRequired)),
		},
	}

	pack, err := agg.Aggregate(context.Background(), input, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pack.TotalItems != 1 {
		t.Fatalf("TotalItems = %d, want 1", pack.TotalItems)
	}

	item := pack.Items[0]
	if item.Category != CategoryConflict {
		t.Errorf("category = %s, want %s", item.Category, CategoryConflict)
	}
	if item.Risk != RiskHigh {
		t.Errorf("risk = %s, want %s", item.Risk, RiskHigh)
	}
	if item.SuggestedAction != ActionInspect {
		t.Errorf("action = %s, want %s", item.SuggestedAction, ActionInspect)
	}
	if pack.HighRiskCount != 1 {
		t.Errorf("HighRiskCount = %d, want 1", pack.HighRiskCount)
	}

	found := false
	for _, rc := range item.ReasonCodes {
		if rc == ReasonConflictDetected {
			found = true
		}
	}
	if !found {
		t.Errorf("reason codes %v should contain %s", item.ReasonCodes, ReasonConflictDetected)
	}
}

func TestAggregate_DuplicateFiltering(t *testing.T) {
	agg := NewAggregator()
	input := AggregateInput{
		Proposals: []agentmemory.AgentProposalRow{
			validProposalRow("prop-approved", string(agentprotocol.ProposalStatusApproved)),
			validProposalRow("prop-rejected", string(agentprotocol.ProposalStatusRejected)),
			validProposalRow("prop-superseded", string(agentprotocol.ProposalStatusSuperseded)),
		},
	}

	pack, err := agg.Aggregate(context.Background(), input, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pack.TotalItems != 0 {
		t.Fatalf("TotalItems = %d, want 0 (approved/rejected/superseded must be filtered)", pack.TotalItems)
	}
	if len(pack.Items) != 0 {
		t.Errorf("Items should be empty, got %d", len(pack.Items))
	}
}

func TestAggregate_StaleMemory(t *testing.T) {
	agg := NewAggregator()
	now := time.Now().UTC()
	input := AggregateInput{
		Memories: []agentprotocol.MemoryRecord{
			{
				SchemaVersion: "test",
				ID:            "mem-stale",
				Kind:          agentprotocol.MemoryKindFact,
				Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_1"},
				State:         agentprotocol.LifecycleConfirmed,
				Subject:       "Old confirmed memory",
				CreatorID:     "p_1",
				CreatedAt:     now.Add(-100 * 24 * time.Hour),
				UpdatedAt:     now.Add(-100 * 24 * time.Hour),
			},
		},
	}

	pack, err := agg.Aggregate(context.Background(), input, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pack.TotalItems != 1 {
		t.Fatalf("TotalItems = %d, want 1", pack.TotalItems)
	}

	item := pack.Items[0]
	if item.Category != CategoryStale {
		t.Errorf("category = %s, want %s", item.Category, CategoryStale)
	}
	if item.Risk != RiskLow {
		t.Errorf("risk = %s, want %s", item.Risk, RiskLow)
	}
	if item.MemoryID != "mem-stale" {
		t.Errorf("MemoryID = %s, want mem-stale", item.MemoryID)
	}
	if item.ItemID != "mem-mem-stale" {
		t.Errorf("ItemID = %s, want mem-mem-stale", item.ItemID)
	}
}

func TestAggregate_ExpiredMemory(t *testing.T) {
	agg := NewAggregator()
	now := time.Now().UTC()
	input := AggregateInput{
		Memories: []agentprotocol.MemoryRecord{
			{
				SchemaVersion: "test",
				ID:            "mem-expired",
				Kind:          agentprotocol.MemoryKindFact,
				Scope:         agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_1"},
				State:         agentprotocol.LifecycleExpired,
				Subject:       "Expired memory",
				CreatorID:     "p_1",
				CreatedAt:     now.Add(-30 * 24 * time.Hour),
				UpdatedAt:     now.Add(-1 * 24 * time.Hour),
			},
		},
	}

	pack, err := agg.Aggregate(context.Background(), input, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pack.TotalItems != 1 {
		t.Fatalf("TotalItems = %d, want 1", pack.TotalItems)
	}

	item := pack.Items[0]
	if item.Category != CategoryExpired {
		t.Errorf("category = %s, want %s", item.Category, CategoryExpired)
	}
	if item.Risk != RiskLow {
		t.Errorf("risk = %s, want %s", item.Risk, RiskLow)
	}
	if item.MemoryID != "mem-expired" {
		t.Errorf("MemoryID = %s, want mem-expired", item.MemoryID)
	}
	if item.SuggestedAction != ActionInspect {
		t.Errorf("action = %s, want %s", item.SuggestedAction, ActionInspect)
	}
}

func TestAggregate_Limit(t *testing.T) {
	agg := NewAggregator()
	proposals := make([]agentmemory.AgentProposalRow, 5)
	for i := range 5 {
		proposals[i] = validProposalRow(
			fmt.Sprintf("prop-%d", i),
			string(agentprotocol.ProposalStatusApprovalRequired),
		)
	}
	input := AggregateInput{Proposals: proposals}

	pack, err := agg.Aggregate(context.Background(), input, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pack.Truncated {
		t.Error("expected Truncated=true when limit exceeded")
	}
	if len(pack.Items) != 2 {
		t.Errorf("len(Items) = %d, want 2", len(pack.Items))
	}
	if pack.TotalItems != 2 {
		t.Errorf("TotalItems = %d, want 2 (post-truncation count)", pack.TotalItems)
	}

	// Verify that limit=0 disables truncation
	fullPack, err := agg.Aggregate(context.Background(), input, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fullPack.Truncated {
		t.Error("expected Truncated=false when limit=0")
	}
	if len(fullPack.Items) != 5 {
		t.Errorf("len(Items) = %d, want 5", len(fullPack.Items))
	}
}

func TestAggregate_Deterministic(t *testing.T) {
	agg := NewAggregator()
	now := time.Now().UTC()
	input := AggregateInput{
		Proposals: []agentmemory.AgentProposalRow{
			validProposalRow("prop-a", string(agentprotocol.ProposalStatusApprovalRequired)),
			validProposalRow("prop-b", string(agentprotocol.ProposalStatusConflictRequired)),
		},
		Memories: []agentprotocol.MemoryRecord{
			{
				ID:        "mem-stale",
				Kind:      agentprotocol.MemoryKindFact,
				Scope:     agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "proj_1"},
				State:     agentprotocol.LifecycleConfirmed,
				CreatorID: "p_1",
				CreatedAt: now.Add(-100 * 24 * time.Hour),
				UpdatedAt: now.Add(-100 * 24 * time.Hour),
			},
		},
	}

	pack1, err := agg.Aggregate(context.Background(), input, 0)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	pack2, err := agg.Aggregate(context.Background(), input, 0)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}

	if pack1.TotalItems != pack2.TotalItems {
		t.Fatalf("TotalItems differ: %d vs %d", pack1.TotalItems, pack2.TotalItems)
	}
	if pack1.HighRiskCount != pack2.HighRiskCount {
		t.Errorf("HighRiskCount differ: %d vs %d", pack1.HighRiskCount, pack2.HighRiskCount)
	}
	if len(pack1.Items) != len(pack2.Items) {
		t.Fatalf("item count differs: %d vs %d", len(pack1.Items), len(pack2.Items))
	}
	for i := range pack1.Items {
		a, b := pack1.Items[i], pack2.Items[i]
		if a.ItemID != b.ItemID {
			t.Errorf("item[%d] ItemID: %q vs %q", i, a.ItemID, b.ItemID)
		}
		if a.Category != b.Category {
			t.Errorf("item[%d] Category: %q vs %q", i, a.Category, b.Category)
		}
		if a.Risk != b.Risk {
			t.Errorf("item[%d] Risk: %q vs %q", i, a.Risk, b.Risk)
		}
		if a.SuggestedAction != b.SuggestedAction {
			t.Errorf("item[%d] SuggestedAction: %q vs %q", i, a.SuggestedAction, b.SuggestedAction)
		}
		if a.Subject != b.Subject {
			t.Errorf("item[%d] Subject: %q vs %q", i, a.Subject, b.Subject)
		}
	}
	// Conflict has highest category priority and must sort first
	if pack1.TotalItems > 1 && pack1.Items[0].Category != CategoryConflict {
		t.Errorf("first sorted item category = %s, want %s",
			pack1.Items[0].Category, CategoryConflict)
	}
}

func TestIsBulkAllowed_HighRisk(t *testing.T) {
	items := []InboxItem{
		{ItemID: "a", Category: CategoryNewFact, Risk: RiskLow},
		{ItemID: "b", Category: CategoryNewFact, Risk: RiskHigh},
	}
	ok, reason := IsBulkAllowed(items)
	if ok {
		t.Error("expected bulk to be blocked by high-risk item")
	}
	if reason != ReasonBulkActionBlocked {
		t.Errorf("reason = %s, want %s", reason, ReasonBulkActionBlocked)
	}
}

func TestIsBulkAllowed_Conflict(t *testing.T) {
	// Conflict item with low risk isolates the conflict path; otherwise the
	// high-risk check would short-circuit first.
	items := []InboxItem{
		{ItemID: "a", Category: CategoryConflict, Risk: RiskLow},
	}
	ok, reason := IsBulkAllowed(items)
	if ok {
		t.Error("expected bulk to be blocked by conflict item")
	}
	if reason != ReasonConflictingScope {
		t.Errorf("reason = %s, want %s", reason, ReasonConflictingScope)
	}
}

func TestIsBulkAllowed_LowRisk(t *testing.T) {
	items := []InboxItem{
		{ItemID: "a", Category: CategoryNewFact, Risk: RiskLow},
		{ItemID: "b", Category: CategoryStale, Risk: RiskLow},
		{ItemID: "c", Category: CategoryPreference, Risk: RiskMedium},
	}
	ok, reason := IsBulkAllowed(items)
	if !ok {
		t.Errorf("expected bulk to be allowed, got reason %s", reason)
	}
	if reason != "" {
		t.Errorf("reason = %s, want empty", reason)
	}
}

func TestSummaryLine(t *testing.T) {
	// Pack with conflict and high-risk items
	pack := InboxPack{
		SchemaVersion: InboxSchemaVersion,
		Items: []InboxItem{
			{Category: CategoryConflict, Risk: RiskHigh},
			{Category: CategoryNewFact, Risk: RiskLow},
			{Category: CategoryNewFact, Risk: RiskLow},
		},
	}
	pack.ComputeCounts()

	summary := SummaryLine(pack)
	if !strings.Contains(summary, "3 items") {
		t.Errorf("summary %q should contain total item count", summary)
	}
	if !strings.Contains(summary, "1 high-risk") {
		t.Errorf("summary %q should contain high-risk count", summary)
	}
	if !strings.Contains(summary, "1 conflicts") {
		t.Errorf("summary %q should contain conflict count", summary)
	}
	if strings.Contains(summary, "truncated") {
		t.Errorf("summary %q should not mention truncated", summary)
	}

	// Truncated pack
	truncatedPack := InboxPack{
		SchemaVersion: InboxSchemaVersion,
		Truncated:     true,
		Items: []InboxItem{
			{Category: CategoryNewFact, Risk: RiskLow},
		},
	}
	truncatedPack.ComputeCounts()

	truncatedSummary := SummaryLine(truncatedPack)
	if !strings.Contains(truncatedSummary, "truncated") {
		t.Errorf("summary %q should contain truncated marker", truncatedSummary)
	}
}
