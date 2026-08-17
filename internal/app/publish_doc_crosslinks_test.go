package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func writeCrossLinkNote(t *testing.T, root, rel, noteID, title, body string) {
	t.Helper()
	content := "---\nschema_version: pinax.note.v1\nnote_id: " + noteID + "\ntitle: " + title + "\nkind: concept\nstatus: active\n---\n\n" + body + "\n"
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCrossLinkMapping(t *testing.T, root, noteID, url string) {
	t.Helper()
	target := domain.PublishDocTargetLarkDoc
	mapping := domain.PublishDocMapping{
		SchemaVersion:  domain.PublishDocMappingSchemaVersion,
		NoteID:         noteID,
		Target:         target,
		Provider:       "lark",
		ExternalObject: domain.PublishDocExternalObject{Provider: "lark", Target: string(target), Type: domain.PublishDocObjectTypeDocx, ID: "tok_" + noteID, URL: url},
		ContentDigest:  "sha256:dummy",
		PublishStatus:  domain.PublishDocStatusPublished,
	}
	if err := writePublishDocMapping(root, mapping); err != nil {
		t.Fatal(err)
	}
}

func TestPublishDocResolveCrossDocLinksRewritesWikiLink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "# Alpha\n\nSee [[Beta]] for details.")
	writeCrossLinkNote(t, root, "notes/b.md", "note_b", "Beta", "# Beta\n\nbody")
	writeCrossLinkMapping(t, root, "note_b", "https://example.test/docx/b_token")

	source := domain.Note{ID: "note_a", Title: "Alpha", Path: "notes/a.md"}
	body := "See [[Beta]] for details."
	out, count := publishDocResolveCrossDocLinks(root, source, body)
	if count != 1 {
		t.Fatalf("expected 1 resolved cross-doc link, got %d", count)
	}
	if !containsStr(out, "https://example.test/docx/b_token") {
		t.Fatalf("rewritten body missing feishu url: %q", out)
	}
	if containsStr(out, "[[Beta]]") {
		t.Fatalf("wikilink not rewritten: %q", out)
	}
}

func TestPublishDocResolveCrossDocLinksRewritesMarkdownLink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "see [beta note](b.md)")
	writeCrossLinkNote(t, root, "notes/b.md", "note_b", "Beta", "body")
	writeCrossLinkMapping(t, root, "note_b", "https://example.test/docx/b_token")

	source := domain.Note{ID: "note_a", Title: "Alpha", Path: "notes/a.md"}
	body := "see [beta note](b.md)"
	out, count := publishDocResolveCrossDocLinks(root, source, body)
	if count != 1 {
		t.Fatalf("expected 1 resolved, got %d: %q", count, out)
	}
	if !containsStr(out, "[beta note](https://example.test/docx/b_token)") {
		t.Fatalf("markdown link not rewritten: %q", out)
	}
}

func TestPublishDocResolveCrossDocLinksSkipsUnpublishedTarget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "See [[Beta]].")
	writeCrossLinkNote(t, root, "notes/b.md", "note_b", "Beta", "body")
	// note_b 没有发布 mapping → 引用保留原样

	source := domain.Note{ID: "note_a", Title: "Alpha", Path: "notes/a.md"}
	body := "See [[Beta]]."
	out, count := publishDocResolveCrossDocLinks(root, source, body)
	if count != 0 {
		t.Fatalf("unpublished target should not resolve, got %d", count)
	}
	if !containsStr(out, "[[Beta]]") {
		t.Fatalf("wikilink to unpublished note must stay as-is: %q", out)
	}
}

