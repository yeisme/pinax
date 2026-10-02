package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	noteindex "github.com/yeisme/pinax/internal/index"
)

// TestReaderAccess covers the P02 reader discovery lane at the application
// boundary: independent reader authorization, bounded pagination with
// session/query/index-bound cursors, index gating that never fabricates
// empty pages, read-only behavior, and snippet redaction. Fixture vaults
// only; no model, no sync, no real user vault.

func readerTestVault(t *testing.T, withIndex bool) string {
	t.Helper()
	root := t.TempDir()
	fixtures := map[string]string{
		"notes/alpha.md": readerNoteFixture("note_alpha", "Alpha 设计", "design", "active", "proj-1", "2026-09-01T10:00:00+00:00", "2026-09-10T10:00:00+00:00", "Alpha contains needle marker one."),
		"notes/beta.md":  readerNoteFixture("note_beta", "Beta <b>raw</b>", "design", "inbox", "proj-1", "2026-09-02T10:00:00+00:00", "2026-09-11T10:00:00+00:00", "Beta body with <script>alert(1)</script> needle."),
		"notes/gamma.md": readerNoteFixture("note_gamma", "Gamma 计划", "plan", "draft", "", "2026-09-03T10:00:00+00:00", "2026-09-12T10:00:00+00:00", "Gamma plain body."),
	}
	for path, content := range fixtures {
		writeAppFixture(t, filepath.Join(root, filepath.FromSlash(path)), content)
	}
	if withIndex {
		if err := refreshIndex(root); err != nil {
			t.Fatalf("refreshIndex: %v", err)
		}
	}
	return root
}

func readerNoteFixture(id, title, tag, status, project, createdAt, updatedAt, body string) string {
	frontmatter := fmt.Sprintf("---\nschema_version: pinax.note.v1\nnote_id: %s\ntitle: %q\ntags:\n  - %s\nstatus: %s\ncreated_at: %s\nupdated_at: %s\n", id, title, tag, status, createdAt, updatedAt)
	if project != "" {
		frontmatter += fmt.Sprintf("project: %s\n", project)
	}
	return frontmatter + "---\n\n" + body + "\n"
}

func readerTestSession(scopes ...string) ReaderSessionInfo {
	return ReaderSessionInfo{SessionID: "sess-1", BindingID: "binding-1", Scopes: scopes}
}

func readerCursorSecret() []byte {
	return []byte("test-cursor-secret-0123456789abcdef")
}

func statIndexFile(root string) (bool, error) {
	_, err := os.Stat(filepath.Join(root, ".pinax", "index.sqlite"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func removeIndexFile(root string) error {
	indexDir := filepath.Join(root, ".pinax")
	if err := os.RemoveAll(filepath.Join(indexDir, "index.sqlite")); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(indexDir, "index.sqlite-wal")); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(indexDir, "index.sqlite-shm"))
}

func TestReaderAccessRequiresIndependentScopes(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()

	// No metadata scope: source/notes refuse even with a valid session id.
	_, err := svc.ReaderSource(context.Background(), ReaderSourceRequest{VaultPath: root, Session: readerTestSession(ReaderScopeBody)})
	if err == nil || err.Error() == "" || !strings.Contains(err.Error(), "insufficient_scope") {
		t.Fatalf("source without metadata scope err = %v", err)
	}
	_, err = svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: readerTestSession(ReaderScopeBody), CursorSecret: readerCursorSecret()})
	if err == nil || !strings.Contains(err.Error(), "insufficient_scope") {
		t.Fatalf("notes without metadata scope err = %v", err)
	}
	// No body scope: search refuses even though metadata was granted.
	_, err = svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: readerTestSession(ReaderScopeMetadata), QueryText: "needle", CursorSecret: readerCursorSecret()})
	if err == nil || !strings.Contains(err.Error(), "insufficient_scope") {
		t.Fatalf("search without body scope err = %v", err)
	}
}

