package briefing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceLedgerDedupeAndTrust(t *testing.T) {
	root := t.TempDir()
	items := []EvidenceItem{
		{SourceID: "fake:default", URL: "https://example.test/a?utm_source=x", Title: "Alpha", Summary: "One", TrustHint: 0.7},
		{SourceID: "fake:default", URL: "https://example.test/a", Title: "Alpha duplicate", Summary: "Duplicate", TrustHint: 0.5},
		{SourceID: "hermes:hot", URL: "https://example.test/b", Title: "Beta", Summary: "Two"},
	}
	ledger, err := WriteEvidence(root, items)
	if err != nil {
		t.Fatalf("write evidence: %v", err)
	}
	if ledger.SchemaVersion != EvidenceLedgerSchemaVersion || len(ledger.Items) != 2 || ledger.Duplicates != 1 {
		t.Fatalf("ledger = %#v", ledger)
	}
	if ledger.Items[0].CanonicalURL != "https://example.test/a" {
		t.Fatalf("canonical url = %#v", ledger.Items[0])
	}
	if ledger.Items[0].TrustScore != 0.7 || ledger.Items[1].TrustScore <= ledger.Items[0].TrustScore {
		t.Fatalf("trust scores = %#v", ledger.Items)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "briefing", "evidence.jsonl")); err != nil {
		t.Fatalf("evidence asset missing: %v", err)
	}
}

func TestEvidenceSourceTrust(t *testing.T) {
	cases := map[string]float64{"user:curated": 1, "hermes:hot": 0.8, "fake:default": 0.6, "unknown": 0.4}
	for source, want := range cases {
		if got := SourceTrust(source, 0); got != want {
			t.Fatalf("SourceTrust(%q) = %v want %v", source, got, want)
		}
	}
	if got := SourceTrust("fake:default", 1.4); got != 1 {
		t.Fatalf("trust hint should clamp to 1, got %v", got)
	}
}