func TestPublishDocResolveCrossDocLinksSkipsDetachedTarget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "See [[Beta]].")
	writeCrossLinkNote(t, root, "notes/b.md", "note_b", "Beta", "body")
	writeCrossLinkMapping(t, root, "note_b", "https://example.test/docx/b_token")
	mapping, err := readPublishDocMapping(root, "note_b", domain.PublishDocTargetLarkDoc)
	if err != nil {
		t.Fatal(err)
	}
	mapping.PublishStatus = domain.PublishDocStatusDetached
	if err := writePublishDocMapping(root, mapping); err != nil {
		t.Fatal(err)
	}

	source := domain.Note{ID: "note_a", Title: "Alpha", Path: "notes/a.md"}
	result := publishDocAnalyzeCrossDocLinks(root, source, "See [[Beta]].", domain.PublishDocTargetLarkDoc, &publishDocSnapshotLoader{})
	if result.Summary.Rewritten != 0 || result.Summary.Unpublished != 1 {
		t.Fatalf("detached mapping must be treated as unpublished, summary=%+v body=%q", result.Summary, result.Body)
	}
	if !containsStr(result.Body, "[[Beta]]") {
		t.Fatalf("detached target link must stay local: %q", result.Body)
	}
}

func TestPublishDocResolveCrossDocLinksSkipsSelfReference(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "self [[Alpha]]")
	writeCrossLinkMapping(t, root, "note_a", "https://example.test/docx/a_token")

	source := domain.Note{ID: "note_a", Title: "Alpha", Path: "notes/a.md"}
	body := "self [[Alpha]]"
	out, count := publishDocResolveCrossDocLinks(root, source, body)
	if count != 0 {
		t.Fatalf("self-reference should not resolve, got %d", count)
	}
	if !containsStr(out, "[[Alpha]]") {
		t.Fatalf("self wikilink should stay as-is: %q", out)
	}
}

func TestPublishDocResolveCrossDocLinksWikiAlias(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "[[Beta|the beta]]")
	writeCrossLinkNote(t, root, "notes/b.md", "note_b", "Beta", "body")
	writeCrossLinkMapping(t, root, "note_b", "https://example.test/docx/b_token")

	source := domain.Note{ID: "note_a", Title: "Alpha", Path: "notes/a.md"}
	out, count := publishDocResolveCrossDocLinks(root, source, "[[Beta|the beta]]")
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}
	if !containsStr(out, "[the beta](https://example.test/docx/b_token)") {
		t.Fatalf("alias not preserved: %q", out)
	}
}

func TestPublishDocResolveCrossDocLinksUnresolvedTargetStays(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "[[NonExistent]] note")
	// no note "NonExistent" → unresolved, stays as-is

	source := domain.Note{ID: "note_a", Title: "Alpha", Path: "notes/a.md"}
	out, count := publishDocResolveCrossDocLinks(root, source, "[[NonExistent]] note")
	if count != 0 {
		t.Fatalf("unresolved should not count, got %d", count)
	}
	if !containsStr(out, "[[NonExistent]]") {
		t.Fatalf("unresolved wikilink must stay: %q", out)
	}
}

func TestPublishDocAnalyzeCrossDocLinksCountsOccurrencesAndConflicts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "[[Beta]] and [[Beta]] and [[Missing]] and [[Draft]]")
	writeCrossLinkNote(t, root, "notes/b.md", "note_b", "Beta", "body")
	writeCrossLinkNote(t, root, "notes/d.md", "note_d", "Draft", "body")
	writeCrossLinkMapping(t, root, "note_b", "https://example.test/docx/b_token")

	source := domain.Note{ID: "note_a", Title: "Alpha", Path: "notes/a.md"}
	result := publishDocAnalyzeCrossDocLinks(root, source, "[[Beta]] and [[Beta]] and [[Missing]] and [[Draft]]", domain.PublishDocTargetLarkDoc, &publishDocSnapshotLoader{})
	if result.Summary.Total != 3 {
		t.Fatalf("expected three distinct raw links, got %d", result.Summary.Total)
	}
	if result.Summary.Rewritten != 2 {
		t.Fatalf("expected two rewritten occurrences, got %d", result.Summary.Rewritten)
	}
	if result.Summary.Broken != 1 {
		t.Fatalf("expected one broken link, got %d", result.Summary.Broken)
	}
	if result.Summary.Unpublished != 1 {
		t.Fatalf("expected one unpublished link, got %d", result.Summary.Unpublished)
	}
	if containsStr(result.Body, "[[Beta]]") {
		t.Fatalf("published link should be rewritten in all occurrences: %q", result.Body)
	}
	if !containsStr(result.Body, "[[Missing]]") || !containsStr(result.Body, "[[Draft]]") {
		t.Fatalf("unresolved/unpublished links must stay local: %q", result.Body)
	}
}

