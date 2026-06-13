package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func setupFixtureVault(t *testing.T, root string) {
	t.Helper()
	writeAppFixture(t, filepath.Join(root, "inbox", "inbox-item-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_inbox_1\ntitle: Inbox Item 1\nstatus: inbox\nkind: inbox\n---\n# Inbox Item 1\nbody inbox\n")
	writeAppFixture(t, filepath.Join(root, "drafts", "draft-item-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_draft_1\ntitle: Draft Item 1\nstatus: draft\nkind: draft\n---\n# Draft Item 1\nbody draft\n")
	writeAppFixture(t, filepath.Join(root, "notes", "active-item-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_active_1\ntitle: Active Item 1\nstatus: active\nkind: reference\n---\n# Active Item 1\nbody active\n")
	writeAppFixture(t, filepath.Join(root, "notes", "archived-item-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_archived_1\ntitle: Archived Item 1\nstatus: archived\nkind: reference\n---\n# Archived Item 1\nbody archived\n")
	writeAppFixture(t, filepath.Join(root, "notes", "discarded-item-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_discarded_1\ntitle: Discarded Item 1\nstatus: discarded\nkind: reference\n---\n# Discarded Item 1\nbody discarded\n")
	writeAppFixture(t, filepath.Join(root, "index", "inbox.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_index_inbox\ntitle: Inbox Index\nstatus: system\nkind: index\n---\n# Inbox Index\n<!-- pinax:index -->\n- hello\n<!-- pinax:end -->\n")
	writeAppFixture(t, filepath.Join(root, "notes", "custom-status-item-1.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_custom_1\ntitle: Custom Status Item 1\nstatus: doing\nkind: task\n---\n# Custom Status Item 1\nbody custom\n")
}

func initTestEnv(t *testing.T) (context.Context, string, *Service) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Test Vault"}); err != nil {
		t.Fatalf("failed to init vault: %v", err)
	}
	setupFixtureVault(t, root)
	if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("failed to rebuild index: %v", err)
	}
	return ctx, root, svc
}

func TestDraftInboxLifecycle(t *testing.T) {
	// 1.2.1 Test Draft Create
	t.Run("DraftCreate", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		req := CreateNoteRequest{
			VaultPath: root,
			Title:     "New Draft Idea",
			Body:      "Body of new draft idea",
		}
		proj, err := svc.DraftCreate(ctx, req)
		if err != nil {
			t.Fatalf("DraftCreate failed: %v", err)
		}
		if proj.Facts["status"] != "draft" {
			t.Errorf("expected status 'draft', got %q", proj.Facts["status"])
		}
		expectedPath := "drafts/new-draft-idea.md"
		if !strings.Contains(proj.Facts["path"], "new-draft-idea") {
			t.Errorf("expected path to contain 'new-draft-idea', got %q", proj.Facts["path"])
		}
		filePath := filepath.Join(root, expectedPath)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("file was not created at %q", filePath)
		}
	})

	// 1.2.2 Test Draft List
	t.Run("DraftList", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		proj, err := svc.DraftList(ctx, VaultRequest{VaultPath: root})
		if err != nil {
			t.Fatalf("DraftList failed: %v", err)
		}
		if proj.Facts["fact.filter.status"] != "draft" {
			t.Errorf("expected filter status 'draft', got %q", proj.Facts["fact.filter.status"])
		}
	})

	// 1.2.3 Test Draft Show
	t.Run("DraftShow", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		proj, err := svc.DraftShow(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "note_draft_1"})
		if err != nil {
			t.Fatalf("DraftShow failed: %v", err)
		}
		if proj.Facts["note_id"] != "note_draft_1" {
			t.Errorf("expected note_id 'note_draft_1', got %q", proj.Facts["note_id"])
		}
	})

	// 1.2.4 Test Draft Promote (Normal)
	t.Run("DraftPromote_Normal", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		req := DraftPromoteRequest{
			VaultPath: root,
			NoteRef:   "note_draft_1",
			Status:    "active",
			Folder:    "research",
			Kind:      "reference",
			Yes:       true,
		}
		proj, err := svc.DraftPromote(ctx, req)
		if err != nil {
			t.Fatalf("DraftPromote failed: %v", err)
		}
		if proj.Facts["status"] != "active" {
			t.Errorf("expected status 'active', got %q", proj.Facts["status"])
		}
		if proj.Facts["writes"] != "true" {
			t.Errorf("expected writes 'true', got %q", proj.Facts["writes"])
		}
	})

	// 1.2.5 Test Draft Promote (Dry Run - No Side Effects)
	t.Run("DraftPromote_DryRun", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		req := DraftPromoteRequest{
			VaultPath: root,
			NoteRef:   "note_draft_1",
			Status:    "active",
			Folder:    "research",
			Kind:      "reference",
			DryRun:    true,
		}
		proj, err := svc.DraftPromote(ctx, req)
		if err != nil {
			t.Fatalf("DraftPromote DryRun failed: %v", err)
		}
		if proj.Facts["writes"] != "false" {
			t.Errorf("expected writes 'false' for DryRun, got %q", proj.Facts["writes"])
		}
		if _, err := os.Stat(filepath.Join(root, "drafts", "draft-item-1.md")); os.IsNotExist(err) {
			t.Errorf("file drafts/draft-item-1.md should still exist")
		}
	})

	// 1.2.6 Test Draft Archive
	t.Run("DraftArchive", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		req := NoteMutationRequest{
			VaultPath: root,
			NoteRef:   "note_draft_1",
			Yes:       true,
		}
		proj, err := svc.DraftArchive(ctx, req)
		if err != nil {
			t.Fatalf("DraftArchive failed: %v", err)
		}
		if proj.Facts["status"] != "archived" {
			t.Errorf("expected status 'archived', got %q", proj.Facts["status"])
		}
	})

	// 1.2.7 Test Draft Discard (Needs Yes)
	t.Run("DraftDiscard_NeedsYes", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		req := NoteMutationRequest{
			VaultPath: root,
			NoteRef:   "note_draft_1",
		}
		_, err := svc.DraftDiscard(ctx, req)
		if err == nil {
			t.Fatalf("expected error without Yes")
		}
		var cmdErr *domain.CommandError
		if !errors.As(err, &cmdErr) || cmdErr.Code != "approval_required" {
			t.Errorf("expected approval_required error, got %v", err)
		}
	})

	// 1.2.8 Test Draft Discard (Normal)
	t.Run("DraftDiscard_Normal", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		req := NoteMutationRequest{
			VaultPath: root,
			NoteRef:   "note_draft_1",
			Yes:       true,
		}
		proj, err := svc.DraftDiscard(ctx, req)
		if err != nil {
			t.Fatalf("DraftDiscard failed: %v", err)
		}
		if proj.Facts["status"] != "discarded" {
			t.Errorf("expected status 'discarded', got %q", proj.Facts["status"])
		}
		if proj.Facts["deleted"] != "false" {
			t.Errorf("expected deleted 'false', got %q", proj.Facts["deleted"])
		}
	})

	// 1.2.9 Test Inbox Show
	t.Run("InboxShow", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		proj, err := svc.InboxShow(ctx, ShowNoteRequest{VaultPath: root, NoteRef: "note_inbox_1"})
		if err != nil {
			t.Fatalf("InboxShow failed: %v", err)
		}
		if proj.Facts["note_id"] != "note_inbox_1" {
			t.Errorf("expected note_id 'note_inbox_1', got %q", proj.Facts["note_id"])
		}
	})

	// 1.2.10 Test Inbox Promote (To Draft)
	t.Run("InboxPromote_ToDraft", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		req := InboxPromoteRequest{
			VaultPath: root,
			NoteRef:   "note_inbox_1",
			To:        "draft",
			Yes:       true,
		}
		proj, err := svc.InboxPromote(ctx, req)
		if err != nil {
			t.Fatalf("InboxPromote failed: %v", err)
		}
		if proj.Facts["status"] != "draft" {
			t.Errorf("expected status 'draft', got %q", proj.Facts["status"])
		}
	})

	// 1.2.11 Test Inbox Promote (To Active with Folder and Kind)
	t.Run("InboxPromote_ToActive", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		req := InboxPromoteRequest{
			VaultPath: root,
			NoteRef:   "note_inbox_1",
			To:        "active",
			Group:     "work",
			Folder:    "ideas",
			Kind:      "reference",
			Yes:       true,
		}
		proj, err := svc.InboxPromote(ctx, req)
		if err != nil {
			t.Fatalf("InboxPromote failed: %v", err)
		}
		if proj.Facts["status"] != "active" {
			t.Errorf("expected status 'active', got %q", proj.Facts["status"])
		}
		if !strings.Contains(proj.Facts["path"], "notes/work/ideas/inbox-item-1.md") {
			t.Errorf("expected path notes/work/ideas/inbox-item-1.md, got %q", proj.Facts["path"])
		}
	})

	// 1.2.12 Test Inbox Promote Path Conflict
	t.Run("InboxPromote_Conflict", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		writeAppFixture(t, filepath.Join(root, "notes", "work", "ideas", "inbox-item-1.md"), "conflict")
		if _, err := svc.RebuildIndex(ctx, VaultRequest{VaultPath: root}); err != nil {
			t.Fatalf("failed to rebuild index: %v", err)
		}
		req := InboxPromoteRequest{
			VaultPath: root,
			NoteRef:   "note_inbox_1",
			To:        "active",
			Group:     "work",
			Folder:    "ideas",
			Yes:       true,
		}
		_, err := svc.InboxPromote(ctx, req)
		if err == nil {
			t.Fatalf("expected path conflict error")
		}
		var cmdErr *domain.CommandError
		if !errors.As(err, &cmdErr) || cmdErr.Code != "note_path_conflict" {
			t.Errorf("expected note_path_conflict error, got %v", err)
		}
	})

	// 1.2.13 Test Invalid Lifecycle Transition
	t.Run("InvalidLifecycleTransition", func(t *testing.T) {
		ctx, root, svc := initTestEnv(t)
		req := DraftPromoteRequest{
			VaultPath: root,
			NoteRef:   "note_draft_1",
			Status:    "inbox",
			Yes:       true,
		}
		_, err := svc.DraftPromote(ctx, req)
		if err == nil {
			t.Fatalf("expected error for invalid transition")
		}
		var cmdErr *domain.CommandError
		if !errors.As(err, &cmdErr) || cmdErr.Code != "invalid_lifecycle_transition" {
			t.Errorf("expected invalid_lifecycle_transition error, got %v", err)
		}
	})
}

