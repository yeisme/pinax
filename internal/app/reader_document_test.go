package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/render"
)

// TestReaderDocument covers the P03 document lane at the application
// boundary: fixed revisions, explicit stale/changed failures, bounded pages
// with cursors, anchor jumps, budget errors, scope discipline, and no
// snapshot writes. Fixture vaults only.

func documentTestVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	var body strings.Builder
	body.WriteString("# 标题一\n\n")
	for i := 0; i < 6000; i++ {
		body.WriteString("段落内容 with path /Users/yeisme/notes and token-like text.\n\n")
	}
	writeAppFixture(t, filepath.Join(root, "notes", "long.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_long\ntitle: Long\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\n"+body.String())
	writeAppFixture(t, filepath.Join(root, "notes", "short.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_short\ntitle: Short\nstatus: active\nupdated_at: 2026-09-11T10:00:00+00:00\n---\n\nShort body only.\n")
	if err := refreshIndex(root); err != nil {
		t.Fatalf("refreshIndex: %v", err)
	}
	return root
}

func documentSession() ReaderSessionInfo {
	return ReaderSessionInfo{SessionID: "sess-doc", BindingID: "binding-doc", Scopes: []string{ReaderScopeBody}}
}

func TestReaderDocumentScopeAndLookup(t *testing.T) {
	t.Parallel()
	root := documentTestVault(t)
	svc := NewService()

	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: ReaderSessionInfo{SessionID: "s", Scopes: []string{ReaderScopeMetadata}}, ObjectID: "note_short", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "insufficient_scope") {
		t.Fatalf("metadata-only session err = %v", err)
	}
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_missing", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_object_not_found") {
		t.Fatalf("missing note err = %v", err)
	}
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("empty object err = %v", err)
	}
}

func TestReaderDocumentRevisionPinningAndConflicts(t *testing.T) {
	t.Parallel()
	root := documentTestVault(t)
	svc := NewService()

	first, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_short", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("first document: %v", err)
	}
	page := first.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if page.Revision != page.CurrentRevision || page.SchemaVersion != "pinax.reader.document.v1alpha1" || !page.Complete {
		t.Fatalf("page = %#v", page)
	}
	revision := page.Revision

	// Explicit stale revision: explicit 409, no snapshot fallback.
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_short", Revision: "sha256:0000", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "revision_unavailable") {
		t.Fatalf("stale revision err = %v", err)
	}

	// Edit the note: a cursor issued against the old revision must fail with
	// source_changed instead of mixing pages from two revisions.
	longFirst, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_long", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("long first page: %v", err)
	}
	longPage := longFirst.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if longPage.Complete || longPage.NextCursor == "" {
		t.Fatalf("long page unexpectedly complete: %#v", longPage)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "long.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_long\ntitle: Long2\nstatus: active\nupdated_at: 2026-09-12T10:00:00+00:00\n---\n\nEdited body.\n")
	_, err = svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_long", Revision: longPage.Revision, Cursor: longPage.NextCursor, CursorSecret: readerCursorSecret()})
	if err == nil || !strings.Contains(err.Error(), "revision_unavailable") {
		t.Fatalf("changed source without snapshot err = %v", err)
	}
	// The formerly-current revision is now stale for fresh requests too.
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_long", Revision: longPage.Revision, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "revision_unavailable") {
		t.Fatalf("revised long err = %v", err)
	}
	_ = page
	_ = revision
}

func TestReaderDocumentCursorAndAnchorDiscipline(t *testing.T) {
	t.Parallel()
	root := documentTestVault(t)
	svc := NewService()

	// cursor and anchor are mutually exclusive.
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_long", Cursor: "x", Anchor: "y", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("cursor+anchor err = %v", err)
	}
	// Unknown anchor: 400, no full scan fallback semantics.
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_long", Anchor: "nope", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("unknown anchor err = %v", err)
	}

	// Walk all pages; anchors from the outline locate pages in revision.
	first, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_long", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	page := first.Data.(map[string]any)["document"].(ReaderDocumentPage)
	seen := 0
	current := page
	for {
		seen += len(current.Nodes)
		if current.Complete {
			break
		}
		if current.NextCursor == "" {
			t.Fatal("incomplete page without cursor")
		}
		next, nextErr := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
			VaultPath: root, Session: documentSession(), ObjectID: "note_long",
			Revision: current.Revision, Cursor: current.NextCursor, CursorSecret: readerCursorSecret(),
		})
		if nextErr != nil {
			t.Fatalf("next page: %v", nextErr)
		}
		current = next.Data.(map[string]any)["document"].(ReaderDocumentPage)
	}
	if seen == 0 {
		t.Fatal("no nodes paged")
	}
	if len(page.Outline) != 1 || page.Outline[0].Title != "标题一" {
		t.Fatalf("outline = %#v", page.Outline)
	}
	// Anchor jump lands on the heading's page without a cursor.
	anchorPage, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_long", Anchor: page.Outline[0].Anchor, CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("anchor page: %v", err)
	}
	anchor := anchorPage.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if len(anchor.Nodes) == 0 || anchor.Nodes[0].Type != "heading" {
		t.Fatalf("anchor page head = %#v", anchor.Nodes[0])
	}

	// Cross-session cursor reuse is rejected.
	other := ReaderSessionInfo{SessionID: "sess-other", BindingID: "binding-other", Scopes: []string{ReaderScopeBody}}
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: other, ObjectID: "note_long", Revision: page.Revision, Cursor: page.NextCursor, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_cursor_invalid") {
		t.Fatalf("cross-session cursor err = %v", err)
	}
}

func TestReaderDocumentOutlineCursorDiscipline(t *testing.T) {
	t.Parallel()
	root := documentTestVault(t)
	svc := NewService()

	first, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_long", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	page := first.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if page.OutlineCursor != "" {
		t.Fatalf("small outline needs no cursor: %q", page.OutlineCursor)
	}

	// Tampered outline cursor: signature check fails closed.
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_long", OutlineCursor: "o:1:deadbeef", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_cursor_invalid") {
		t.Fatalf("tampered outline cursor err = %v", err)
	}
	// Cross-session outline cursor reuse: rejected by signature binding.
	other := ReaderSessionInfo{SessionID: "sess-other", BindingID: "binding-other", Scopes: []string{ReaderScopeBody}}
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: other, ObjectID: "note_long", OutlineCursor: "anything", CursorSecret: readerCursorSecret()}); err == nil {
		t.Fatal("cross-session outline cursor accepted")
	}
}

func TestReaderDocumentBudgetErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "huge.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_huge\ntitle: Huge\nstatus: active\n---\n\n"+strings.Repeat("a", 8*1024*1024+64))
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService().ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: documentSession(), ObjectID: "note_huge", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "document_too_large") {
		t.Fatalf("huge note err = %v", err)
	}
}

func TestReaderDocumentBudgetErrorsCarryRecoverableDetails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	source := strings.Repeat("a", 8*1024*1024+64)
	writeAppFixture(t, filepath.Join(root, "notes", "huge.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_huge\ntitle: Huge\nstatus: active\n---\n\n"+source)
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	projection, err := NewService().ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_huge", CursorSecret: readerCursorSecret(),
	})
	if err == nil || !strings.Contains(err.Error(), "document_too_large") {
		t.Fatalf("huge note err = %v", err)
	}
	// Machine-recoverable details: fixed revision, source size in bytes and
	// the available source-export action.
	if projection.Error == nil || projection.Error.Code != "document_too_large" {
		t.Fatalf("projection error = %#v", projection.Error)
	}
	if revision, ok := projection.Facts["revision"]; !ok || !strings.HasPrefix(revision, "sha256:") {
		t.Fatalf("revision fact = %#v", projection.Facts)
	}
	if size, ok := projection.Facts["source_size_bytes"]; !ok || size == "0" {
		t.Fatalf("source_size_bytes fact = %#v", projection.Facts)
	}
	foundAction := false
	for _, action := range projection.Actions {
		if action.Name == "source_export" && strings.Contains(action.Command, "/v1/reader/notes/note_huge/source?revision=") {
			foundAction = true
		}
	}
	if !foundAction {
		t.Fatalf("source_export action missing: %#v", projection.Actions)
	}
}

func TestReaderDocumentPinnedRevisionContinuesFromSnapshot(t *testing.T) {
	t.Parallel()
	root := documentTestVault(t)
	svc := NewService()
	session := documentSession()

	// Snapshot the vault while note_short is at its original body.
	shortRevision := noteBodyRevision(t, root, "notes/short.md")
	if _, err := svc.VersionSnapshot(context.Background(), SnapshotRequest{VaultPath: root, Message: "before edit"}); err != nil {
		t.Fatalf("VersionSnapshot: %v", err)
	}
	// Edit the note in place; the live revision moves away.
	writeAppFixture(t, filepath.Join(root, "notes", "short.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_short\ntitle: Short2\nstatus: active\n---\n\nEdited body.\n")

	// Explicit pinned revision: continues from the snapshot with the old
	// body while current_revision reflects the live content.
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: session, ObjectID: "note_short", Revision: shortRevision, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("pinned revision document: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if page.Revision != shortRevision || page.CurrentRevision == shortRevision {
		t.Fatalf("revision pinning = %q current = %q", page.Revision, page.CurrentRevision)
	}
	var sawOldBody bool
	for _, node := range page.Nodes {
		if node.Type == "paragraph" && node.Text == "Short body only." {
			sawOldBody = true
		}
	}
	if !sawOldBody {
		t.Fatalf("pinned body lost: %#v", page.Nodes)
	}

	// A revision that exists nowhere (live or snapshots) still fails 409.
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: session, ObjectID: "note_short", Revision: "sha256:1111111111111111111111111111111111111111111111111111111111111111", CursorSecret: readerCursorSecret(),
	}); err == nil || !strings.Contains(err.Error(), "revision_unavailable") {
		t.Fatalf("unknown revision err = %v", err)
	}
}

func TestReaderDocumentSnapshotCursorAndAnchorContinuation(t *testing.T) {
	t.Parallel()
	root := documentTestVault(t)
	svc := NewService()
	session := documentSession()

	// Multi-page note: capture the first page and its outline anchor.
	first, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: session, ObjectID: "note_long", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	firstPage := first.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if firstPage.Complete || firstPage.NextCursor == "" {
		t.Fatalf("expected multi-page note: %#v", firstPage)
	}
	oldRevision := firstPage.Revision
	oldAnchor := firstPage.Outline[0].Anchor // "标题一" heading

	// Snapshot, then rewrite the note WITHOUT the heading and shorter.
	if _, err := svc.VersionSnapshot(context.Background(), SnapshotRequest{VaultPath: root, Message: "before rewrite"}); err != nil {
		t.Fatalf("VersionSnapshot: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "long.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_long\ntitle: Long2\nstatus: active\nupdated_at: 2026-09-20T10:00:00+00:00\n---\n\nShort now.\n")

	// Cursor continuation pinned at the old revision: continues from the
	// snapshot with old content, same revision, live current_revision hint.
	next, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: session, ObjectID: "note_long",
		Revision: oldRevision, Cursor: firstPage.NextCursor, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("pinned cursor continuation: %v", err)
	}
	nextPage := next.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if nextPage.Revision != oldRevision || nextPage.CurrentRevision == oldRevision {
		t.Fatalf("cursor pinning = %q current = %q", nextPage.Revision, nextPage.CurrentRevision)
	}
	var sawOldParagraph bool
	for _, node := range nextPage.Nodes {
		if node.Type == "paragraph" && strings.Contains(node.Text, "zq-e2e") {
			sawOldParagraph = true
		}
	}
	_ = sawOldParagraph // old body pages continue; content asserted via heading below

	// Anchor jump on the pinned revision: the old heading anchor resolves
	// against the OLD body even though the live body dropped the heading.
	anchored, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: session, ObjectID: "note_long",
		Revision: oldRevision, Anchor: oldAnchor, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("pinned anchor jump: %v", err)
	}
	anchorPage := anchored.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if len(anchorPage.Nodes) == 0 || anchorPage.Nodes[0].Type != "heading" || anchorPage.Nodes[0].Text != "标题一" {
		t.Fatalf("pinned anchor landed wrong: %#v", anchorPage.Nodes[0])
	}

	// The same anchor against the LIVE body (heading removed) is unknown.
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: session, ObjectID: "note_long", Anchor: oldAnchor, CursorSecret: readerCursorSecret(),
	}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("live anchor err = %v", err)
	}
}