func TestPublishDocAnalyzeCrossDocLinksReportsAmbiguous(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "[[Beta]]")
	writeCrossLinkNote(t, root, "notes/b.md", "note_b", "Beta", "body")
	writeCrossLinkNote(t, root, "other/b.md", "note_b2", "Beta", "body")

	source := domain.Note{ID: "note_a", Title: "Alpha", Path: "notes/a.md"}
	result := publishDocAnalyzeCrossDocLinks(root, source, "[[Beta]]", domain.PublishDocTargetLarkDoc, &publishDocSnapshotLoader{})
	if result.Summary.Ambiguous != 1 {
		t.Fatalf("expected ambiguous link, got summary: %+v", result.Summary)
	}
	if len(result.Summary.Links) != 1 || len(result.Summary.Links[0].Candidates) != 2 {
		t.Fatalf("expected two ambiguity candidates, got: %+v", result.Summary.Links)
	}
	if result.Body != "[[Beta]]" {
		t.Fatalf("ambiguous link must stay unchanged: %q", result.Body)
	}
}

func TestPublishDocPrepareAllProcessesVaultCrossDocSummary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "[[Beta]] and [[Draft]] and [[Missing]]")
	writeCrossLinkNote(t, root, "notes/b.md", "note_b", "Beta", "body")
	writeCrossLinkNote(t, root, "notes/d.md", "note_d", "Draft", "body")
	writeCrossLinkMapping(t, root, "note_b", "https://example.test/docx/b_token")
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	profile.Folder = "fld_test"
	if err := writePublishDocProfile(root, profile); err != nil {
		t.Fatal(err)
	}

	projection, err := NewService().PublishDocPrepareAll(context.Background(), PublishRequest{VaultPath: root, Target: "lark-doc"})
	if err != nil {
		t.Fatalf("prepare all failed: %v", err)
	}
	for key, want := range map[string]string{"all": "true", "notes": "3", "packages": "3", "cross_doc_links": "1", "cross_doc_unpublished": "1", "cross_doc_broken": "1"} {
		if got := projection.Facts[key]; got != want {
			t.Fatalf("fact %s = %q, want %q; facts=%v", key, got, want, projection.Facts)
		}
	}
	if len(projection.Actions) == 0 || !containsStr(projection.Actions[0].Command, "pinax publish doc push --all") {
		t.Fatalf("prepare all should suggest vault-wide push: %+v", projection.Actions)
	}
}

