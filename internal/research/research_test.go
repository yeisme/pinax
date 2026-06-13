package research

import "testing"

func TestFakeResearchAdapter(t *testing.T) {
	adapter := NewFakeAdapter([]Evidence{{URL: "https://example.test/ai", Title: "AI tooling", Summary: "Agent workflow"}})
	resp, err := adapter.Search(ResearchRequest{Topic: "AI", Limit: 3, Capabilities: []string{"daily_hot_notes"}})
	if err != nil {
		t.Fatalf("fake search: %v", err)
	}
	if resp.SchemaVersion != ResearchResponseSchemaVersion || len(resp.Evidence) != 1 || resp.Provider != "fake" {
		t.Fatalf("response = %#v", resp)
	}
}

func TestHermesAdapterFallsBackToFakeFixture(t *testing.T) {
	adapter := NewHermesAdapter(HermesConfig{}, NewFakeAdapter(nil))
	resp, err := adapter.Search(ResearchRequest{Topic: "AI tooling", Limit: 2})
	if err != nil {
		t.Fatalf("hermes fallback: %v", err)
	}
	if resp.Provider != "fake" || len(resp.Evidence) == 0 {
		t.Fatalf("fallback response = %#v", resp)
	}
}

func TestResearchRequestValidation(t *testing.T) {
	adapter := NewFakeAdapter(nil)
	if _, err := adapter.Search(ResearchRequest{}); err == nil {
		t.Fatalf("empty topic accepted")
	}
	if _, err := adapter.Search(ResearchRequest{Topic: "AI", Limit: 100}); err == nil {
		t.Fatalf("oversized limit accepted")
	}
}
