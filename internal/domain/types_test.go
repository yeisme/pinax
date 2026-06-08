package domain

import (
	"encoding/json"
	"testing"
)

func TestLinkKindValidation(t *testing.T) {
	for _, kind := range []string{"wiki", "markdown"} {
		if !IsValidLinkKind(kind) {
			t.Fatalf("expected %q to be valid link kind", kind)
		}
	}
	for _, kind := range []string{"", "external", "Wiki", "MARKDOWN"} {
		if IsValidLinkKind(kind) {
			t.Fatalf("expected %q to be invalid link kind", kind)
		}
	}
}

func TestLinkStatusValidation(t *testing.T) {
	for _, status := range []string{"resolved", "broken", "ambiguous", "external", "ignored"} {
		if !IsValidLinkStatus(status) {
			t.Fatalf("expected %q to be valid link status", status)
		}
	}
	for _, status := range []string{"", "unknown", "Resolved"} {
		if IsValidLinkStatus(status) {
			t.Fatalf("expected %q to be invalid link status", status)
		}
	}
}

func TestNoteLinkJSONIncludesExtendedFields(t *testing.T) {
	link := NoteLink{
		SourcePath:    "notes/a.md",
		SourceTitle:   "A",
		Target:        "B",
		TargetPath:    "notes/b.md",
		TargetTitle:   "B",
		Kind:          "wiki",
		Broken:        false,
		SourceNoteID:  "note_a",
		TargetNoteID:  "note_b",
		TargetRaw:     "B",
		TargetHeading: "Section",
		Status:        "resolved",
		Line:          5,
		Evidence:      "wiki link resolved by title",
		Candidates:    []NoteLinkCandidate{{Path: "notes/b.md", Title: "B", NoteID: "note_b"}},
	}
	b, err := json.Marshal(link)
	if err != nil {
		t.Fatalf("marshal note link: %v", err)
	}
	s := string(b)
	for _, want := range []string{
		`"source_path":"notes/a.md"`,
		`"source_note_id":"note_a"`,
		`"target_note_id":"note_b"`,
		`"target_heading":"Section"`,
		`"status":"resolved"`,
		`"line":5`,
		`"evidence":"wiki link resolved by title"`,
		`"candidates":[{"path":"notes/b.md","title":"B","note_id":"note_b"}]`,
	} {
		if !contains(s, want) {
			t.Fatalf("JSON missing %q:\n%s", want, s)
		}
	}
}

func TestNoteLinkBackwardCompat(t *testing.T) {
	// 旧代码只设置 Kind 和 Broken，不设置 Status。
	link := NoteLink{Kind: "wiki", Broken: true}
	if link.Status != "" {
		t.Fatalf("expected empty status for legacy note link")
	}
	b, _ := json.Marshal(link)
	if !contains(string(b), `"broken":true`) {
		t.Fatalf("expected broken=true in JSON: %s", string(b))
	}
}

func TestNoteGraphProjectionJSON(t *testing.T) {
	p := NoteGraphProjection{
		Engine:      "index",
		IndexStatus: "fresh",
		TotalNotes:  10,
		TotalLinks:  25,
		Resolved:    20,
		Broken:      3,
		Ambiguous:   1,
		Ignored:     1,
		Orphans:     2,
		Facts:       map[string]string{"vault": "/tmp/test"},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal graph projection: %v", err)
	}
	s := string(b)
	for _, want := range []string{`"engine":"index"`, `"total_notes":10`, `"resolved":20`, `"broken":3`, `"ambiguous":1`} {
		if !contains(s, want) {
			t.Fatalf("JSON missing %q:\n%s", want, s)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
