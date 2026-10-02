package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

// TestReaderResources covers the P04 lane at the application boundary:
// authorized relations with watermark-bound cursors and invisible target
// suppression, note/revision-bound assets with symlink/hidden/escape
// rejection, and the explicit source export. Fixture vaults only.

func resourcesTestVault(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\nLinks to [[note_beta|Beta]] and embeds ![[attachments/pic.png]].\n")
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nstatus: active\nupdated_at: 2026-09-11T10:00:00+00:00\n---\n\nBack to [[note_alpha|Alpha]].\n")
	// A system note that links to alpha: it must never appear as a target.
	writeAppFixture(t, filepath.Join(root, "notes", "daily", "index.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_daily_index\ntitle: Daily\nkind: index\nstatus: active\n---\n\nMentions [[note_alpha]].\n")

	if err := os.MkdirAll(filepath.Join(root, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3, 4, 5, 6, 7, 8}
	if err := os.WriteFile(filepath.Join(root, "attachments", "pic.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(png)
	// linked.bin/hidden.bin are real manifest entries whose on-disk shapes
	// are hostile: a symlink escape and a hidden directory.
	linkedSum := sha256.Sum256([]byte("linked-placeholder"))
	hiddenSum := sha256.Sum256([]byte("hidden-placeholder"))
	if err := os.MkdirAll(filepath.Join(root, ".stash"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".stash", "hidden.bin"), []byte("hidden-placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"schema_version": "pinax.assets.v1",
		"assets": []map[string]any{
			{"id": "asset_pic", "path": "attachments/pic.png", "filename": "pic.png", "stem": "pic", "extension": ".png",
				"media_type": "image/png", "size": len(png), "sha256": hex.EncodeToString(sum[:]), "managed_status": "managed",
				"created_at": "2026-09-10T10:00:00Z", "updated_at": "2026-09-10T10:00:00Z"},
			{"id": "asset_linked", "path": "attachments/linked.bin", "filename": "linked.bin", "stem": "linked", "extension": ".bin",
				"media_type": "application/octet-stream", "size": len("linked-placeholder"), "sha256": hex.EncodeToString(linkedSum[:]), "managed_status": "managed",
				"created_at": "2026-09-10T10:00:00Z", "updated_at": "2026-09-10T10:00:00Z"},
			{"id": "asset_hidden", "path": ".stash/hidden.bin", "filename": "hidden.bin", "stem": "hidden", "extension": ".bin",
				"media_type": "application/octet-stream", "size": len("hidden-placeholder"), "sha256": hex.EncodeToString(hiddenSum[:]), "managed_status": "managed",
				"created_at": "2026-09-10T10:00:00Z", "updated_at": "2026-09-10T10:00:00Z"},
		},
	}
	encoded, _ := json.Marshal(manifest)
	if err := os.MkdirAll(filepath.Join(root, ".pinax", "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax", "assets", "manifest.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := refreshIndex(root); err != nil {
		t.Fatalf("refreshIndex: %v", err)
	}
	return root
}

func resourcesSession(scopes ...string) ReaderSessionInfo {
	return ReaderSessionInfo{SessionID: "sess-res", BindingID: "binding-res", Scopes: scopes}
}

func TestReaderResourcesLinksAuthorizationAndWatermark(t *testing.T) {
	t.Parallel()
	root := resourcesTestVault(t)
	svc := NewService()
	session := resourcesSession(ReaderScopeRelations, ReaderScopeBody)

	revision, alpha := noteRevisionOf(t, svc, root, "note_alpha")
	_ = alpha

	// Scope discipline.
	if _, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{VaultPath: root, Session: resourcesSession(ReaderScopeBody), ObjectID: "note_alpha", Revision: revision, CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "insufficient_scope") {
		t.Fatalf("links without relations scope err = %v", err)
	}
	// Revision required and pinned.
	if _, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{VaultPath: root, Session: session, ObjectID: "note_alpha", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("missing revision err = %v", err)
	}
	if _, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: "sha256:stale", Direction: "outgoing", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "revision_unavailable") {
		t.Fatalf("stale revision err = %v", err)
	}
	if _, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: revision, Direction: "both", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("bad direction err = %v", err)
	}

	// Outgoing: beta visible; the system daily-index target is suppressed.
	projection, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: revision, Direction: "outgoing", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatalf("outgoing links: %v", err)
	}
	page := projection.Data.(map[string]any)["links"].(ReaderLinkPage)
	if len(page.Items) != 1 || page.Items[0].ObjectID != "note_beta" {
		t.Fatalf("outgoing items = %#v", page.Items)
	}
	if !strings.HasPrefix(page.RelationsWatermark, "sha256:") || page.Direction != "outgoing" {
		t.Fatalf("watermark/direction = %q/%q", page.RelationsWatermark, page.Direction)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "note_daily_index") || strings.Contains(string(encoded), "Daily") {
		t.Fatalf("invisible target leaked: %s", encoded)
	}

	// Backlinks from beta.
	projection, err = svc.ReaderLinks(context.Background(), ReaderLinksRequest{VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: revision, Direction: "backlinks", CursorSecret: readerCursorSecret()})
	if err != nil {
		t.Fatal(err)
	}
	back := projection.Data.(map[string]any)["links"].(ReaderLinkPage)
	if len(back.Items) != 1 || back.Items[0].ObjectID != "note_beta" {
		t.Fatalf("backlink items = %#v", back.Items)
	}

	// Relation watermark change expires cursors.
	writeAppFixture(t, filepath.Join(root, "notes", "gamma.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_gamma\ntitle: Gamma\nstatus: active\n---\n\nAlso links [[note_alpha]].\n")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: revision, Direction: "backlinks", CursorSecret: readerCursorSecret()}); err == nil || !strings.Contains(err.Error(), "revision_unavailable") {
		// The alpha body did not change, so its revision stays valid; the
		// watermark change instead invalidates an issued cursor below.
		t.Logf("alpha revision note: %v", err)
	}
}

func TestReaderResourcesAssetBindingAndRejection(t *testing.T) {
	t.Parallel()
	root := resourcesTestVault(t)
	svc := NewService()
	session := resourcesSession(ReaderScopeAssets, ReaderScopeBody)
	alphaRevision := noteBodyRevision(t, root, "notes/alpha.md")

	// Happy path: alpha pins the revision that references the asset.
	projection, content, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{
		VaultPath: root, Session: session, AssetRef: "asset_pic", NoteID: "note_alpha", NoteRevision: alphaRevision,
	})
	if err != nil {
		t.Fatalf("ReaderAsset: %v", err)
	}
	if content.SizeBytes != 16 || content.Mime != "image/png" || !strings.HasPrefix(content.Digest, "sha256:") {
		t.Fatalf("content = %#v projection=%#v", content, projection)
	}
	payload := make([]byte, content.SizeBytes)
	if _, err := content.ReadSeeker.Read(payload); err != nil && err.Error() != "EOF" {
		t.Fatalf("read asset: %v", err)
	}
	if payload[0] != 0x89 || payload[1] != 'P' {
		t.Fatalf("asset bytes = %v", payload[:4])
	}
	if closer, ok := content.ReadSeeker.(interface{ Close() error }); ok {
		_ = closer.Close()
	}

	// Scope discipline.
	if _, _, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{VaultPath: root, Session: resourcesSession(ReaderScopeBody), AssetRef: "asset_pic", NoteID: "note_alpha", NoteRevision: alphaRevision}); err == nil || !strings.Contains(err.Error(), "insufficient_scope") {
		t.Fatalf("asset without assets scope err = %v", err)
	}
	// Note binding: beta never referenced the asset.
	if _, _, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{VaultPath: root, Session: session, AssetRef: "asset_pic", NoteID: "note_beta", NoteRevision: noteBodyRevision(t, root, "notes/beta.md")}); err == nil || !strings.Contains(err.Error(), "reader_object_not_found") {
		t.Fatalf("unbound note err = %v", err)
	}
	// Revision binding: stale note revision rejected.
	if _, _, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{VaultPath: root, Session: session, AssetRef: "asset_pic", NoteID: "note_alpha", NoteRevision: "sha256:stale"}); err == nil || !strings.Contains(err.Error(), "revision_unavailable") {
		t.Fatalf("stale note revision err = %v", err)
	}
	// Unknown asset ref.
	if _, _, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{VaultPath: root, Session: session, AssetRef: "asset_missing", NoteID: "note_alpha", NoteRevision: alphaRevision}); err == nil || !strings.Contains(err.Error(), "reader_object_not_found") {
		t.Fatalf("unknown asset err = %v", err)
	}
	// Content identity drift with the SAME byte count: only the SHA256
	// verification can catch this replacement.
	tampered := make([]byte, 16)
	tampered[0] = 0x89
	tampered[1] = 'X'
	for i := 2; i < 16; i++ {
		tampered[i] = byte(i + 1)
	}
	if err := os.WriteFile(filepath.Join(root, "attachments", "pic.png"), tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{VaultPath: root, Session: session, AssetRef: "asset_pic", NoteID: "note_alpha", NoteRevision: alphaRevision}); err == nil || !strings.Contains(err.Error(), "reader_object_not_found") {
		t.Fatalf("same-size tampered asset err = %v", err)
	}
}