func TestReaderAccessSourceDescriptor(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	projection, err := NewService().ReaderSource(context.Background(), ReaderSourceRequest{
		VaultPath: root, Session: readerTestSession(ReaderScopeMetadata, ReaderScopeBody),
	})
	if err != nil {
		t.Fatalf("ReaderSource: %v", err)
	}
	source := projection.Data.(map[string]any)["source"].(ReaderSourceDescriptor)
	if source.SchemaVersion != "pinax.reader.source.v1alpha1" || source.SessionID != "sess-1" || source.BindingID != "binding-1" {
		t.Fatalf("source identity = %#v", source)
	}
	if len(source.Capabilities) != 2 || source.Capabilities[0] != ReaderScopeMetadata {
		t.Fatalf("capabilities = %#v", source.Capabilities)
	}
	if source.Limits.ListDefaultLimit != 50 || source.Limits.ListMaxLimit != 200 || source.Limits.SnippetMaxChars != 320 {
		t.Fatalf("limits drift = %#v", source.Limits)
	}
	if source.IndexState.Status != "ready" || source.IndexState.Watermark == "" {
		t.Fatalf("index state = %#v", source.IndexState)
	}
	if source.DisplayName == "" || strings.Contains(source.DisplayName, "/") {
		t.Fatalf("display name = %q", source.DisplayName)
	}
}

func TestReaderAccessMissingIndexIsNotAnEmptyPage(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, false)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)

	_, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, CursorSecret: readerCursorSecret()})
	if err == nil || !strings.Contains(err.Error(), "index_unavailable") {
		t.Fatalf("notes without index err = %v", err)
	}
	_, err = svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: "needle", CursorSecret: readerCursorSecret()})
	if err == nil || !strings.Contains(err.Error(), "index_unavailable") {
		t.Fatalf("search without index err = %v", err)
	}
	if exists, statErr := statIndexFile(root); statErr != nil || exists {
		t.Fatal("reader requests must not create or refresh the index")
	}
}

func TestReaderAccessListPaginationAndFilters(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata)

	projection, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("ReaderNotes: %v", err)
	}
	list := projection.Data.(map[string]any)["notes"].(ReaderNoteList)
	if len(list.Items) != 3 || list.NextCursor != "" {
		t.Fatalf("list = %#v", list)
	}
	// updated_desc default: gamma, beta, alpha.
	if list.Items[0].ObjectID != "note_gamma" || list.Items[2].ObjectID != "note_alpha" {
		t.Fatalf("order = %#v", list.Items)
	}
	card := list.Items[2] // alpha carries proj-1
	if card.ProjectID == nil || *card.ProjectID != "proj-1" {
		t.Fatalf("project id = %#v", card.ProjectID)
	}

	// limit default 50/max 200 and 400 otherwise.
	if _, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 201}, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("limit 201 err = %v", err)
	}
	if _, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Status: "bogus"}, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("bogus status err = %v", err)
	}
	if _, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Sort: "title_desc"}, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("bogus sort err = %v", err)
	}

	// Multi-tag AND semantics: design matches alpha+beta.
	projection, err = svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Tags: []string{"design"}}, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	design := projection.Data.(map[string]any)["notes"].(ReaderNoteList)
	if len(design.Items) != 2 {
		t.Fatalf("design items = %#v", design.Items)
	}
	projection, err = svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Tags: []string{"design", "plan"}}, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	both := projection.Data.(map[string]any)["notes"].(ReaderNoteList)
	if len(both.Items) != 0 {
		t.Fatalf("AND semantics broken: %#v", both.Items)
	}
}