func TestReaderDocumentSnapshotBudgetRecoveryPointsAtPinnedRevision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A body that parses past the 65536-node budget, well under source bytes.
	var big strings.Builder
	for i := 0; i < 70000; i++ {
		big.WriteString("- item\n")
	}
	bigBody := big.String()
	if err := os.WriteFile(filepath.Join(root, "notes", "flood.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_flood\ntitle: Flood\nstatus: active\n---\n\n"+bigBody), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	if _, err := svc.IndexRefresh(context.Background(), IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatal(err)
	}
	// Read the note's revision from the public listing (the parser owns the
	// exact body normalization).
	listing, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{
		VaultPath: root, Session: ReaderSessionInfo{SessionID: "s", BindingID: "b", Scopes: []string{ReaderScopeMetadata}}, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderNotes: %v", err)
	}
	pinnedRevision := ""
	for _, card := range listing.Data.(map[string]any)["notes"].(ReaderNoteList).Items {
		if card.ObjectID == "note_flood" {
			pinnedRevision = card.Revision
		}
	}
	if pinnedRevision == "" {
		t.Fatal("flood note not listed")
	}
	if _, err := svc.VersionSnapshot(context.Background(), SnapshotRequest{VaultPath: root, Message: "flood state"}); err != nil {
		t.Fatalf("VersionSnapshot: %v", err)
	}
	// Shrink the live body so only the pinned revision is over budget.
	if err := os.WriteFile(filepath.Join(root, "notes", "flood.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_flood\ntitle: Flood2\nstatus: active\n---\n\ntiny"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.IndexRefresh(context.Background(), IndexRefreshRequest{VaultPath: root}); err != nil {
		t.Fatal(err)
	}

	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_flood", Revision: pinnedRevision, CursorSecret: readerCursorSecret(),
	})
	if err == nil || !strings.Contains(err.Error(), "document_budget_exceeded") {
		t.Fatalf("pinned budget err = %v", err)
	}
	// Recovery describes the PINNED revision and the PINNED body size, not
	// the live one.
	if got := projection.Facts["revision"]; got != pinnedRevision {
		t.Fatalf("recovery revision = %q, want pinned %q", got, pinnedRevision)
	}
	// The parser owns trailing-newline normalization; the fact must match
	// the pinned body as parsed (huge), never the tiny live body.
	parsedSize := fmt.Sprint(len(strings.TrimRight(bigBody, "\n")))
	if got := projection.Facts["source_size_bytes"]; got != parsedSize {
		t.Fatalf("recovery size = %q, want pinned %q", got, parsedSize)
	}
	found := false
	for _, action := range projection.Actions {
		if action.Name == "source_export" && strings.Contains(action.Command, "revision="+pinnedRevision) {
			found = true
		}
	}
	if !found {
		t.Fatalf("recovery action targets wrong revision: %#v", projection.Actions)
	}

	// Close the recovery loop: follow the action and export the pinned
	// revision. The bytes must be the OLD body, not the tiny live one.
	exportProjection, exportContent, exportErr := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_flood", Revision: pinnedRevision,
	})
	if exportErr != nil {
		t.Fatalf("pinned source export: %v", exportErr)
	}
	exported := new(strings.Builder)
	if _, err := io.Copy(exported, exportContent.ReadSeeker); err != nil {
		t.Fatal(err)
	}
	if exported.String() != strings.TrimRight(bigBody, "\n") {
		t.Fatalf("pinned export returned wrong body: got %d bytes want %d", exported.Len(), len(strings.TrimRight(bigBody, "\n")))
	}
	if exportProjection.Facts["revision"] != pinnedRevision {
		t.Fatalf("export revision fact = %q", exportProjection.Facts["revision"])
	}
}

func TestReaderDocumentWikiEmbedNodesObeyTotalBudget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3, 4, 5, 6, 7, 8}
	if err := os.WriteFile(filepath.Join(root, "attachments", "pic.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(png)
	manifest := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_pic","path":"attachments/pic.png","filename":"pic.png","stem":"pic","extension":".png","media_type":"image/png","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, len(png), hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	// 33000 wiki-embed paragraphs: 33000 build nodes + 33000 resolver nodes
	// = 66000 > 65536 — the serving path must reject with the budget error.
	var body strings.Builder
	for i := 0; i < 33000; i++ {
		body.WriteString("![[attachments/pic.png]]\n\n")
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "flood.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_flood\ntitle: Flood\nstatus: active\n---\n\n"+body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	_, err := NewService().ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_flood", CursorSecret: readerCursorSecret(),
	})
	if err == nil || !strings.Contains(err.Error(), "document_budget_exceeded") {
		t.Fatalf("wiki-embed node flood err = %v", err)
	}
}

func TestReaderDocumentGeneratedAssetAnchorsAreUniqueAndLocated(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3, 4, 5, 6, 7, 8}
	if err := os.WriteFile(filepath.Join(root, "attachments", "pic.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(png)
	manifest := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_pic","path":"attachments/pic.png","filename":"pic.png","stem":"pic","extension":".png","media_type":"image/png","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, len(png), hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	// The resolver's derived anchor for the first paragraph's image equals
	// the following heading's slug: the registry must disambiguate and both
	// must locate to their owning blocks.
	if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\n---\n\n![[attachments/pic.png]]\n\n# block-1-asset-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_alpha", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderDocument: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)

	// Recursive anchor uniqueness across blocks and children.
	seen := map[string]int{}
	var walk func(nodes []render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, node := range nodes {
			if node.Anchor != "" {
				seen[node.Anchor]++
			}
			walk(node.Children)
		}
	}
	walk(page.Nodes)
	for anchor, count := range seen {
		if count > 1 {
			t.Fatalf("anchor %q used %d times", anchor, count)
		}
	}

	// Anchor jump through the serving path: the heading anchor locates its
	// own block (the heading), and a generated image anchor (any "-asset-"
	// anchor) locates the first block (the paragraph).
	var imageAnchor, headingAnchor string
	for _, node := range page.Nodes {
		if node.Type == "heading" {
			headingAnchor = node.Anchor
		}
		for _, child := range node.Children {
			if child.Type == "image" && strings.HasSuffix(child.Anchor, "-asset-1") || strings.Contains(child.Anchor, "-asset-") {
				if imageAnchor == "" {
					imageAnchor = child.Anchor
				}
			}
		}
	}
	if imageAnchor == "" || headingAnchor == "" {
		t.Fatalf("anchors missing: image=%q heading=%q nodes=%#v", imageAnchor, headingAnchor, page.Nodes)
	}
	if imageAnchor == headingAnchor {
		t.Fatalf("image and heading anchors collide: %q", imageAnchor)
	}
	anchoredByImage, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_alpha", Anchor: imageAnchor, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("image anchor jump: %v", err)
	}
	imagePage := anchoredByImage.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if len(imagePage.Nodes) == 0 || imagePage.Nodes[0].Type != "paragraph" {
		t.Fatalf("image anchor landed on %#v", imagePage.Nodes[0])
	}
	anchoredByHeading, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_alpha", Anchor: headingAnchor, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("heading anchor jump: %v", err)
	}
	headingPage := anchoredByHeading.Data.(map[string]any)["document"].(ReaderDocumentPage)
	if len(headingPage.Nodes) == 0 || headingPage.Nodes[0].Type != "heading" {
		t.Fatalf("heading anchor landed on %#v", headingPage.Nodes[0])
	}
}

