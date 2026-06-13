package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRootContentLayoutCreatesDefaultNoteJournalAndIndexPaths(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	note, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Demo", Slug: "demo", Body: "body", DryRun: true})
	if err != nil {
		t.Fatalf("preview note: %v", err)
	}
	if note.Facts["planned_path"] != "demo.md" {
		t.Fatalf("default note planned path = %#v", note.Facts)
	}

	created, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Demo", Slug: "demo", Body: "body"})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if created.Facts["path"] != "demo.md" || !fileExistsApp(filepath.Join(root, "demo.md")) {
		t.Fatalf("default note root path facts=%#v", created.Facts)
	}
	if fileExistsApp(filepath.Join(root, "notes", "demo.md")) {
		t.Fatalf("default note should not be written under legacy notes/")
	}

	daily, err := svc.DailyShow(ctx, DailyRequest{VaultPath: root, Date: "2026-06-08"})
	if err != nil {
		t.Fatalf("daily show: %v", err)
	}
	if daily.Facts["path"] != "daily/2026-06-08.md" || !fileExistsApp(filepath.Join(root, "daily", "2026-06-08.md")) {
		t.Fatalf("daily path facts=%#v", daily.Facts)
	}

	index, err := svc.CreateIndexPage(ctx, IndexPageRequest{VaultPath: root, Name: "home"})
	if err != nil {
		t.Fatalf("create index page: %v", err)
	}
	if index.Facts["path"] != "index/home.md" || !fileExistsApp(filepath.Join(root, "index", "home.md")) {
		t.Fatalf("index page facts=%#v", index.Facts)
	}
}

func TestLegacyNotesCompatKeepsExistingDailyNoteInPlace(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	legacyPath := filepath.Join(root, "notes", "daily", "2026-06-08.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy daily: %v", err)
	}
	legacyBody := "---\nschema_version: pinax.note.v1\nnote_id: note_legacy_daily\ntitle: Legacy Daily\ntags: []\nfolder: daily\nkind: daily\n---\n\n# Legacy Daily\n\n用户旧内容\n"
	if err := os.WriteFile(legacyPath, []byte(legacyBody), 0o644); err != nil {
		t.Fatalf("write legacy daily: %v", err)
	}

	projection, err := svc.DailyShow(ctx, DailyRequest{VaultPath: root, Date: "2026-06-08"})
	if err != nil {
		t.Fatalf("daily show legacy: %v", err)
	}
	if projection.Facts["path"] != "notes/daily/2026-06-08.md" {
		t.Fatalf("legacy daily path facts=%#v", projection.Facts)
	}
	if fileExistsApp(filepath.Join(root, "daily", "2026-06-08.md")) {
		t.Fatalf("legacy daily open should not auto-migrate into root daily/")
	}
	if got := readFile(t, legacyPath); got != legacyBody {
		t.Fatalf("legacy daily should not be rewritten:\n%s", got)
	}
}