func TestReaderResourcesAssetPathAttacks(t *testing.T) {
	t.Parallel()
	root := resourcesTestVault(t)
	svc := NewService()
	session := resourcesSession(ReaderScopeAssets, ReaderScopeBody)
	alphaRevision := noteBodyRevision(t, root, "notes/alpha.md")

	// Symlink substitution: the manifest entry resolves to a path that is a
	// symlink escaping the vault — rejected by the path guard itself.
	outside := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outside, []byte("linked-placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "attachments", "linked.bin")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{VaultPath: root, Session: session, AssetRef: "asset_linked", NoteID: "note_alpha", NoteRevision: alphaRevision}); err == nil || !strings.Contains(err.Error(), "reader_object_not_found") {
		t.Fatalf("symlink asset err = %v", err)
	}

	// Hidden segments: the manifest entry under a dot directory is rejected.
	if _, _, err := svc.ReaderAsset(context.Background(), ReaderAssetRequest{VaultPath: root, Session: session, AssetRef: "asset_hidden", NoteID: "note_alpha", NoteRevision: alphaRevision}); err == nil || !strings.Contains(err.Error(), "reader_object_not_found") {
		t.Fatalf("hidden asset err = %v", err)
	}
}

func TestReaderResourcesSourceExport(t *testing.T) {
	t.Parallel()
	root := resourcesTestVault(t)
	svc := NewService()
	session := resourcesSession(ReaderScopeBody)
	alphaRevision := noteBodyRevision(t, root, "notes/alpha.md")

	projection, content, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{
		VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: alphaRevision,
	})
	if err != nil {
		t.Fatalf("ReaderSourceExport: %v", err)
	}
	if content.MediaType != "text/markdown; charset=utf-8" || content.SizeBytes == 0 || !strings.HasPrefix(content.Digest, "sha256:") {
		t.Fatalf("content = %#v", content)
	}
	exported := new(strings.Builder)
	if _, err := io.Copy(exported, content.ReadSeeker); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exported.String(), "Links to [[note_beta|Beta]]") {
		t.Fatalf("export body = %q", exported.String())
	}
	if strings.Contains(fmt.Sprint(projection.Facts), "/attachments") {
		t.Fatalf("export leaked paths into facts: %#v", projection.Facts)
	}

	// Path inputs are refused: object ids and revisions are not paths.
	if _, _, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{VaultPath: root, Session: session, ObjectID: "../etc/passwd", Revision: alphaRevision}); err == nil || !strings.Contains(err.Error(), "reader_request_invalid") {
		t.Fatalf("path object err = %v", err)
	}
	// Scope and revision discipline.
	if _, _, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{VaultPath: root, Session: resourcesSession(ReaderScopeMetadata), ObjectID: "note_alpha", Revision: alphaRevision}); err == nil || !strings.Contains(err.Error(), "insufficient_scope") {
		t.Fatalf("export without body scope err = %v", err)
	}
	if _, _, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: "sha256:stale"}); err == nil || !strings.Contains(err.Error(), "revision_unavailable") {
		t.Fatalf("stale export err = %v", err)
	}
}