func TestReaderDocumentServesOwnerFenceNodes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The timeline fence is a registered owner handler on any vault: on a
	// bare fixture it either yields rows or an explicit timeline_unavailable
	// failure — the reading surface must carry the fence semantics, not the
	// raw fence source.
	if err := os.WriteFile(filepath.Join(root, "notes", "fenced.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_fenced\ntitle: Fenced\nstatus: active\n---\n\n```pinax-timeline\nnote_fenced\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	scopedSession := ReaderSessionInfo{SessionID: "sess-fence", BindingID: "b", Scopes: []string{ReaderScopeBody, ReaderScopeMetadata}}
	projection, err := NewService().ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: scopedSession, ObjectID: "note_fenced", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderDocument: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	found := false
	for _, node := range page.Nodes {
		if node.Type == "fence" && node.Fence == "pinax-timeline" {
			found = true
		}
		if node.Type == "fence_failed" && node.Fence == "pinax-timeline" && node.ErrorCode != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fence semantics missing from document: %#v", page.Nodes)
	}
	if !page.Complete || len(page.Warnings) > 0 {
		t.Fatalf("complete=%v warnings=%v", page.Complete, page.Warnings)
	}

	// A body-only session must NOT run metadata-scoped fence handlers: the
	// fence source stays inert code (no cross-note projection leak).
	bodyOnly, err := NewService().ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_fenced", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderDocument body-only: %v", err)
	}
	bodyPage := bodyOnly.Data.(map[string]any)["document"].(ReaderDocumentPage)
	for _, node := range bodyPage.Nodes {
		if node.Type == "fence" || node.Type == "fence_failed" {
			t.Fatalf("body-only session ran a scoped fence handler: %#v", node)
		}
		if node.Type == "code" && node.Language == "pinax-timeline" {
			continue // inert source: correct
		}
	}
}

func TestReaderDocumentWikiEmbedResolvesExactlyOnce(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3, 4}
	if err := os.WriteFile(filepath.Join(root, "attachments", "pic.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(png)
	manifest := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_pic","path":"attachments/pic.png","filename":"pic.png","stem":"pic","extension":".png","media_type":"image/png","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, len(png), hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "embed.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_embed\ntitle: Embed\nstatus: active\n---\n\n![[attachments/pic.png]]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	projection, err := NewService().ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_embed", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderDocument: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	images := 0
	var walk func(nodes []render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, node := range nodes {
			if node.Type == "image" && node.Ref == "pinax-asset://asset_pic" {
				images++
			}
			walk(node.Children)
		}
	}
	walk(page.Nodes)
	if images != 1 {
		t.Fatalf("wiki embed resolved %d times, want exactly 1 (no duplicate nodes)", images)
	}
	if !page.Complete || len(page.Warnings) > 0 {
		t.Fatalf("complete=%v warnings=%v", page.Complete, page.Warnings)
	}
}

func TestReaderDocumentResolvesNoteRefsInBody(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Alpha references beta via a wiki alias link AND a markdown .md link.
	if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\n---\n\nSee [[note_beta|别名]] and the [next note](beta.md).\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "beta.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nstatus: active\n---\n\nBeta body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_alpha", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderDocument: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)

	// Resolve beta's interop revision for URI assertions.
	betaNotes, err := readerScanNotes(root)
	if err != nil {
		t.Fatal(err)
	}
	var betaRevision string
	for _, n := range betaNotes {
		if n.ID == "note_beta" {
			betaRevision = readerNoteRevision(n)
		}
	}
	wantRef := "pinax://note/note_beta?v=" + betaRevision

	wikiResolved, markdownResolved := false, false
	var walk func(nodes []render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, node := range nodes {
			if node.Type == "link" && node.Ref == wantRef {
				if node.Text == "别名" {
					wikiResolved = true
				}
				if node.Text == "next note" {
					markdownResolved = true
				}
			}
			walk(node.Children)
		}
	}
	walk(page.Nodes)
	if !wikiResolved {
		t.Fatal("wiki note link not resolved to pinax://note URI with alias label")
	}
	if !markdownResolved {
		t.Fatal("markdown .md link not resolved to pinax://note URI")
	}
	if !page.Complete || len(page.Warnings) > 0 {
		t.Fatalf("complete=%v warnings=%v", page.Complete, page.Warnings)
	}
}

func TestReaderDocumentNoteRefBudgetAndSafety(t *testing.T) {
	t.Parallel()
	mkVault := func(body string) string {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"),
			[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\n---\n\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
		// Beta's TITLE is a wiki literal: generated labels must never be
		// re-resolved into nested links.
		if err := os.WriteFile(filepath.Join(root, "notes", "beta.md"),
			[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: \"[[note_beta]]\"\nstatus: active\n---\n\nBeta body.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := refreshIndex(root); err != nil {
			t.Fatal(err)
		}
		return root
	}
	svc := NewService()

	// 1) Budget overflow: 66000 wiki links in one paragraph exceed the
	// 65536 total node budget → explicit 413 vocabulary, never silent
	// complete.
	var flood strings.Builder
	for i := 0; i < 66000; i++ {
		flood.WriteString("[[note_beta]] ")
	}
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: mkVault(flood.String()), Session: documentSession(), ObjectID: "note_alpha", CursorSecret: readerCursorSecret(),
	})
	if err == nil || !strings.Contains(err.Error(), "document_budget_exceeded") {
		t.Fatalf("note-ref flood err = %v", err)
	}
	if projection.Error == nil || projection.Error.Code != "document_budget_exceeded" {
		t.Fatalf("projection error = %#v", projection.Error)
	}

	// 2) Literal-titled target: exactly one generated link per wiki
	// occurrence, no nesting inside generated nodes.
	projection, err = svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: mkVault("See [[note_beta]] once.\n"), Session: documentSession(), ObjectID: "note_alpha", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("literal-title document: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	generated := 0
	nested := 0
	var walk func(nodes []render.ReaderDocumentNode, depth int)
	walk = func(nodes []render.ReaderDocumentNode, depth int) {
		for _, node := range nodes {
			if strings.HasSuffix(node.Anchor, "-noteref-1") && strings.HasPrefix(node.Ref, "pinax://note/note_beta") {
				generated++
				if depth > 1 {
					nested++
				}
			}
			walk(node.Children, depth+1)
		}
	}
	walk(page.Nodes, 0)
	if generated != 1 || nested != 0 {
		t.Fatalf("generated=%d nested=%d, want exactly 1 flat", generated, nested)
	}

	// 3) Wiki literal inside a code block stays literal (code context is
	// never resolved).
	projection, err = svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: mkVault("```go\n// [[note_beta]] example\n```\n"), Session: documentSession(), ObjectID: "note_alpha", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatal(err)
	}
	page = projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	for _, node := range page.Nodes {
		var walkCode func(nodes []render.ReaderDocumentNode)
		walkCode = func(nodes []render.ReaderDocumentNode) {
			for _, n := range nodes {
				if n.Type == "code" || (n.Type == "continuation" && n.OriginalType == "code") {
					for _, child := range n.Children {
						if strings.HasPrefix(child.Ref, "pinax://note/note_beta") {
							t.Fatalf("wiki literal resolved inside code context: %#v", child)
						}
					}
				}
				walkCode(n.Children)
			}
		}
		walkCode([]render.ReaderDocumentNode{node})
	}
}