func TestReaderAccessCursorBindings(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)
	otherSession := ReaderSessionInfo{SessionID: "sess-2", BindingID: "binding-2", Scopes: []string{ReaderScopeMetadata}}

	first, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1}, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	cursor := first.Data.(map[string]any)["notes"].(ReaderNoteList).NextCursor
	if cursor == "" {
		t.Fatal("expected a next cursor")
	}
	second, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1, Cursor: cursor}, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	secondList := second.Data.(map[string]any)["notes"].(ReaderNoteList)
	if len(secondList.Items) != 1 || secondList.Items[0].ObjectID == first.Data.(map[string]any)["notes"].(ReaderNoteList).Items[0].ObjectID {
		t.Fatalf("cursor did not advance: %#v", secondList.Items)
	}

	// Cross-session cursor: rejected (binding isolation).
	if _, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: otherSession, Query: ReaderPageQuery{Limit: 1, Cursor: cursor}, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_cursor_invalid") {
		t.Fatalf("cross-session cursor err = %v", err)
	}
	// Tampered cursor: rejected.
	if _, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1, Cursor: cursor + "x"}, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_cursor_invalid") {
		t.Fatalf("tampered cursor err = %v", err)
	}
	// Different query with the same cursor: rejected.
	if _, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1, Cursor: cursor, Status: "inbox"}, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_cursor_invalid") {
		t.Fatalf("cross-query cursor err = %v", err)
	}
	// Different signing secret: rejected.
	if _, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1, Cursor: cursor}, CursorSecret: []byte("other-secret")}); err == nil || !strings.Contains(err.Error(), "reader_cursor_invalid") {
		t.Fatalf("cross-secret cursor err = %v", err)
	}

	// Index generation change: cursor expires (410 vocabulary), never wraps
	// silently back to page one.
	writeAppFixture(t, filepath.Join(root, "notes", "delta.md"), readerNoteFixture("note_delta", "Delta", "plan", "active", "", "2026-09-04T10:00:00+00:00", "2026-09-13T10:00:00+00:00", "Delta body."))
	if err := refreshIndex(root); err != nil {
		t.Fatalf("refreshIndex: %v", err)
	}
	_, err = svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1, Cursor: cursor}, CursorSecret: readerCursorSecret()})
	if err == nil || !strings.Contains(err.Error(), "reader_cursor_expired") {
		t.Fatalf("stale generation cursor err = %v", err)
	}
}

func TestReaderAccessWatermarkBindsContentNotCount(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)

	first, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1}, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	cursor := first.Data.(map[string]any)["notes"].(ReaderNoteList).NextCursor
	if cursor == "" {
		t.Fatal("expected cursor")
	}

	// Same-size vault: edit beta in place, refresh — the note count and index
	// status stay identical, but content identity changed, so the watermark
	// must move and the outstanding cursor must expire.
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"), readerNoteFixture("note_beta", "Beta2", "design", "inbox", "proj-1", "2026-09-02T10:00:00+00:00", "2026-09-11T10:00:00+00:00", "Beta body with <script>alert(1)</script> needle, edited."))
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1, Cursor: cursor}, CursorSecret: readerCursorSecret()})
	if err == nil || !strings.Contains(err.Error(), "reader_cursor_expired") {
		t.Fatalf("content-drift cursor err = %v", err)
	}
}

func TestReaderAccessSearchScopesAndRedaction(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)

	projection, err := svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: "needle", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("ReaderSearch: %v", err)
	}
	results := projection.Data.(map[string]any)["search"].(ReaderSearchResults)
	if len(results.Items) != 2 {
		t.Fatalf("results = %#v", results.Items)
	}
	encoded, _ := json.Marshal(results)
	if strings.Contains(string(encoded), "<script>") || strings.Contains(string(encoded), "<b>") {
		t.Fatalf("raw HTML leaked into snippets: %s", encoded)
	}
	if len(results.Snippets) != 2 {
		t.Fatalf("snippets = %#v", results.Snippets)
	}
	for _, snippet := range results.Snippets {
		if len([]rune(snippet.Snippet)) > 320 {
			t.Fatalf("snippet exceeds 320 chars: %d", len([]rune(snippet.Snippet)))
		}
	}
	// The query itself must not appear in projection facts or evidence.
	if strings.Contains(fmt.Sprint(projection.Facts), "needle") {
		t.Fatalf("query leaked into facts: %#v", projection.Facts)
	}

	// Query bounds: 1..512 after trim.
	if _, err := svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: "   ", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("blank query err = %v", err)
	}
	if _, err := svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: strings.Repeat("q", 513), CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("oversized query err = %v", err)
	}

	// Search pagination uses the same cursor binding discipline.
	first, err := svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: "needle", Query: ReaderPageQuery{Limit: 1}, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	cursor := first.Data.(map[string]any)["search"].(ReaderSearchResults).NextCursor
	if cursor == "" {
		t.Fatal("expected search cursor")
	}
	_, err = svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: "plain", Query: ReaderPageQuery{Limit: 1, Cursor: cursor}, CursorSecret: readerCursorSecret()})
	if err == nil || !strings.Contains(err.Error(), "reader_cursor_invalid") {
		t.Fatalf("cross-query search cursor err = %v", err)
	}
}