func TestListNotesQuery_DiscardedFilter(t *testing.T) {
	ctx, root, svc := initTestEnv(t)

	t.Run("excludes_discarded_by_default", func(t *testing.T) {
		proj, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root})
		if err != nil {
			t.Fatalf("ListNotesQuery failed: %v", err)
		}
		dataMap, ok := proj.Data.(map[string]any)
		if !ok {
			t.Fatal("expected proj.Data to be map[string]any")
		}
		notesRaw, ok := dataMap["notes"]
		if !ok || notesRaw == nil {
			t.Fatal("expected notes in data")
		}
		notes := notesRaw.([]domain.Note)
		for _, n := range notes {
			if n.Status == "discarded" {
				t.Errorf("discarded note should be excluded, got note %q with status discarded", n.ID)
			}
		}
	})

	t.Run("includes_discarded_when_explicit", func(t *testing.T) {
		proj, err := svc.ListNotesQuery(ctx, NoteListRequest{VaultPath: root, Status: "discarded"})
		if err != nil {
			t.Fatalf("ListNotesQuery failed: %v", err)
		}
		dataMap, ok := proj.Data.(map[string]any)
		if !ok {
			t.Fatal("expected proj.Data to be map[string]any")
		}
		notesRaw, ok := dataMap["notes"]
		if !ok || notesRaw == nil {
			t.Fatal("expected notes in data")
		}
		notes := notesRaw.([]domain.Note)
		found := false
		for _, n := range notes {
			if n.Status == "discarded" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected to find discarded note when Status='discarded'")
		}
	})
}

func TestFilterSearchNotes_DiscardedFilter(t *testing.T) {
	notes := []domain.Note{
		{ID: "n1", Title: "Active", Status: "active"},
		{ID: "n2", Title: "Discarded", Status: "discarded"},
		{ID: "n3", Title: "Inbox", Status: "inbox"},
	}

	t.Run("excludes_discarded_by_default", func(t *testing.T) {
		filtered := filterSearchNotes(notes, SearchRequest{})
		for _, n := range filtered {
			if n.Status == "discarded" {
				t.Error("discarded should be excluded by default")
			}
		}
		if len(filtered) != 2 {
			t.Errorf("expected 2 notes, got %d", len(filtered))
		}
	})

	t.Run("includes_discarded_when_explicit", func(t *testing.T) {
		filtered := filterSearchNotes(notes, SearchRequest{Status: "discarded"})
		if len(filtered) != 1 || filtered[0].Status != "discarded" {
			t.Errorf("expected 1 discarded note, got %v", filtered)
		}
	})
}