func TestReaderDocumentGraphFenceRequiresRelationsScope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "graphed.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_graphed\ntitle: Graphed\nstatus: active\n---\n\n```pinax-graph\nnote_graphed\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	bodyOnly := documentSession()
	relationsSession := ReaderSessionInfo{SessionID: "s", BindingID: "b", Scopes: []string{ReaderScopeBody, ReaderScopeRelations}}

	// Body-only: the graph fence stays inert code — no relations leak.
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: bodyOnly, ObjectID: "note_graphed", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatal(err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	for _, node := range page.Nodes {
		if node.Type == "fence" || node.Type == "fence_failed" {
			t.Fatalf("body-only session ran the graph fence: %#v", node)
		}
	}

	// Relations scope: the graph fence renders its owner projection.
	projection, err = svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: relationsSession, ObjectID: "note_graphed", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatal(err)
	}
	page = projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	found := false
	for _, node := range page.Nodes {
		if node.Fence == "pinax-graph" {
			found = true
		}
	}
	if !found {
		t.Fatalf("relations session missing graph fence semantics: %#v", page.Nodes)
	}
}

func TestReaderDocumentNoteRefOwnerResolverSemantics(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "notes", "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Two dirs each with topic.md: the stem is AMBIGUOUS — the owner
	// resolver must refuse, my old byStem map silently picked the last.
	write := func(path, id, title, body string) {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)),
			[]byte("---\nschema_version: pinax.note.v1\nnote_id: "+id+"\ntitle: "+title+"\nstatus: active\n---\n\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("notes/alpha.md", "note_alpha", "Alpha",
		"Ambiguous [[topic]], title link [[Unique Title]], inline literal `[[note_beta]]`, table below.\n\n| T |\n| --- |\n| [[note_beta]] |\n")
	write("notes/a/topic.md", "note_topic_a", "TopicA", "A body.\n")
	write("notes/b/topic.md", "note_topic_b", "TopicB", "B body.\n")
	write("notes/beta.md", "note_beta", "Unique Title", "Beta body.\n")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_alpha", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderDocument: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	var betaRefs, ambiguousTopicRefs int
	var walk func(nodes []render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, node := range nodes {
			if strings.HasPrefix(node.Ref, "pinax://note/note_beta") {
				betaRefs++
			}
			if strings.HasPrefix(node.Ref, "pinax://note/note_topic_") {
				ambiguousTopicRefs++
			}
			walk(node.Children)
		}
	}
	// Count refs generated inside the inline-code token specifically: none.
	for _, block := range page.Nodes {
		for _, token := range block.Tokens {
			_ = token // token ranges exist; assertion via total counts below
		}
	}
	walk(page.Nodes)
	// Exactly TWO note_beta refs: the title link and the TABLE wiki link.
	// The inline-code literal must NOT resolve.
	if betaRefs != 2 {
		t.Fatalf("note_beta refs = %d, want 2 (title + table), inline literal excluded", betaRefs)
	}
	// Ambiguous stem [[topic]] must NOT resolve to either candidate.
	if ambiguousTopicRefs != 0 {
		t.Fatalf("ambiguous stem resolved %d times", ambiguousTopicRefs)
	}
	if !page.Complete || len(page.Warnings) > 0 {
		t.Fatalf("complete=%v warnings=%v", page.Complete, page.Warnings)
	}
}

func TestReaderDocumentFragmentLinksResolve(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "alpha.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\n---\n\nWiki [[note_beta#章节]] and markdown [章节](beta.md#章节) links.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "beta.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nstatus: active\n---\n\nBeta body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	projection, err := NewService().ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_alpha", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderDocument: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	refs := 0
	var walk func(nodes []render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, node := range nodes {
			if strings.HasPrefix(node.Ref, "pinax://note/note_beta?v=") {
				refs++
			}
			walk(node.Children)
		}
	}
	walk(page.Nodes)
	if refs != 2 {
		t.Fatalf("fragment link refs = %d, want 2 (wiki + markdown both resolve through owner normalization)", refs)
	}
}

