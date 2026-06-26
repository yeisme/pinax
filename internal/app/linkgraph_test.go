package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestParseNoteLinksWikiBasic(t *testing.T) {
	body := "# Title\n\nSee [[Alpha]] and [[Beta|Note B]] for details.\nAlso [[Gamma#Section]].\n"
	links := parseRawLinksFromBody(body)
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d: %+v", len(links), links)
	}
	if links[0].Kind != "wiki" || links[0].Target != "Alpha" {
		t.Fatalf("first link = %+v", links[0])
	}
	if links[1].Alias != "Note B" || links[1].Target != "Beta" {
		t.Fatalf("second link (alias) = %+v", links[1])
	}
	if links[2].Heading != "Section" || links[2].Target != "Gamma" {
		t.Fatalf("third link (heading) = %+v", links[2])
	}
}

func TestParseNoteLinksKeepsDistinctWikiAliasesAndHeadings(t *testing.T) {
	body := "[[Alpha|Short]] [[Alpha#Details]] [[Alpha|Short]]\n"
	links := parseRawLinksFromBody(body)
	if len(links) != 2 {
		t.Fatalf("expected exact duplicate to collapse but alias and heading variants to remain, got %d: %+v", len(links), links)
	}
	if links[0].Raw != "Alpha|Short" || links[0].Alias != "Short" || links[0].Heading != "" {
		t.Fatalf("first link = %+v", links[0])
	}
	if links[1].Raw != "Alpha#Details" || links[1].Alias != "" || links[1].Heading != "Details" {
		t.Fatalf("second link = %+v", links[1])
	}
}

func TestParseNoteLinksIgnoresWikiMediaEmbeds(t *testing.T) {
	body := "![[diagram.png]] [[Alpha]] ![[clips/demo.mp4|Demo]]\n"
	links := parseRawLinksFromBody(body)
	if len(links) != 1 {
		t.Fatalf("expected only note wiki link, got %d: %+v", len(links), links)
	}
	if links[0].Target != "Alpha" {
		t.Fatalf("target = %q", links[0].Target)
	}
}

func TestParseNoteLinksMarkdownRelative(t *testing.T) {
	body := "# Title\n\n[link](../other.md) and [another](notes/sub/deep.md)\n"
	links := parseRawLinksFromBody(body)
	if len(links) != 2 {
		t.Fatalf("expected 2 links, got %d: %+v", len(links), links)
	}
	if links[0].Kind != "markdown" {
		t.Fatalf("first link kind = %q", links[0].Kind)
	}
	if links[1].Target != "notes/sub/deep.md" {
		t.Fatalf("second link target = %q", links[1].Target)
	}
}

func TestParseNoteLinksIgnoresExternal(t *testing.T) {
	body := "[web](https://example.com) [mail](mailto:a@b) [local](#section)\n"
	links := parseRawLinksFromBody(body)
	if len(links) != 0 {
		t.Fatalf("expected 0 links, got %d: %+v", len(links), links)
	}
}

func TestParseNoteLinksLineNumber(t *testing.T) {
	body := "line1\nline2\nSee [[Target]] here\n"
	links := parseRawLinksFromBody(body)
	if len(links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(links))
	}
	if links[0].Line != 3 {
		t.Fatalf("expected line 3, got %d", links[0].Line)
	}
}

func TestSplitWikiLinkParts(t *testing.T) {
	tests := []struct {
		input, target, alias, heading string
	}{
		{"Title", "Title", "", ""},
		{"Title|Alias", "Title", "Alias", ""},
		{"Title#Section", "Title", "", "Section"},
		{"Title|Alias#Section", "Title", "Alias#Section", ""},
	}
	for i, tt := range tests {
		target, alias, heading := splitWikiLinkParts(tt.input)
		if target != tt.target || alias != tt.alias || heading != tt.heading {
			t.Fatalf("case %d: split(%q) = (%q,%q,%q), want (%q,%q,%q)", i, tt.input, target, alias, heading, tt.target, tt.alias, tt.heading)
		}
	}
}

func TestResolverSnapshotResolveByTitle(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_a", Title: "Alpha", Path: "notes/alpha.md"},
		{ID: "note_b", Title: "Beta", Path: "notes/beta.md"},
	}
	snap := BuildResolverSnapshot(notes)
	raw := ParseRawLink{Kind: "wiki", Target: "Alpha", Raw: "Alpha", Line: 1}
	result := ResolveLinkTarget(notes[1], raw, snap)
	if result.Link.Status != string(domain.LinkStatusResolved) {
		t.Fatalf("expected resolved, got %q: %+v", result.Link.Status, result.Link)
	}
	if result.Link.TargetPath != "notes/alpha.md" {
		t.Fatalf("target path = %q", result.Link.TargetPath)
	}
}

