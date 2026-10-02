package agentprotocol

import (
	"strings"
	"testing"
)

func TestNoteProposalValidationRiskAndRationale(t *testing.T) {
	for _, kind := range []string{"inbox_capture", "journal_append", "note_draft"} {
		c := NoteChange{Kind: kind, Title: "Title", Body: "body", Rationale: strings.Repeat("中", 500)}
		if kind == "journal_append" {
			c.Title = ""
		}
		if err := c.Validate(); err != nil || c.Risk() != RiskLow {
			t.Fatal("whitelist validation failed", err)
		}
		c.Rationale += "中"
		if err := c.Validate(); err == nil {
			t.Fatal("rationale limit bypassed")
		}
	}
	c := NoteChange{Kind: "note_edit", Body: "body", TargetObjectID: "note-id", ExpectedRevision: "sha256:revision"}
	if err := c.Validate(); err != nil || c.Risk() != RiskMedium {
		t.Fatal("edit contract failed", err)
	}
	for _, kind := range []string{"organize", "repair", "future"} {
		c.Kind = kind
		if err := c.Validate(); err == nil {
			t.Fatal("unknown risk entered the whitelist")
		}
	}
}
