package semantic

import (
	"context"
	"testing"

	"github.com/yeisme/lance"
)

func TestKBDomain_Name(t *testing.T) {
	d := KBDomain{}
	if got := d.Name(); got != "kb" {
		t.Fatalf("Name() = %q, want %q", got, "kb")
	}
}

func TestKBDomain_TableName(t *testing.T) {
	d := KBDomain{}
	if got := d.TableName(); got != "note_chunks" {
		t.Fatalf("TableName() = %q, want %q", got, "note_chunks")
	}
}

func TestKBDomain_Redact(t *testing.T) {
	d := KBDomain{}
	in := map[string]any{
		"chunk_id":         "c1",
		"note_id":          "n1",
		"vault_path":       "vault/note.md",
		"heading_path":     "Section > Sub",
		"title":            "My Note",
		"preview":          "short preview...",
		"kind":             "note",
		"status":           "active",
		"tags":             []string{"x", "y"},
		"content_hash":     "abc",
		"chunk_hash":       "def",
		"token_count":      42,
		"note_body":        "full body that must be stripped",
		"full_text":        "full text that must be stripped",
		"raw_prompt":       "secret prompt",
		"provider_payload": map[string]any{"x": 1},
		"authorization":    "Bearer token",
		"token":            "tok",
		"secret":           "shh",
		"ocr_text":         "extracted text",
	}
	out := d.Redact(in)

	for _, field := range []string{"note_body", "full_text", "raw_prompt", "provider_payload", "authorization", "token", "secret", "ocr_text"} {
		if _, ok := out[field]; ok {
			t.Errorf("Redact() left sensitive field %q in output", field)
		}
	}
	for _, field := range []string{"chunk_id", "note_id", "vault_path", "heading_path", "title", "preview", "kind", "status", "tags", "content_hash", "chunk_hash", "token_count"} {
		if _, ok := out[field]; !ok {
			t.Errorf("Redact() dropped safe field %q", field)
		}
	}
	// Redact must not mutate the input map.
	if _, ok := in["note_body"]; !ok {
		t.Errorf("Redact() mutated the input map")
	}
	// Nil input is safe.
	if got := d.Redact(nil); got != nil {
		t.Errorf("Redact(nil) = %v, want nil", got)
	}
}

func TestKBDomain_ResolvePermission_StatusFilter(t *testing.T) {
	d := KBDomain{}
	records := []lance.Record{
		{ID: "r1", Metadata: map[string]any{"status": "active", "kind": "note"}},
		{ID: "r2", Metadata: map[string]any{"status": "archived", "kind": "note"}},
		{ID: "r3", Metadata: map[string]any{"status": "active", "kind": "journal"}},
	}
	got := d.ResolvePermission(context.Background(), records, lance.PermissionFilter{
		"note_status": []string{"active"},
	})
	want := map[string]bool{"r1": true, "r3": true}
	if len(got) != len(want) {
		t.Fatalf("ResolvePermission() = %v, want %d ids", got, len(want))
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("ResolvePermission() returned unexpected id %q", id)
		}
	}
}

func TestKBDomain_ResolvePermission_KindFilter(t *testing.T) {
	d := KBDomain{}
	records := []lance.Record{
		{ID: "r1", Metadata: map[string]any{"status": "active", "kind": "note"}},
		{ID: "r2", Metadata: map[string]any{"status": "active", "kind": "journal"}},
	}
	got := d.ResolvePermission(context.Background(), records, lance.PermissionFilter{
		"note_kind": []any{"journal"},
	})
	if len(got) != 1 || got[0] != "r2" {
		t.Fatalf("ResolvePermission(kind=journal) = %v, want [r2]", got)
	}
}

func TestKBDomain_ResolvePermission_NoFilter_AllAll(t *testing.T) {
	d := KBDomain{}
	records := []lance.Record{
		{ID: "r1", Metadata: map[string]any{"status": "active"}},
		{ID: "r2", Metadata: map[string]any{"status": "archived"}},
		{ID: "r3"},
	}
	got := d.ResolvePermission(context.Background(), records, nil)
	if len(got) != 3 {
		t.Fatalf("ResolvePermission(nil filter) = %v, want all 3 ids", got)
	}
}