func TestReaderAccessReadOnlyNeverRefreshes(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)
	before, err := statIndexFile(root)
	if err != nil || !before {
		t.Fatalf("stat index: %v %v", before, err)
	}
	// Remove the index: reader must fail closed, never rebuild.
	if err := removeIndexFile(root); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	if _, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "index_unavailable") {
		t.Fatalf("missing index err = %v", err)
	}
	if exists, err := statIndexFile(root); err == nil && exists {
		t.Fatal("reader recreated the missing index")
	}
}

func TestReaderAccessWatermarkBindsMetadata(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)

	first, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1}, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	cursor := first.Data.(map[string]any)["notes"].(ReaderNoteList).NextCursor
	if cursor == "" {
		t.Fatal("expected cursor")
	}

	// Metadata-only edit: same body, same count, same status — only the
	// title changes. The watermark must still move.
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"), readerNoteFixture("note_beta", "Beta Renamed", "design", "inbox", "proj-1", "2026-09-02T10:00:00+00:00", "2026-09-11T10:00:00+00:00", "Beta body with <script>alert(1)</script> needle."))
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{Limit: 1, Cursor: cursor}, CursorSecret: readerCursorSecret()})
	if err == nil || !strings.Contains(err.Error(), "reader_cursor_expired") {
		t.Fatalf("metadata-only edit cursor err = %v", err)
	}
}