func TestPublishDocPushAllDryRunDoesNotWritePackages(t *testing.T) {
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "body")
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	profile.Folder = "fld_test"
	if err := writePublishDocProfile(root, profile); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(root, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeLark := filepath.Join(fakeBin, "lark-cli")
	if err := os.WriteFile(fakeLark, []byte("#!/bin/sh\nif [ \"$1\" = \"docs\" ] && [ \"$2\" = \"+create\" ]; then echo '{\"ok\":true}'; exit 0; fi\necho '{\"ok\":true}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	projection, err := NewService().PublishDocPushAll(context.Background(), PublishRequest{VaultPath: root, Target: "lark-doc", DryRun: true})
	if err != nil {
		t.Fatalf("push all dry-run failed: %v projection=%+v", err, projection)
	}
	if projection.Facts["dry_run"] != "true" || projection.Facts["notes"] != "1" {
		t.Fatalf("unexpected dry-run facts: %+v", projection.Facts)
	}
	packageDir := filepath.Join(root, ".pinax", "publish", "doc", "packages")
	entries, readErr := os.ReadDir(packageDir)
	if readErr == nil && len(entries) > 0 {
		t.Fatalf("dry-run wrote package files under %s: %+v", packageDir, entries)
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read package dir: %v", readErr)
	}
}

func TestPublishDocPushAllRequiresApprovalForRemoteWrites(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "body")
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	profile.Folder = "fld_test"
	if err := writePublishDocProfile(root, profile); err != nil {
		t.Fatal(err)
	}

	projection, err := NewService().PublishDocPushAll(context.Background(), PublishRequest{VaultPath: root, Target: "lark-doc"})
	if err == nil {
		t.Fatalf("push all without --yes unexpectedly succeeded: %+v", projection)
	}
	if projection.Error == nil || projection.Error.Code != "approval_required" {
		t.Fatalf("expected approval_required, got %+v", projection.Error)
	}
	if len(projection.Actions) == 0 || !containsStr(projection.Actions[0].Command, "--yes") {
		t.Fatalf("approval action missing --yes: %+v", projection.Actions)
	}
}

func TestPublishDocPushRejectsPackageRendererMismatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "notes/a.md", "note_a", "Alpha", "body")
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	profile.Folder = "fld_test"
	profile.Renderer = domain.PublishDocRendererMarkdownFile
	if err := writePublishDocProfile(root, profile); err != nil {
		t.Fatal(err)
	}
	prepare, err := NewService().PublishDocPrepare(context.Background(), PublishRequest{VaultPath: root, Target: "lark-doc", Note: "note_a"})
	if err != nil {
		t.Fatalf("prepare markdown-file package: %v", err)
	}
	pkg, ok := publishDocProjectionPackage(prepare)
	if !ok || pkg.ID == "" {
		t.Fatalf("prepared package missing: %+v", prepare)
	}
	profile.Renderer = domain.PublishDocRendererNativeDocx
	if err := writePublishDocProfile(root, profile); err != nil {
		t.Fatal(err)
	}

	projection, err := NewService().PublishDocPush(context.Background(), PublishRequest{VaultPath: root, Target: "lark-doc", PackageID: pkg.ID, DryRun: true})
	if err == nil {
		t.Fatalf("push unexpectedly accepted mismatched package: %+v", projection)
	}
	if projection.Error == nil || projection.Error.Code != "publish_package_renderer_mismatch" {
		t.Fatalf("expected renderer mismatch, got %+v", projection.Error)
	}
}