func TestResolverSnapshotResolveByPath(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_a", Title: "Alpha", Path: "notes/alpha.md"},
	}
	snap := BuildResolverSnapshot(notes)
	raw := ParseRawLink{Kind: "wiki", Target: "notes/alpha.md", Raw: "notes/alpha.md", Line: 1}
	result := ResolveLinkTarget(notes[0], raw, snap)
	if result.Link.Status != string(domain.LinkStatusResolved) {
		t.Fatalf("expected resolved by path, got %q", result.Link.Status)
	}
}

func TestResolverSnapshotResolveByNoteID(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_abc123", Title: "Alpha", Path: "notes/alpha.md"},
	}
	snap := BuildResolverSnapshot(notes)
	raw := ParseRawLink{Kind: "wiki", Target: "note_abc123", Raw: "note_abc123", Line: 1}
	result := ResolveLinkTarget(notes[0], raw, snap)
	if result.Link.Status != string(domain.LinkStatusResolved) {
		t.Fatalf("expected resolved by note_id, got %q", result.Link.Status)
	}
	if result.Link.Evidence != "resolved by note_id" {
		t.Fatalf("evidence = %q", result.Link.Evidence)
	}
}

func TestResolverSnapshotAmbiguousTitle(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_a", Title: "Meeting", Path: "notes/work/meeting-a.md"},
		{ID: "note_b", Title: "Meeting", Path: "notes/work/meeting-b.md"},
	}
	snap := BuildResolverSnapshot(notes)
	raw := ParseRawLink{Kind: "wiki", Target: "Meeting", Raw: "Meeting", Line: 1}
	result := ResolveLinkTarget(notes[0], raw, snap)
	if result.Link.Status != string(domain.LinkStatusAmbiguous) {
		t.Fatalf("expected ambiguous, got %q", result.Link.Status)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(result.Candidates))
	}
}

func TestResolverSnapshotResolveByFrontmatterAlias(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_alpha", Title: "Alpha", Path: "notes/alpha.md", Frontmatter: map[string]string{"aliases": "[One, First Alpha]"}},
		{ID: "note_source", Title: "Source", Path: "notes/source.md"},
	}
	snap := BuildResolverSnapshot(notes)
	raw := ParseRawLink{Kind: "wiki", Target: "First Alpha", Raw: "First Alpha", Line: 1}
	result := ResolveLinkTarget(notes[1], raw, snap)
	if result.Link.Status != string(domain.LinkStatusResolved) {
		t.Fatalf("expected resolved by frontmatter alias, got %q: %+v", result.Link.Status, result.Link)
	}
	if result.Link.TargetPath != "notes/alpha.md" || result.Link.Evidence != "resolved by alias/stem" {
		t.Fatalf("unexpected result: %+v", result.Link)
	}
}

func TestResolverSnapshotBrokenLink(t *testing.T) {
	notes := []domain.Note{
		{ID: "note_a", Title: "Alpha", Path: "notes/alpha.md"},
	}
	snap := BuildResolverSnapshot(notes)
	raw := ParseRawLink{Kind: "wiki", Target: "NonExistent", Raw: "NonExistent", Line: 1}
	result := ResolveLinkTarget(notes[0], raw, snap)
	if result.Link.Status != string(domain.LinkStatusBroken) {
		t.Fatalf("expected broken, got %q", result.Link.Status)
	}
	if !result.Link.Broken {
		t.Fatalf("expected Broken=true")
	}
}