func TestReaderAccessSnippetBoundHoldsAfterEscaping(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// A note whose first snippet window is almost entirely ampersands: after
	// escaping each & becomes &amp; (5 chars), so the bound must be applied
	// post-escape for the frozen 320-char wire budget to hold.
	writeAppFixture(t, filepath.Join(root, "notes", "amp.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_amp\ntitle: Amp\ndescription: "+strings.Repeat("&", 400)+"\nstatus: active\n---\n\n"+strings.Repeat("& ", 200)+"needle\n")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	projection, err := svc.ReaderSearch(context.Background(), ReaderSearchRequest{
		VaultPath: root, Session: readerTestSession(ReaderScopeMetadata, ReaderScopeBody),
		QueryText: "needle", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderSearch: %v", err)
	}
	results := projection.Data.(map[string]any)["search"].(ReaderSearchResults)
	for _, snippet := range results.Snippets {
		if len([]rune(snippet.Snippet)) > 320 {
			t.Fatalf("escaped snippet exceeds 320 chars: %d", len([]rune(snippet.Snippet)))
		}
	}
}

func TestReaderAccessHiddenAndSymlinkedNotesAreNotReaderResources(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)

	// A hidden markdown file with valid frontmatter...
	writeAppFixture(t, filepath.Join(root, "notes", ".secret.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_hidden\ntitle: Hidden\nstatus: active\n---\n\nzq-hidden-body-marker\n")
	// ...and a .md symlink pointing OUTSIDE the vault, both excluded from
	// the reader surface even though the legacy scan tolerates them.
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("---\nschema_version: pinax.note.v1\nnote_id: note_outside\ntitle: Outside\nstatus: active\n---\n\nzq-outside-body-marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "notes", "linked.md")); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}

	listing, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	list := listing.Data.(map[string]any)["notes"].(ReaderNoteList)
	encoded, _ := json.Marshal(list)
	if strings.Contains(string(encoded), "note_hidden") || strings.Contains(string(encoded), "note_outside") {
		t.Fatalf("hidden or symlinked note listed: %s", encoded)
	}

	for _, objectID := range []string{"note_hidden", "note_outside"} {
		if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: session, ObjectID: objectID, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_object_not_found") {
			t.Fatalf("%s document err = %v", objectID, err)
		}
		if _, _, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{VaultPath: root, Session: session, ObjectID: objectID, Revision: "sha256:0"}); err == nil {
			t.Fatalf("%s export accepted", objectID)
		}
	}
}

func TestReaderAccessMalformedTrustNeverLeaksHiddenValues(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)

	// A HIDDEN note whose trust field is malformed: the legacy scan would
	// fail with the raw field value and path; the reader surface must
	// neither error nor echo either of them.
	writeAppFixture(t, filepath.Join(root, "notes", ".badtrust.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_hidden_bad\ntitle: H\nstatus: active\nstale_after: zq-hidden-trust-marker\n---\n\nhidden body\n")
	// A VISIBLE note with the same malformed field: invisible to reader
	// listing rather than an error that echoes the value.
	writeAppFixture(t, filepath.Join(root, "notes", "badtrust.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_visible_bad\ntitle: V\nstatus: active\nstale_after: zq-visible-trust-marker\n---\n\nvisible body\n")
	// The legacy index refresh legitimately fails on malformed trust (its
	// own CLI contract); the reader surface must not. The fixture index
	// from readerTestVault stays in place for the reader gate.
	_ = refreshIndex(root)

	projection, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("ReaderNotes must not fail on malformed trust: %v", err)
	}
	encoded, _ := json.Marshal(projection)
	for _, leak := range []string{"zq-hidden-trust-marker", "zq-visible-trust-marker", ".badtrust.md", "badtrust.md"} {
		if strings.Contains(string(encoded), leak) {
			t.Fatalf("reader response leaked %q: %s", leak, encoded)
		}
	}
	list := projection.Data.(map[string]any)["notes"].(ReaderNoteList)
	if len(list.Items) != 3 { // the original alpha/beta/gamma fixture
		t.Fatalf("listing = %#v", list.Items)
	}
}

func TestReaderAccessUnknownProjectFilterIsRequestInvalid(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)

	// Unknown project_id is a 400 on both surfaces — never a silent empty
	// success that looks like "no notes".
	for _, call := range []struct {
		name string
		run  func() error
	}{
		{"notes", func() error {
			_, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{ProjectID: "no-such-project"}, CursorSecret: readerCursorSecret()})
			return err
		}},
		{"search", func() error {
			_, err := svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: "needle", Query: ReaderPageQuery{ProjectID: "no-such-project"}, CursorSecret: readerCursorSecret()})
			return err
		}},
	} {
		if err := call.run(); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
			t.Fatalf("%s unknown project err = %v", call.name, err)
		}
	}

	// A registered project slug passes validation (matching notes may be
	// empty — that is a legitimate empty result, distinct from invalid).
	writeAppFixture(t, filepath.Join(root, ".pinax", "projects.json"),
		`{"schema_version":"pinax.projects.v1","projects":[{"slug":"proj-1","name":"Project One","notes_prefix":"notes/","created_at":"2026-09-01T10:00:00Z"}]}`)
	if _, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, Query: ReaderPageQuery{ProjectID: "proj-1"}, CursorSecret: readerCursorSecret()}); err != nil {
		t.Fatalf("registered project filter err = %v", err)
	}
}

func TestReaderAccessURIRevisionInteropsWithLegacyResolver(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)

	// The legacy revision algorithm hashes the FULL file (frontmatter
	// included); the reader URI/card revision must equal it.
	raw, err := os.ReadFile(filepath.Join(root, "notes", "alpha.md"))
	if err != nil {
		t.Fatal(err)
	}
	legacyRevision := collaborationRevision(string(raw))
	notes, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	list := notes.Data.(map[string]any)["notes"].(ReaderNoteList)
	var card ReaderNoteCard
	for _, item := range list.Items {
		if item.ObjectID == "note_alpha" {
			card = item
		}
	}
	if card.Revision != legacyRevision {
		t.Fatalf("reader revision %q != legacy collaborationRevision %q", card.Revision, legacyRevision)
	}
	if card.URI != "pinax://note/note_alpha?v="+legacyRevision {
		t.Fatalf("reader URI = %q", card.URI)
	}
	// A legacy-side consumer handing the same revision to the reader
	// resolves: cross-interface round-trip.
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: legacyRevision, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("document with legacy revision: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if page.Revision != legacyRevision || !page.Complete {
		t.Fatalf("page revision=%q complete=%v", page.Revision, page.Complete)
	}
}