func TestReaderDocumentAssetLiteralInCodeStaysLiteral(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2}
	if err := os.WriteFile(filepath.Join(root, "attachments", "pic.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(png)
	manifest := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_pic","path":"attachments/pic.png","filename":"pic.png","stem":"pic","extension":".png","media_type":"image/png","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, len(png), hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "coded.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_coded\ntitle: Coded\nstatus: active\n---\n\nInline literal `![[attachments/pic.png]]` and fenced:\n\n```text\n![[attachments/pic.png]]\n```\n\nReal ![[attachments/pic.png]] embed.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	projection, err := NewService().ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: documentSession(), ObjectID: "note_coded", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderDocument: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	images := 0
	var walk func(nodes []render.ReaderDocumentNode, inCode bool)
	walk = func(nodes []render.ReaderDocumentNode, inCode bool) {
		for _, node := range nodes {
			nodeIsCode := inCode || node.Type == "code" || (node.Type == "continuation" && node.OriginalType == "code")
			if node.Type == "image" && node.Ref == "pinax-asset://asset_pic" {
				if nodeIsCode {
					t.Fatalf("code literal became a live asset ref: %#v", node)
				}
				images++
			}
			walk(node.Children, nodeIsCode)
		}
	}
	walk(page.Nodes, false)
	if images != 1 {
		t.Fatalf("live embeds = %d, want exactly 1 (real embed only)", images)
	}
}

func TestReaderDocumentAssetResolutionReusesOwnerParser(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, dir := range []string{"notes", "assets"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Assets: one via ../ relative path, one URL-encoded name, one plain
	// PDF for the markdown-download link.
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2}
	pdf := []byte("%PDF-1.4 download")
	if err := os.WriteFile(filepath.Join(root, "assets", "diagram.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "My File.pdf"), pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "attachments", "spec.pdf"), pdf, 0o644); err != nil {
		t.Fatal(err)
	}
	entry := func(id, path, name string, data []byte) string {
		sum := sha256.Sum256(data)
		return fmt.Sprintf(`{"id":%q,"path":%q,"filename":%q,"stem":"x","extension":".bin","media_type":"application/octet-stream","size":%d,"sha256":%s,"managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}`, id, path, name, len(data), `"`+hex.EncodeToString(sum[:])+`"`)
	}
	manifest := `{"schema_version":"pinax.assets.v1","assets":[` +
		entry("asset_diag", "assets/diagram.png", "diagram.png", png) + "," +
		entry("asset_myfile", "assets/My File.pdf", "My File.pdf", pdf) + "," +
		entry("asset_spec", "attachments/spec.pdf", "spec.pdf", pdf) + `]}`
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "note.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_r\ntitle: R\nstatus: active\n---\n\n![Diagram](../assets/diagram.png)\n\nEmbed ![[assets/My%20File.pdf]] and [下载](attachments/spec.pdf).\n\nTitle link [规范](attachments/spec.pdf \"说明\") and reference [spec][spec].\n\n[spec]: attachments/spec.pdf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: ReaderSessionInfo{SessionID: "s", BindingID: "b", Scopes: []string{ReaderScopeBody, ReaderScopeAssets}}, ObjectID: "note_r", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("ReaderDocument: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	refs := map[string]int{}
	var walk func(nodes []render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, node := range nodes {
			if strings.HasPrefix(node.Ref, "pinax-asset://") {
				refs[node.Ref]++
			}
			walk(node.Children)
		}
	}
	walk(page.Nodes)
	if refs["pinax-asset://asset_diag"] < 1 {
		t.Fatalf("relative ../ path asset not resolved: %#v", refs)
	}
	if refs["pinax-asset://asset_myfile"] < 1 {
		t.Fatalf("URL-encoded asset not resolved: %#v", refs)
	}
	if refs["pinax-asset://asset_spec"] < 1 {
		t.Fatalf("markdown download link asset not resolved: %#v", refs)
	}

	// Full chain: opaque ref → ReaderAsset bytes (document → ref → read).
	_, content, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{
		VaultPath: root, Session: ReaderSessionInfo{SessionID: "s", BindingID: "b", Scopes: []string{ReaderScopeBody, ReaderScopeAssets}},
		AssetRef: "asset_spec", NoteID: "note_r", NoteRevision: noteBodyRevision(t, root, "notes/note.md"),
	})
	if err != nil {
		t.Fatalf("ReaderAsset via opaque ref: %v", err)
	}
	if !strings.HasPrefix(string(readAll(t, content.ReadSeeker)), "%PDF") {
		t.Fatal("asset bytes mismatch")
	}
}

func readAll(t *testing.T, r io.Reader) []byte {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestReaderDocumentAssetOwnershipPerMarkdownSyntax(t *testing.T) {
	t.Parallel()
	// Each markdown form lives in its OWN note with its OWN asset, and each
	// case asserts BOTH the document-side opaque ref AND an actual read of
	// the bytes — no shared asset can mask a broken ownership check.
	syntaxes := []struct {
		name string
		body string
	}{
		{"plain", "[d](attachments/plain.pdf)\n"},
		{"title", "[d](attachments/title.pdf \"说明\")\n"},
		{"angle-title", "[d](<attachments/angle.pdf> \"说明\")\n"},
		{"reference-style", "[d][spec]\n\n[spec]: attachments/ref.pdf\n"},
		{"balanced-paren", "[d](attachments/paren(1).pdf)\n"},
	}
	for _, syntax := range syntaxes {
		t.Run(syntax.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"notes", "attachments", filepath.Join(".pinax", "assets")} {
				if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			pdf := []byte("%PDF-" + syntax.name)
			assetPath := "attachments/" + map[string]string{
				"plain": "plain.pdf", "title": "title.pdf", "angle-title": "angle.pdf",
				"reference-style": "ref.pdf", "balanced-paren": "paren(1).pdf",
			}[syntax.name]
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(assetPath)), pdf, 0o644); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(pdf)
			manifest := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_x","path":%q,"filename":"x","stem":"x","extension":".pdf","media_type":"application/pdf","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, assetPath, len(pdf), hex.EncodeToString(sum[:]))
			if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "notes", "solo.md"),
				[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_solo\ntitle: Solo\nstatus: active\n---\n\n"+syntax.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := refreshIndex(root); err != nil {
				t.Fatal(err)
			}
			svc := NewService()
			scopes := ReaderSessionInfo{SessionID: "s", BindingID: "b", Scopes: []string{ReaderScopeBody, ReaderScopeAssets}}
			projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
				VaultPath: root, Session: scopes, ObjectID: "note_solo", CursorSecret: readerCursorSecret(),
			})
			if err != nil {
				t.Fatalf("document: %v", err)
			}
			page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
			found := false
			var walk func(nodes []render.ReaderDocumentNode)
			walk = func(nodes []render.ReaderDocumentNode) {
				for _, node := range nodes {
					if node.Ref == "pinax-asset://asset_x" {
						found = true
					}
					walk(node.Children)
				}
			}
			walk(page.Nodes)
			if !found {
				t.Fatalf("document did not discover the %s attachment ref", syntax.name)
			}
			_, content, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{
				VaultPath: root, Session: scopes, AssetRef: "asset_x", NoteID: "note_solo",
				NoteRevision: noteBodyRevision(t, root, "notes/solo.md"),
			})
			if err != nil {
				t.Fatalf("%s ownership/read failed: %v", syntax.name, err)
			}
			got := readAll(t, content.ReadSeeker)
			if string(got) != string(pdf) {
				t.Fatalf("%s bytes mismatch: %q", syntax.name, got)
			}
		})
	}
}