func TestPublishDocMoveObjectTypeUsesNativeMappingType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		mapping domain.PublishDocMapping
		want    string
	}{
		{name: "explicit docx", mapping: domain.PublishDocMapping{ExternalObject: domain.PublishDocExternalObject{Type: domain.PublishDocObjectTypeDocx}}, want: domain.PublishDocObjectTypeDocx},
		{name: "native renderer", mapping: domain.PublishDocMapping{Target: domain.PublishDocTargetLarkDoc, Renderer: domain.PublishDocRendererNativeDocx}, want: domain.PublishDocObjectTypeDocx},
		{name: "markdown renderer", mapping: domain.PublishDocMapping{Target: domain.PublishDocTargetLarkDoc, Renderer: domain.PublishDocRendererMarkdownFile}, want: domain.PublishDocObjectTypeFile},
	} {
		if got := publishDocMoveObjectType(tc.mapping); got != tc.want {
			t.Fatalf("%s move object type = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPublishDocProfileSetPreservesIndexObject(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	profile.Folder = "fld_old"
	profile.IndexObject = &domain.PublishDocExternalObject{Provider: "lark", Target: "lark-doc", Type: domain.PublishDocObjectTypeDocx, ID: "idx_docx", URL: "https://example.test/docx/idx"}
	if err := writePublishDocProfile(root, profile); err != nil {
		t.Fatal(err)
	}

	_, err := NewService().PublishDocProfileSet(context.Background(), PublishRequest{VaultPath: root, Target: "lark-doc", Folder: "fld_new", Renderer: "native-docx"})
	if err != nil {
		t.Fatalf("profile set failed: %v", err)
	}
	updated, err := readPublishDocProfile(root, domain.PublishDocTargetLarkDoc)
	if err != nil {
		t.Fatal(err)
	}
	if updated.IndexObject == nil || updated.IndexObject.ID != "idx_docx" || updated.IndexObject.URL == "" {
		t.Fatalf("index object was not preserved: %+v", updated.IndexObject)
	}
	if updated.Folder != "fld_new" {
		t.Fatalf("profile update did not apply new folder: %+v", updated)
	}
}

func TestPublishDocUnlinkAllDetachesTargetMappings(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	profile.Folder = "fld_test"
	if err := writePublishDocProfile(root, profile); err != nil {
		t.Fatal(err)
	}
	writeCrossLinkMapping(t, root, "note_a", "https://example.test/docx/a")
	writeCrossLinkMapping(t, root, "note_b", "https://example.test/docx/b")

	projection, err := NewService().PublishDocUnlinkAll(context.Background(), PublishRequest{VaultPath: root, Target: "lark-doc"})
	if err != nil {
		t.Fatalf("unlink all failed: %v", err)
	}
	if projection.Facts["detached"] != "2" {
		t.Fatalf("detached fact = %q, facts=%v", projection.Facts["detached"], projection.Facts)
	}
	for _, noteID := range []string{"note_a", "note_b"} {
		mapping, err := readPublishDocMapping(root, noteID, domain.PublishDocTargetLarkDoc)
		if err != nil {
			t.Fatalf("read mapping %s: %v", noteID, err)
		}
		if mapping.PublishStatus != domain.PublishDocStatusDetached {
			t.Fatalf("mapping %s status = %s", noteID, mapping.PublishStatus)
		}
	}
}

func TestPublishDocPrepareUsesRewrittenBodyForNativePlanAssets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCrossLinkNote(t, root, "alpha.md", "note_alpha", "Alpha", "See [[Beta]].\n\nMarkdown link to [Beta md](beta.md).")
	writeCrossLinkNote(t, root, "beta.md", "note_beta", "Beta", "body")
	writeCrossLinkMapping(t, root, "note_beta", "https://example.test/docx/beta")
	profile := domain.NewPublishDocProfile(domain.PublishDocTargetLarkDoc)
	profile.Folder = "fld_test"
	profile.Template = "plain"
	if err := writePublishDocProfile(root, profile); err != nil {
		t.Fatal(err)
	}

	projection, err := NewService().PublishDocPrepare(context.Background(), PublishRequest{VaultPath: root, Target: "lark-doc", Note: "note_alpha"})
	if err != nil {
		t.Fatalf("prepare failed: %v", err)
	}
	pkg, ok := publishDocProjectionPackage(projection)
	if !ok {
		t.Fatalf("package missing from projection: %#v", projection.Data)
	}
	if pkg.CrossDocLinks == nil || pkg.CrossDocLinks.Rewritten != 2 {
		t.Fatalf("expected two rewritten cross-doc links, got %+v", pkg.CrossDocLinks)
	}
	if containsStr(string(pkg.NativePlan), `"kind":"attachment"`) || containsStr(string(pkg.NativePlan), "beta.md") {
		t.Fatalf("native plan should use rewritten body and not treat cross-doc markdown link as attachment: %s", string(pkg.NativePlan))
	}
	if !containsStr(pkg.BodyMarkdown, "https://example.test/docx/beta") {
		t.Fatalf("body markdown did not include rewritten Feishu URL: %q", pkg.BodyMarkdown)
	}
}