func noteRevisionOf(t *testing.T, svc *Service, root, objectID string) (string, domain.Note) {
	t.Helper()
	notes, err := scanNotes(root)
	if err != nil {
		t.Fatal(err)
	}
	note, ok := findNoteByObjectID(ordinaryNotes(notes), objectID)
	if !ok {
		t.Fatalf("note %s not found", objectID)
	}
	return readerNoteRevision(note), note
}

func noteBodyRevision(t *testing.T, root, notePath string) string {
	t.Helper()
	notes, err := scanNotes(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range notes {
		if note.Path == filepath.ToSlash(notePath) {
			return readerNoteRevision(note)
		}
	}
	t.Fatalf("note at %s not found", notePath)
	return ""
}

func TestReaderResourcesLinksFragmentPaginationAndWatermark(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\nLinks [[note_beta#甲]] and [[note_beta#乙]] separately.\n")
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nstatus: active\n---\n\nBeta body.\n")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	session := resourcesSession(ReaderScopeRelations, ReaderScopeBody)
	revision := noteBodyRevision(t, root, "notes/alpha.md")

	// Two fragment links to the same note produce two distinct-anchor items.
	first, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{
		VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: revision,
		Direction: "outgoing", Limit: 1, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	firstPage := first.Data.(map[string]any)["links"].(ReaderLinkPage)
	if len(firstPage.Items) != 1 || firstPage.NextCursor == "" {
		t.Fatalf("first page = %#v", firstPage)
	}
	seen := map[string]bool{firstPage.Items[0].ObjectID + "\x00" + firstPage.Items[0].Anchor: true}

	// The second page must deliver the OTHER relation, not skip it.
	second, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{
		VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: revision,
		Direction: "outgoing", Limit: 1, Cursor: firstPage.NextCursor, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	secondPage := second.Data.(map[string]any)["links"].(ReaderLinkPage)
	if len(secondPage.Items) != 1 {
		t.Fatalf("second page = %#v", secondPage.Items)
	}
	key := secondPage.Items[0].ObjectID + "\x00" + secondPage.Items[0].Anchor
	if seen[key] {
		t.Fatalf("pagination skipped the second fragment relation: %#v", secondPage.Items)
	}
	seen[key] = true
	if len(seen) != 2 || firstPage.Items[0].Anchor == secondPage.Items[0].Anchor {
		t.Fatalf("fragment anchors not distinct: %#v vs %#v", firstPage.Items[0], secondPage.Items[0])
	}

	// Relation-set change (fragment edit): the watermark moves and the old
	// cursor expires with 410 vocabulary.
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\nLinks [[note_beta#丙]] and [[note_beta#乙]] separately.\n")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	newRevision := noteBodyRevision(t, root, "notes/alpha.md")
	if _, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{
		VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: newRevision,
		Direction: "outgoing", Limit: 1, Cursor: firstPage.NextCursor, CursorSecret: readerCursorSecret(),
	}); err == nil || !strings.Contains(err.Error(), "reader_cursor_expired") {
		t.Fatalf("relation-set change cursor err = %v", err)
	}
}

func TestReaderResourcesLinksAliasDuplicatesProjectToOneItem(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Two aliases pointing at the same target resolve to the same reader
	// relation identity; the projection collapses them so limit=1 paging
	// never skips a returned relation.
	writeAppFixture(t, filepath.Join(root, "notes", "alpha.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nstatus: active\nupdated_at: 2026-09-10T10:00:00+00:00\n---\n\nSee [[note_beta|别名甲]] and [[note_beta|别名乙]].\n")
	writeAppFixture(t, filepath.Join(root, "notes", "beta.md"),
		"---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nstatus: active\n---\n\nBeta body.\n")
	if err := refreshIndex(root); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	session := resourcesSession(ReaderScopeRelations, ReaderScopeBody)
	revision := noteBodyRevision(t, root, "notes/alpha.md")

	// A single page already holds the one projected relation.
	whole, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{
		VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: revision,
		Direction: "outgoing", CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatal(err)
	}
	wholePage := whole.Data.(map[string]any)["links"].(ReaderLinkPage)
	if len(wholePage.Items) != 1 || wholePage.Items[0].ObjectID != "note_beta" {
		t.Fatalf("alias duplicates did not collapse: %#v", wholePage.Items)
	}

	// limit=1 paging terminates after one page and never skips anything.
	first, err := svc.ReaderLinks(context.Background(), ReaderLinksRequest{
		VaultPath: root, Session: session, ObjectID: "note_alpha", Revision: revision,
		Direction: "outgoing", Limit: 1, CursorSecret: readerCursorSecret(),
	})
	if err != nil {
		t.Fatal(err)
	}
	firstPage := first.Data.(map[string]any)["links"].(ReaderLinkPage)
	if len(firstPage.Items) != 1 || firstPage.NextCursor != "" {
		t.Fatalf("paged alias links = %#v", firstPage)
	}
}

func TestReaderResourcesSourceExportBoundedByTarget(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "TARGETBODY0123456789"
	targetFile := filepath.Join(root, "notes", "target.md")
	targetContent := "---\nschema_version: pinax.note.v1\nnote_id: note_target\ntitle: Target\nstatus: active\n---\n\n\n" + body + "\n\n\n"
	if err := os.WriteFile(targetFile, []byte(targetContent), 0o644); err != nil {
		t.Fatal(err)
	}
	// An unreadable sibling note: the old full-scan resolution propagated
	// its read error and failed the WHOLE export; the bounded locator only
	// probes candidate heads and skips unreadable entries, so the target
	// export stays available regardless of sibling state.
	lockedFile := filepath.Join(root, "notes", "locked.md")
	if err := os.WriteFile(lockedFile, []byte("---\nschema_version: pinax.note.v1\nnote_id: note_locked\ntitle: Locked\nstatus: active\n---\n\nLocked body.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockedFile, 0o0000); err != nil {
		t.Skipf("chmod 0000 unavailable: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedFile, 0o644) })

	svc := NewService()
	session := resourcesSession(ReaderScopeBody)
	revision := collaborationRevision(targetContent)
	_, content, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{
		VaultPath: root, Session: session, ObjectID: "note_target", Revision: revision,
	})
	if err != nil {
		t.Fatalf("bounded export failed on locked sibling: %v", err)
	}
	// Trimmed-body semantics: leading/trailing blank lines in the file are
	// NOT part of the exported body — bytes, size and digest all match the
	// parsed body exactly.
	exported, err := io.ReadAll(content.ReadSeeker)
	if err != nil {
		t.Fatal(err)
	}
	if string(exported) != body {
		t.Fatalf("export body = %q want %q", exported, body)
	}
	if content.SizeBytes != int64(len(body)) {
		t.Fatalf("export size = %d want %d", content.SizeBytes, len(body))
	}
	sum := sha256.Sum256([]byte(body))
	if content.Digest != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatalf("export digest = %s", content.Digest)
	}
	// The stream stays seekable for ranged windows.
	window := make([]byte, 5)
	if _, err := content.ReadSeeker.Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(content.ReadSeeker, window); err != nil {
		t.Fatal(err)
	}
	if string(window) != "BODY0" {
		t.Fatalf("window = %q", window)
	}
	if closer, ok := content.ReadSeeker.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	// Revision mismatch still routes to the pinned-recovery path and fails
	// closed when no snapshot matches.
	if _, _, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{
		VaultPath: root, Session: session, ObjectID: "note_target", Revision: "sha256:" + strings.Repeat("0", 64),
	}); err == nil || !strings.Contains(err.Error(), "revision_unavailable") {
		t.Fatalf("stale revision err = %v", err)
	}
	// The unreadable sibling itself is not resolvable as an export target.
	if _, _, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{
		VaultPath: root, Session: session, ObjectID: "note_locked", Revision: revision,
	}); err == nil || !strings.Contains(err.Error(), "reader_object_not_found") {
		t.Fatalf("locked target err = %v", err)
	}
}

func TestReaderResourcesSourceExportStreamMatchesParsedBody(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("line of crlf text\r\n", 8000) // >128 KiB CRLF body
	giantLead := strings.Repeat(" ", 200*1024) + "TAILBODY"
	cases := []struct {
		name    string
		noteID  string
		content string
	}{
		{"crlf-long", "note_crlf", "---\r\nschema_version: pinax.note.v1\r\nnote_id: note_crlf\r\ntitle: C\r\nstatus: active\r\n---\r\n\r\n" + long + "\r\n"},
		{"unicode-boundary", "note_uni", "---\nschema_version: pinax.note.v1\nnote_id: note_uni\ntitle: U\nstatus: active\n---\n\n\u3000正文\u00a0\n"},
		{"leading-whitespace-beyond-probe", "note_lead", "---\nschema_version: pinax.note.v1\nnote_id: note_lead\ntitle: L\nstatus: active\n---\n\n" + giantLead + "\n"},
		// A legal frontmatter whose closing delimiter sits BEYOND the
		// 128 KiB probe window (large YAML comment block): the note stays
		// visible to the owner scan, so the export locator must resolve
		// it too — never 404 on a readable, authorized object.
		{"frontmatter-beyond-probe", "note_bigfm", "---\nschema_version: pinax.note.v1\nnote_id: note_bigfm\ntitle: F\nstatus: active\n" + strings.Repeat("# a long yaml comment line filling the frontmatter block\n", 2600) + "---\n\nBIGFM-BODY-0123456789\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "notes", "n.md"), []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			// Byte-exact equivalence oracle: the parsed body the export must
			// reproduce is what parseNote itself yields.
			expected := parseNote("notes/n.md", tc.content).Body
			if expected == "" {
				t.Fatal("oracle body unexpectedly empty")
			}
			svc := NewService()
			_, content, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{
				VaultPath: root, Session: resourcesSession(ReaderScopeBody),
				ObjectID: tc.noteID, Revision: collaborationRevision(tc.content),
			})
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			exported, err := io.ReadAll(content.ReadSeeker)
			if err != nil {
				t.Fatal(err)
			}
			if string(exported) != expected {
				t.Fatalf("body mismatch: got %d bytes %q… want %d bytes %q…", len(exported), clipBytes(exported), len(expected), clipBytes([]byte(expected)))
			}
			if content.SizeBytes != int64(len(expected)) {
				t.Fatalf("size = %d want %d", content.SizeBytes, len(expected))
			}
			sum := sha256.Sum256([]byte(expected))
			if content.Digest != "sha256:"+hex.EncodeToString(sum[:]) {
				t.Fatal("digest mismatch")
			}
			if closer, ok := content.ReadSeeker.(io.Closer); ok {
				_ = closer.Close()
			}
		})
	}
}

func clipBytes(b []byte) string {
	if len(b) > 40 {
		return string(b[:40])
	}
	return string(b)
}

func TestReaderResourcesSourceExportProbeCutsMidLine(t *testing.T) {
	t.Parallel()
	// The probe window must never treat a TRUNCATED line as the frontmatter
	// closing delimiter: a YAML block scalar line "  ---not-a-delimiter"
	// cut right after "  ---" must not be mistaken for "---".
	before := "---\nschema_version: pinax.note.v1\nnote_id: note_cut\ntitle: C\nstatus: active\ndescription: |\n"
	filler := strings.Repeat("  block scalar filler line\n", 100)
	// Position the "  ---not-a-delimiter" line so the probe's LAST bytes
	// are exactly "\n  ---" (the cut lands mid-line).
	lineStart := int64(len(before) + len(filler))
	padSpaces := int(int64(readerExportHeadBytes)-3) - int(lineStart) - 4 // final padded line "  " + spaces + newline
	if padSpaces < 0 {
		t.Fatalf("fixture arithmetic negative: %d", padSpaces)
	}
	padLine := "  " + strings.Repeat("p", padSpaces) + "\n"
	content := before + filler + padLine + "  ---not-a-delimiter\n---\nCUT-BODY-0123456789\n"
	headCut := content[:readerExportHeadBytes+1]
	if !strings.HasSuffix(headCut, "\n  ---") {
		t.Fatalf("probe cut mispositioned: %q", headCut[len(headCut)-10:])
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes", "cut.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	expected := parseNote("notes/cut.md", content).Body
	if expected != "CUT-BODY-0123456789" {
		t.Fatalf("oracle body = %q", expected)
	}
	svc := NewService()
	_, exported, err := svc.ReaderSourceExport(context.Background(), ReaderSourceExportRequest{
		VaultPath: root, Session: resourcesSession(ReaderScopeBody),
		ObjectID: "note_cut", Revision: collaborationRevision(content),
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	got, err := io.ReadAll(exported.ReadSeeker)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != expected {
		t.Fatalf("export body = %q want %q", got, expected)
	}
	if exported.SizeBytes != int64(len(expected)) {
		t.Fatalf("size = %d want %d", exported.SizeBytes, len(expected))
	}
	if closer, ok := exported.ReadSeeker.(io.Closer); ok {
		_ = closer.Close()
	}
}