func TestReaderAccessIndexUntouchedAndStaleSurfaced(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")

	readStates := func() (string, string) {
		if _, err := svc.ReaderSource(context.Background(), ReaderSourceRequest{VaultPath: root, Session: session}); err != nil {
			t.Fatalf("source: %v", err)
		}
		notesProjection, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, CursorSecret: readerCursorSecret()})
		if err != nil {
			t.Fatalf("notes: %v", err)
		}
		list := notesProjection.Data.(map[string]any)["notes"].(ReaderNoteList)
		searchProjection, err := svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: "needle", CursorSecret: readerCursorSecret()})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		results := searchProjection.Data.(map[string]any)["search"].(ReaderSearchResults)
		return list.IndexState.Status, results.IndexState.Status
	}

	// Fresh index: ready surfaced in both list and search, and the reader
	// leaves the index file byte-identical.
	before := readerIndexDigest(t, indexPath)
	listState, searchState := readStates()
	if listState != "ready" || searchState != "ready" {
		t.Fatalf("fresh states = %q/%q", listState, searchState)
	}
	if after := readerIndexDigest(t, indexPath); after != before {
		t.Fatal("reader writes touched the index file")
	}

	// Edit a note WITHOUT refreshing: stale is explicit in the responses
	// (no opaque-watermark decoding needed) and still no index writes.
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"), readerNoteFixture("note_alpha", "Alpha 设计", "design", "active", "proj-1", "2026-09-01T10:00:00+00:00", "2026-09-10T10:00:00+00:00", "Alpha contains needle marker one. EDITED needle."))
	listState, searchState = readStates()
	if listState != "stale" || searchState != "stale" {
		t.Fatalf("stale states = %q/%q", listState, searchState)
	}
	if after := readerIndexDigest(t, indexPath); after != before {
		t.Fatal("reader writes touched the index file on stale reads")
	}
}

func readerIndexDigest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestReaderAccessPartialIndexSurfacedNotFailed(t *testing.T) {
	t.Parallel()
	root := readerTestVault(t, true)
	// Real partial shape: note record present, text projection missing.
	if err := noteindex.RemoveNoteTextForTest(root, "notes/alpha.md"); err != nil {
		t.Fatalf("remove text: %v", err)
	}
	svc := NewService()
	session := readerTestSession(ReaderScopeMetadata, ReaderScopeBody)
	notesProjection, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("notes on partial index must succeed, got %v", err)
	}
	list := notesProjection.Data.(map[string]any)["notes"].(ReaderNoteList)
	if list.IndexState.Status != "partial" {
		t.Fatalf("notes index_state = %q, want partial", list.IndexState.Status)
	}
	if list.IndexState.Watermark == "" {
		t.Fatal("partial state lost the watermark")
	}
	searchProjection, err := svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: "needle", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("search on partial index must succeed, got %v", err)
	}
	results := searchProjection.Data.(map[string]any)["search"].(ReaderSearchResults)
	if results.IndexState.Status != "partial" {
		t.Fatalf("search index_state = %q, want partial", results.IndexState.Status)
	}
	// Source still resolves with the same partial health.
	sourceProjection, err := svc.ReaderSource(context.Background(), ReaderSourceRequest{VaultPath: root, Session: session})
	if err != nil {
		t.Fatalf("source on partial index: %v", err)
	}
	source := sourceProjection.Data.(map[string]any)["source"].(ReaderSourceDescriptor)
	if source.IndexState.Status != "partial" {
		t.Fatalf("source index_state = %q, want partial", source.IndexState.Status)
	}
}