func TestBuildEnhancedLinkGraphEndToEnd(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\ntags: [test]\n---\n\n# Alpha\n\nSee [[Beta]] and [[Missing]].\n")
	writeFile(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\ntags: [test]\n---\n\n# Beta\n\nBack to [[Alpha]].\n")

	// Test outgoing links
	linksProj, err := svc.NoteLinks(ctx, NoteLinkRequest{VaultPath: root, NoteRef: "Alpha"})
	if err != nil {
		t.Fatalf("note links: %v", err)
	}
	if linksProj.Facts["links"] == "0" {
		t.Fatalf("expected links, got facts = %#v", linksProj.Facts)
	}
	// Alpha should have 2 outgoing links: Beta (resolved) and Missing (broken)
	if linksProj.Facts["broken"] == "0" {
		t.Fatalf("expected broken links, got facts = %#v", linksProj.Facts)
	}

	// Test backlinks
	backProj, err := svc.NoteBacklinks(ctx, NoteLinkRequest{VaultPath: root, NoteRef: "Alpha"})
	if err != nil {
		t.Fatalf("note backlinks: %v", err)
	}
	if backProj.Facts["backlinks"] == "0" {
		t.Fatalf("expected backlinks, got facts = %#v", backProj.Facts)
	}

	// Test orphans
	orphanProj, err := svc.NoteOrphans(ctx, VaultRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("note orphans: %v", err)
	}
	if orphanProj.Facts["engine"] == "" {
		t.Fatalf("expected engine fact, got facts = %#v", orphanProj.Facts)
	}
}

func TestGraphSummary(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_a\ntitle: A\ntags: [test]\n---\n\n# A\n\n[[B]]\n")
	writeFile(t, filepath.Join(root, "notes", "b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_b\ntitle: B\ntags: [test]\n---\n\n# B\n\n[[Missing]]\n")

	summary, err := svc.GraphSummary(ctx, root)
	if err != nil {
		t.Fatalf("graph summary: %v", err)
	}
	if summary.TotalNotes != 2 {
		t.Fatalf("total_notes = %d", summary.TotalNotes)
	}
	if summary.TotalLinks == 0 {
		t.Fatalf("total_links = 0")
	}
	if summary.Broken == 0 {
		t.Fatalf("expected at least 1 broken link")
	}
}

func TestLinkGraphCompatibilityMatrixAndRepairPlanFacts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "source.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_source\ntitle: Source\n---\n\n# Source\n\nSee [[Target|Alias]], [[Target#Details]], [[Missing]], and [[Duplicate]].\n")
	writeFile(t, filepath.Join(root, "notes", "target.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_target\ntitle: Target\naliases: [T]\n---\n\n# Target\n")
	writeFile(t, filepath.Join(root, "notes", "dup-a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_dup_a\ntitle: Duplicate\n---\n\n# Duplicate\n")
	writeFile(t, filepath.Join(root, "notes", "dup-b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_dup_b\ntitle: Duplicate\n---\n\n# Duplicate\n")

	links, err := svc.NoteLinks(ctx, NoteLinkRequest{VaultPath: root, NoteRef: "Source"})
	if err != nil {
		t.Fatalf("note links: %v", err)
	}
	if links.Status != "partial" || links.Facts["ambiguous"] != "1" || links.Facts["broken"] != "1" || links.Facts["compat.wikilink"] != "supported" || links.Facts["compat.alias"] != "supported" || links.Facts["repair_mode"] != "plan_only" {
		t.Fatalf("link compatibility facts = %#v status=%s", links.Facts, links.Status)
	}
	if len(links.Actions) == 0 || !strings.Contains(links.Actions[0].Command, "pinax repair plan") {
		t.Fatalf("links should point to repair plan: %#v", links.Actions)
	}

	backlinks, err := svc.NoteBacklinks(ctx, NoteLinkRequest{VaultPath: root, NoteRef: "Target"})
	if err != nil {
		t.Fatalf("note backlinks: %v", err)
	}
	if backlinks.Facts["compat.backlink"] != "supported" || backlinks.Facts["backlinks"] == "0" {
		t.Fatalf("backlink compatibility facts = %#v", backlinks.Facts)
	}
	summary, err := svc.GraphSummary(ctx, root)
	if err != nil {
		t.Fatalf("graph summary: %v", err)
	}
	if summary.Facts["compat.graph"] != "supported" || summary.Facts["compat.ambiguous_repair"] != "manual_review" {
		t.Fatalf("graph summary facts = %#v", summary.Facts)
	}
}

func TestQueryOutgoingLinksWithFilters(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_a\ntitle: A\ntags: []\n---\n\n# A\n\n[[B]] [[Missing]]\n")
	writeFile(t, filepath.Join(root, "notes", "b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_b\ntitle: B\ntags: []\n---\n\n# B\n")

	// broken-only filter
	proj, err := svc.QueryOutgoingLinks(ctx, NoteLinkGraphRequest{VaultPath: root, NoteRef: "A", BrokenOnly: true})
	if err != nil {
		t.Fatalf("query outgoing broken: %v", err)
	}
	if proj.Facts["links"] == "0" {
		t.Fatalf("expected broken links, got facts = %#v", proj.Facts)
	}
	if proj.Facts["resolved"] != "0" {
		t.Fatalf("expected no resolved in broken-only filter, got %q", proj.Facts["resolved"])
	}

	// kind filter
	proj, err = svc.QueryOutgoingLinks(ctx, NoteLinkGraphRequest{VaultPath: root, NoteRef: "A", Kind: "markdown"})
	if err != nil {
		t.Fatalf("query outgoing markdown: %v", err)
	}
	if proj.Facts["links"] != "0" {
		t.Fatalf("expected 0 markdown links, got %q", proj.Facts["links"])
	}
}

func TestQueryOrphansModes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_a\ntitle: A\ntags: []\n---\n\n# A\n\n[[B]]\n")
	writeFile(t, filepath.Join(root, "notes", "b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_b\ntitle: B\ntags: []\n---\n\n# B\n")
	writeFile(t, filepath.Join(root, "notes", "c.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_c\ntitle: C\ntags: []\n---\n\n# C\n")

	// full mode: only C is orphan (no in, no out)
	proj, err := svc.QueryOrphans(ctx, NoteOrphansRequest{VaultPath: root, Mode: "full"})
	if err != nil {
		t.Fatalf("orphans full: %v", err)
	}
	if proj.Facts["orphans"] != "1" {
		t.Fatalf("expected 1 full orphan, got %q", proj.Facts["orphans"])
	}

	// no-incoming mode: B and C have no incoming links
	proj, err = svc.QueryOrphans(ctx, NoteOrphansRequest{VaultPath: root, Mode: "no-incoming"})
	if err != nil {
		t.Fatalf("orphans no-incoming: %v", err)
	}
	if proj.Facts["orphans"] != "2" {
		t.Fatalf("expected 2 no-incoming orphans, got %q", proj.Facts["orphans"])
	}

	// no-outgoing mode: B and C have no outgoing links
	proj, err = svc.QueryOrphans(ctx, NoteOrphansRequest{VaultPath: root, Mode: "no-outgoing"})
	if err != nil {
		t.Fatalf("orphans no-outgoing: %v", err)
	}
	if proj.Facts["orphans"] != "2" {
		t.Fatalf("expected 2 no-outgoing orphans, got %q", proj.Facts["orphans"])
	}
}

func TestNoteLinkBackwardCompatibility(t *testing.T) {
	// 验证旧代码创建的 NoteLink（只有 Broken 字段）仍然正常工作
	link := domain.NoteLink{
		SourcePath:  "notes/a.md",
		SourceTitle: "A",
		Target:      "B",
		Kind:        "wiki",
		Broken:      true,
	}
	if !link.Broken {
		t.Fatalf("expected Broken=true for legacy link")
	}
	if link.Status != "" {
		// 旧代码不设置 Status，新代码应该能处理
		t.Fatalf("expected empty Status for legacy link, got %q", link.Status)
	}
}

func TestNoteLinksOutputHasEngineAndIndexStatus(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Test"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFile(t, filepath.Join(root, "notes", "a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_a\ntitle: A\ntags: []\n---\n\n# A\n\n[[B]]\n")

	proj, err := svc.NoteLinks(ctx, NoteLinkRequest{VaultPath: root, NoteRef: "A"})
	if err != nil {
		t.Fatalf("note links: %v", err)
	}
	if proj.Facts["engine"] == "" {
		t.Fatalf("expected engine fact, got facts = %#v", proj.Facts)
	}
	if proj.Facts["note_id"] != "note_a" {
		t.Fatalf("expected note_id=note_a, got facts = %#v", proj.Facts)
	}
}

func TestIsExternalOrHeadingLink(t *testing.T) {
	for _, target := range []string{"https://example.com", "http://a.b", "mailto:x@y", "#section", "ftp://files"} {
		if !isExternalOrHeadingLink(target) {
			t.Fatalf("expected %q to be external/heading", target)
		}
	}
	for _, target := range []string{"notes/a.md", "../other.md", "relative.md", "Title"} {
		if isExternalOrHeadingLink(target) {
			t.Fatalf("expected %q to NOT be external/heading", target)
		}
	}
}

func TestParseNoteLinksDedup(t *testing.T) {
	body := "[[A]] some text [[A]] more [[A]]\n"
	links := parseRawLinksFromBody(body)
	if len(links) != 1 {
		t.Fatalf("expected dedup to 1 link, got %d", len(links))
	}
}

func TestParseNoteLinksMarkdownRelativePath(t *testing.T) {
	// Markdown relative links to .md files should be detected
	body := "[doc](../docs/readme.md) and [local](./sub/note.md)\n"
	links := parseRawLinksFromBody(body)
	if len(links) != 2 {
		t.Fatalf("expected 2 markdown links, got %d: %+v", len(links), links)
	}
	// Verify they are normalized
	if !strings.Contains(links[0].Target, "docs/readme.md") && !strings.Contains(links[0].Target, "../docs/readme.md") {
		t.Fatalf("first markdown link target = %q", links[0].Target)
	}
}

func TestParseNoteLinksIgnoresNonMarkdown(t *testing.T) {
	body := "[image](../img/photo.png) [pdf](doc.pdf) [zip](file.zip)\n"
	links := parseRawLinksFromBody(body)
	if len(links) != 0 {
		t.Fatalf("expected 0 links for non-md, got %d", len(links))
	}
}
