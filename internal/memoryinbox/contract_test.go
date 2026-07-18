package memoryinbox

import (
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/agentprotocol"
)

func validInboxItem() InboxItem {
	return InboxItem{
		SchemaVersion:   InboxSchemaVersion,
		ItemID:          "item-1",
		Category:        CategoryNewFact,
		ReasonCodes:     []ReasonCode{ReasonNewProposal},
		Risk:            RiskLow,
		Subject:         "Test proposal",
		Scope:           agentprotocol.Scope{Kind: agentprotocol.ScopeKindProject, ID: "pinax"},
		SuggestedAction: ActionReview,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
		Experimental:    true,
	}
}

func TestInboxItem_Validate_OK(t *testing.T) {
	item := validInboxItem()
	if err := item.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInboxItem_Validate_MissingItemID(t *testing.T) {
	item := validInboxItem()
	item.ItemID = ""
	if err := item.Validate(); err == nil {
		t.Fatal("expected error for missing item_id")
	}
}

func TestInboxItem_Validate_UnknownCategory(t *testing.T) {
	item := validInboxItem()
	item.Category = InboxCategory("unknown_category")
	if err := item.Validate(); err == nil {
		t.Fatal("expected error for unknown category")
	}
}

func TestInboxItem_Validate_UnknownRisk(t *testing.T) {
	item := validInboxItem()
	item.Risk = RiskLevel("critical")
	if err := item.Validate(); err == nil {
		t.Fatal("expected error for unknown risk")
	}
}

func TestInboxItem_ForwardCompatibility(t *testing.T) {
	// Unknown optional metadata must not break validation
	item := validInboxItem()
	item.HandoffID = "handoff-xyz"
	item.ReceiptID = "receipt-abc"
	item.ReviewVersion = 3
	if err := item.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInboxPack_Validate_OK(t *testing.T) {
	pack := InboxPack{
		SchemaVersion: InboxSchemaVersion,
		Items:         []InboxItem{validInboxItem()},
		Experimental:  true,
	}
	if err := pack.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInboxPack_ComputeCounts(t *testing.T) {
	pack := InboxPack{
		SchemaVersion: InboxSchemaVersion,
		Items: []InboxItem{
			{Category: CategoryConflict, Risk: RiskHigh},
			{Category: CategoryNewFact, Risk: RiskLow},
			{Category: CategoryNewFact, Risk: RiskLow},
			{Category: CategoryStale, Risk: RiskMedium},
		},
	}
	pack.ComputeCounts()
	if pack.TotalItems != 4 {
		t.Errorf("TotalItems = %d, want 4", pack.TotalItems)
	}
	if pack.CountsByCategory[CategoryNewFact] != 2 {
		t.Errorf("NewFact count = %d, want 2", pack.CountsByCategory[CategoryNewFact])
	}
	if pack.CountsByCategory[CategoryConflict] != 1 {
		t.Errorf("Conflict count = %d, want 1", pack.CountsByCategory[CategoryConflict])
	}
	if pack.HighRiskCount != 1 {
		t.Errorf("HighRiskCount = %d, want 1", pack.HighRiskCount)
	}
}

func TestInboxPack_SortedItems(t *testing.T) {
	pack := InboxPack{
		SchemaVersion: InboxSchemaVersion,
		Items: []InboxItem{
			{ItemID: "a", Category: CategoryNewFact, Risk: RiskLow},
			{ItemID: "b", Category: CategoryConflict, Risk: RiskHigh},
			{ItemID: "c", Category: CategoryStale, Risk: RiskMedium},
			{ItemID: "d", Category: CategoryConflict, Risk: RiskLow},
		},
	}
	sorted := pack.SortedItems()
	// Conflict items first (b=high, d=low), then stale (c), then new_fact (a)
	if sorted[0].ItemID != "b" {
		t.Errorf("first item = %s, want b (conflict+high)", sorted[0].ItemID)
	}
	if sorted[1].ItemID != "d" {
		t.Errorf("second item = %s, want d (conflict+low)", sorted[1].ItemID)
	}
	if sorted[2].ItemID != "c" {
		t.Errorf("third item = %s, want c (stale)", sorted[2].ItemID)
	}
	if sorted[3].ItemID != "a" {
		t.Errorf("fourth item = %s, want a (new_fact)", sorted[3].ItemID)
	}
}

func TestAllCategories_Deterministic(t *testing.T) {
	cats := AllCategories()
	if len(cats) != 9 {
		t.Errorf("expected 9 categories, got %d", len(cats))
	}
	// Verify deterministic order
	if cats[0] != CategoryConflict {
		t.Error("first category should be conflict")
	}
}

func TestSourceCoverage_Default(t *testing.T) {
	sc := SourceCoverage{}
	if sc.Total != 0 || sc.Resolved != 0 {
		t.Error("default SourceCoverage should be zero")
	}
}