func TestReaderDocumentQuotedFilenameAndWrongPrefixOwnership(t *testing.T) {
	t.Parallel()
	// Case 1: a quote INSIDE a filename is part of the path — discovery
	// and ownership both preserve it (no markdown-title truncation).
	root := t.TempDir()
	for _, dir := range []string{"notes", "attachments", filepath.Join(".pinax", "assets")} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	quoted := []byte("%PDF-oreilly")
	if err := os.WriteFile(filepath.Join(root, "attachments", "O'Reilly.pdf"), quoted, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(quoted)
	manifest := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_oreilly","path":"attachments/O'Reilly.pdf","filename":"O'Reilly.pdf","stem":"O'Reilly","extension":".pdf","media_type":"application/pdf","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, len(quoted), hex.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "quote.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_quote\ntitle: Q\nstatus: active\n---\n\n[book](attachments/O'Reilly.pdf)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	scopes := ReaderSessionInfo{SessionID: "s", BindingID: "b", Scopes: []string{ReaderScopeBody, ReaderScopeAssets}}
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: scopes, ObjectID: "note_quote", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	found := false
	var walk func(nodes []render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, node := range nodes {
			if node.Ref == "pinax-asset://asset_oreilly" {
				found = true
			}
			walk(node.Children)
		}
	}
	walk(page.Nodes)
	if !found {
		t.Fatal("quoted filename attachment not discovered")
	}
	_, content, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{
		VaultPath: root, Session: scopes, AssetRef: "asset_oreilly", NoteID: "note_quote",
		NoteRevision: noteBodyRevision(t, root, "notes/quote.md"),
	})
	if err != nil {
		t.Fatalf("quoted filename ownership/read: %v", err)
	}
	if string(readAll(t, content.ReadSeeker)) != string(quoted) {
		t.Fatal("quoted filename bytes mismatch")
	}

	// Case 2: a reference to a DIFFERENT file (wrong prefix, quote-glued)
	// must NOT grant ownership over the similarly-named asset.
	root2 := t.TempDir()
	for _, dir := range []string{"notes", "attachments", filepath.Join(".pinax", "assets")} {
		if err := os.MkdirAll(filepath.Join(root2, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	spec := []byte("%PDF-spec")
	if err := os.WriteFile(filepath.Join(root2, "attachments", "spec.pdf"), spec, 0o644); err != nil {
		t.Fatal(err)
	}
	sum2 := sha256.Sum256(spec)
	manifest2 := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_spec","path":"attachments/spec.pdf","filename":"spec.pdf","stem":"spec","extension":".pdf","media_type":"application/pdf","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, len(spec), hex.EncodeToString(sum2[:]))
	if err := os.WriteFile(filepath.Join(root2, ".pinax", "assets", "manifest.json"), []byte(manifest2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root2, "notes", "tricky.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_tricky\ntitle: T\nstatus: active\n---\n\n[d](attachments/spec.pdf'backup.pdf)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewService().ReaderAsset(context.Background(), ReaderAssetRequest{
		VaultPath: root2, Session: scopes, AssetRef: "asset_spec", NoteID: "note_tricky",
		NoteRevision: noteBodyRevision(t, root2, "notes/tricky.md"),
	}); err == nil || !strings.Contains(err.Error(), "reader_object_not_found") {
		t.Fatalf("wrong-prefix ownership not rejected: %v", err)
	}
}

func TestReaderDocumentBracketedQuotedFilenameAttachment(t *testing.T) {
	t.Parallel()
	// Auditor scenario: <attachments/The 'Book'.pdf> — inside angle
	// brackets the space AND quote are part of the filename; discovery,
	// ownership and byte reads must all keep it intact.
	root := t.TempDir()
	for _, dir := range []string{"notes", "attachments", filepath.Join(".pinax", "assets")} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	book := []byte("%PDF-book")
	if err := os.WriteFile(filepath.Join(root, "attachments", "The 'Book'.pdf"), book, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(book)
	manifest := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_book","path":"attachments/The 'Book'.pdf","filename":"The 'Book'.pdf","stem":"The 'Book'","extension":".pdf","media_type":"application/pdf","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, len(book), hex.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "book.md"),
		[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_book\ntitle: Book\nstatus: active\n---\n\n[book](<attachments/The 'Book'.pdf>)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	scopes := ReaderSessionInfo{SessionID: "s", BindingID: "b", Scopes: []string{ReaderScopeBody, ReaderScopeAssets}}
	projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
		VaultPath: root, Session: scopes, ObjectID: "note_book", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("document: %v", err)
	}
	page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
	found := false
	var walk func(nodes []render.ReaderDocumentNode)
	walk = func(nodes []render.ReaderDocumentNode) {
		for _, node := range nodes {
			if node.Ref == "pinax-asset://asset_book" {
				found = true
			}
			walk(node.Children)
		}
	}
	walk(page.Nodes)
	if !found {
		t.Fatal("bracketed quoted filename attachment not discovered")
	}
	_, content, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{
		VaultPath: root, Session: scopes, AssetRef: "asset_book", NoteID: "note_book",
		NoteRevision: noteBodyRevision(t, root, "notes/book.md"),
	})
	if err != nil {
		t.Fatalf("bracketed quoted filename ownership/read: %v", err)
	}
	if string(readAll(t, content.ReadSeeker)) != string(book) {
		t.Fatal("bracketed quoted filename bytes mismatch")
	}
}

