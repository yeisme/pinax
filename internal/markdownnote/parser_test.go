package markdownnote

import "testing"

func TestParseFullExtractsFrontmatterHeadingsLinksTasksAndFencedBlocks(t *testing.T) {
	content := "---\n" +
		"schema_version: pinax.note.v1\n" +
		"note_id: note_alpha\n" +
		"title: Alpha\n" +
		"tags:\n" +
		"  - research\n" +
		"  - search\n" +
		"---\n\n" +
		"# Alpha Heading\n\n" +
		"See [[Beta|beta alias]] and [Gamma](gamma.md).\n\n" +
		"progress:: active\n\n" +
		"- [ ] Ship native search\n\n" +
		"```pinax-sql active\nSELECT title FROM notes LIMIT 5\n```\n"

	doc, err := ParseFull("notes/alpha.md", []byte(content))
	if err != nil {
		t.Fatalf("ParseFull returned error: %v", err)
	}
	if doc.Note.ID != "note_alpha" || doc.Note.Title != "Alpha" || doc.Note.Path != "notes/alpha.md" {
		t.Fatalf("note summary = %#v", doc.Note)
	}
	if got := doc.Frontmatter["tags"]; got != "research,search" {
		t.Fatalf("flattened tags = %q", got)
	}
	if len(doc.Headings) != 1 || doc.Headings[0].Text != "Alpha Heading" || doc.Headings[0].Level != 1 {
		t.Fatalf("headings = %#v", doc.Headings)
	}
	if len(doc.Links) != 2 || doc.Links[0].Target != "Beta" || doc.Links[1].Target != "gamma.md" {
		t.Fatalf("links = %#v", doc.Links)
	}
	if len(doc.Tasks) != 1 || doc.Tasks[0].Text != "Ship native search" || doc.Tasks[0].Done {
		t.Fatalf("tasks = %#v", doc.Tasks)
	}
	if len(doc.Properties) != 1 || doc.Properties[0].Name != "progress" || doc.Properties[0].Value != "active" {
		t.Fatalf("properties = %#v", doc.Properties)
	}
	if len(doc.FencedBlocks) != 1 || doc.FencedBlocks[0].Language != "pinax-sql" || doc.FencedBlocks[0].Info != "active" {
		t.Fatalf("fenced blocks = %#v", doc.FencedBlocks)
	}
}

func TestParseFullFallsBackToFirstHeadingWhenTitleMissing(t *testing.T) {
	content := []byte("---\nschema_version: pinax.note.v1\nnote_id: note_beta\n---\n\n# Beta Title\n\nBody")
	doc, err := ParseFull("beta.md", content)
	if err != nil {
		t.Fatalf("ParseFull returned error: %v", err)
	}
	if doc.Note.Title != "Beta Title" {
		t.Fatalf("title = %q", doc.Note.Title)
	}
}

func TestParseFullHandlesBodyImmediatelyAfterClosingFrontmatter(t *testing.T) {
	content := []byte("---\nschema_version: pinax.note.v1\nnote_id: note_draft_1\ntitle: Draft Item 1\nstatus: draft\nkind: draft\n---\n# Draft Item 1\nbody draft\n")
	doc, err := ParseFull("drafts/draft-item-1.md", content)
	if err != nil {
		t.Fatalf("ParseFull returned error: %v", err)
	}
	if doc.Note.ID != "note_draft_1" || doc.Note.Status != "draft" || doc.Note.Kind != "draft" || doc.Note.Title != "Draft Item 1" {
		t.Fatalf("note = %#v frontmatter=%#v", doc.Note, doc.Frontmatter)
	}
}