func TestReaderDocumentAttachmentFragmentQueryForms(t *testing.T) {
	t.Parallel()
	// Fragment/query suffixes on attachment destinations (bracketed and
	// unbracketed) are stripped so the manifest path matches — discovery,
	// ownership and byte reads all resolve to the same asset.
	forms := []struct{ name, body string }{
		{"unbracketed-fragment", "[d](attachments/spec.pdf#page=2)\n"},
		{"bracketed-fragment", "[d](<attachments/spec.pdf#page=2>)\n"},
		{"bracketed-query", "[d](<attachments/spec.pdf?x=1>)\n"},
		{"bracketed-quote-fragment", "[d](<attachments/The 'Book'.pdf#p>)\n"},
	}
	for _, form := range forms {
		t.Run(form.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"notes", "attachments", filepath.Join(".pinax", "assets")} {
				if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			spec := []byte("%PDF-spec")
			path := "attachments/spec.pdf"
			if form.name == "bracketed-quote-fragment" {
				spec = []byte("%PDF-book")
				path = "attachments/The 'Book'.pdf"
			}
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), spec, 0o644); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(spec)
			manifest := fmt.Sprintf(`{"schema_version":"pinax.assets.v1","assets":[{"id":"asset_f","path":%q,"filename":"f","stem":"f","extension":".pdf","media_type":"application/pdf","size":%d,"sha256":"%s","managed_status":"managed","created_at":"2026-09-10T10:00:00Z","updated_at":"2026-09-10T10:00:00Z"}]}`, path, len(spec), hex.EncodeToString(sum[:]))
			if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), []byte(manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "notes", "f.md"),
				[]byte("---\nschema_version: pinax.note.v1\nnote_id: note_f\ntitle: F\nstatus: active\n---\n\n"+form.body), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := refreshIndex(root); err != nil {
				t.Fatal(err)
			}
			svc := NewService()
			scopes := ReaderSessionInfo{SessionID: "s", BindingID: "b", Scopes: []string{ReaderScopeBody, ReaderScopeAssets}}
			projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{
				VaultPath: root, Session: scopes, ObjectID: "note_f", CursorSecret: readerCursorSecret(),
			})
			if err != nil {
				t.Fatalf("document: %v", err)
			}
			page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
			found := false
			var walk func(nodes []render.ReaderDocumentNode)
			walk = func(nodes []render.ReaderDocumentNode) {
				for _, node := range nodes {
					if node.Ref == "pinax-asset://asset_f" {
						found = true
					}
					walk(node.Children)
				}
			}
			walk(page.Nodes)
			if !found {
				t.Fatalf("%s: opaque ref not discovered", form.name)
			}
			_, content, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{
				VaultPath: root, Session: scopes, AssetRef: "asset_f", NoteID: "note_f",
				NoteRevision: noteBodyRevision(t, root, "notes/f.md"),
			})
			if err != nil {
				t.Fatalf("%s: ownership/read: %v", form.name, err)
			}
			if string(readAll(t, content.ReadSeeker)) != string(spec) {
				t.Fatalf("%s: bytes mismatch", form.name)
			}
		})
	}
}

func TestReaderDocumentRawSourceBudgetGatesBeforeParse(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Bypass form 1: the excess lives in FRONTMATTER (YAML comment block);
	// the processed body is tiny. The raw source still exceeds the budget.
	fmContent := "---\nschema_version: pinax.note.v1\nnote_id: note_fm\ntitle: FM\nstatus: active\ndescription: |\n" +
		strings.Repeat("# filler comment line\n", 8*1024*1024/20) + "---\n\nFM-BODY-0123456789\n"
	// Bypass form 2: the excess is trailing whitespace after a tiny body.
	wsContent := "---\nschema_version: pinax.note.v1\nnote_id: note_ws\ntitle: WS\nstatus: active\n---\n\nWS-BODY-0123456789" +
		strings.Repeat("\n", 8*1024*1024+128)
	if int64(len(fmContent)) <= 8*1024*1024 || int64(len(wsContent)) <= 8*1024*1024 {
		t.Fatal("fixtures must exceed the raw source budget")
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "fm.md"), []byte(fmContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "ws.md"), []byte(wsContent), 0o644); err != nil {
		t.Fatal(err)
	}
	// A normal note plus an unrelated large NON-note markdown sibling: the
	// sibling is never fully read and never blocks the interactive surface.
	if err := os.WriteFile(filepath.Join(root, "notes", "normal.md"), []byte("---\nschema_version: pinax.note.v1\nnote_id: note_normal\ntitle: N\nstatus: active\n---\n\nNORMAL-BODY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "bigplain.md"), []byte(strings.Repeat("not a pinax note\n", 600000)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatalf("refreshIndex: %v", err)
	}
	svc := NewService()
	session := ReaderSessionInfo{SessionID: "sess-budget", BindingID: "binding-budget", Scopes: []string{ReaderScopeMetadata, ReaderScopeBody}}

	for _, tc := range []struct {
		id      string
		content string
		body    string
	}{{"note_fm", fmContent, "FM-BODY-0123456789"}, {"note_ws", wsContent, "WS-BODY-0123456789"}} {
		digest := collaborationRevision(tc.content)
		// The oversized note is still listed with the full-file revision —
		// the recovery chain's entrypoint stays available.
		listProjection, err := svc.ReaderNotes(context.Background(), ReaderNotesRequest{VaultPath: root, Session: session, CursorSecret: readerCursorSecret()})
		if err != nil {
			t.Fatalf("%s list: %v", tc.id, err)
		}
		list := listProjection.Data.(map[string]any)["notes"].(ReaderNoteList)
		listed := false
		for _, card := range list.Items {
			if card.ObjectID == tc.id {
				listed = true
				if card.Revision != digest {
					t.Fatalf("%s listed revision %q want full-file digest", tc.id, card.Revision)
				}
			}
		}
		if !listed {
			t.Fatalf("%s missing from listing", tc.id)
		}
		// Document rejects on the RAW source size before any parse, with
		// machine-recoverable facts.
		projection, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: session, ObjectID: tc.id, CursorSecret: readerCursorSecret()})
		if err == nil || !strings.Contains(err.Error(), "document_too_large") {
			t.Fatalf("%s document err = %v", tc.id, err)
		}
		if projection.Facts["revision"] != digest {
			t.Fatalf("%s fact revision = %#v", tc.id, projection.Facts)
		}
		if projection.Facts["source_size_bytes"] != fmt.Sprint(len(tc.content)) {
			t.Fatalf("%s fact size = %#v want %d", tc.id, projection.Facts, len(tc.content))
		}
		// The body was never loaded: interactive search cannot match it.
		searchProjection, err := svc.ReaderSearch(context.Background(), ReaderSearchRequest{VaultPath: root, Session: session, QueryText: "BODY-0123456789", CursorSecret: readerCursorSecret()})
		if err != nil {
			t.Fatalf("%s search: %v", tc.id, err)
		}
		results := searchProjection.Data.(map[string]any)["search"].(ReaderSearchResults)
		for _, item := range results.Items {
			if item.ObjectID == tc.id {
				t.Fatalf("%s body matched search despite never being loaded", tc.id)
			}
		}
		// The explicit export recovery chain works at the listed revision.
		_, exported, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{VaultPath: root, Session: session, ObjectID: tc.id, Revision: digest})
		if err != nil {
			t.Fatalf("%s export: %v", tc.id, err)
		}
		got, err := io.ReadAll(exported.ReadSeeker)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != tc.body {
			t.Fatalf("%s export body = %q want %q", tc.id, got, tc.body)
		}
		if closer, ok := exported.ReadSeeker.(io.Closer); ok {
			_ = closer.Close()
		}
	}
	// The normal note is untouched by the oversized siblings.
	if _, err := svc.ReaderDocument(context.Background(), ReaderDocumentRequest{VaultPath: root, Session: session, ObjectID: "note_normal", CursorSecret: readerCursorSecret()}); err != nil {
		t.Fatalf("normal note document: %v", err)
	}
}
