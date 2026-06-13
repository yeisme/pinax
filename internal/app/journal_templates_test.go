package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJournalTemplateCreatesDailyAndDoesNotRewriteExisting(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	projection, err := svc.DailyShow(ctx, DailyRequest{VaultPath: root, Date: "2026-06-08", Template: "journal.daily"})
	if err != nil {
		t.Fatalf("daily show: %v", err)
	}
	if projection.Facts["path"] != "daily/2026-06-08.md" || projection.Facts["template"] != "journal.daily" {
		t.Fatalf("daily projection facts = %#v", projection.Facts)
	}
	path := filepath.Join(root, "daily", "2026-06-08.md")
	body := readFile(t, path)
	for _, want := range []string{"title: Daily-2026-06-08", "# 2026-06-08", "## Today's Focus", "<!-- pinax:managed name=daily-captures -->"} {
		if !strings.Contains(body, want) {
			t.Fatalf("daily body missing %q:\n%s", want, body)
		}
	}

	modified := body + "\n用户手写内容\n"
	writeFile(t, path, modified)
	if _, err := svc.DailyShow(ctx, DailyRequest{VaultPath: root, Date: "2026-06-08", Template: "journal.daily"}); err != nil {
		t.Fatalf("daily show existing: %v", err)
	}
	if got := readFile(t, path); got != modified {
		t.Fatalf("existing daily note was rewritten:\n%s", got)
	}
}

func TestIndexPageCreatePreviewAndRefreshManagedBlock(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}

	preview, err := svc.PreviewIndexPage(ctx, IndexPageRequest{VaultPath: root, Name: "home", Template: "index.home"})
	if err != nil {
		t.Fatalf("preview index page: %v", err)
	}
	if preview.Command != "index.page.preview" || preview.Facts["writes"] != "false" {
		t.Fatalf("preview projection = %#v", preview)
	}
	if _, err := os.Stat(filepath.Join(root, "index", "home.md")); !os.IsNotExist(err) {
		t.Fatalf("preview wrote index page: %v", err)
	}

	created, err := svc.CreateIndexPage(ctx, IndexPageRequest{VaultPath: root, Name: "home", Template: "index.home"})
	if err != nil {
		t.Fatalf("create index page: %v", err)
	}
	if created.Facts["path"] != "index/home.md" || created.Facts["template"] != "index.home" || created.Facts["managed_blocks"] == "" {
		t.Fatalf("create facts = %#v", created.Facts)
	}
	path := filepath.Join(root, "index", "home.md")
	original := readFile(t, path)
	writeFile(t, path, original+"\n用户备注\n")

	refreshed, err := svc.RefreshIndexPage(ctx, IndexPageRequest{VaultPath: root, Name: "home", Template: "index.home"})
	if err != nil {
		t.Fatalf("refresh index page: %v", err)
	}
	if refreshed.Command != "index.page.refresh" || refreshed.Facts["path"] != "index/home.md" {
		t.Fatalf("refresh projection = %#v", refreshed)
	}
	body := readFile(t, path)
	if !strings.Contains(body, "用户备注") || !strings.Contains(body, "<!-- pinax:managed name=recent -->") {
		t.Fatalf("refresh should preserve user text and managed block:\n%s", body)
	}
}

func TestExistingDailyNotRewritten(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	path := filepath.Join(root, "daily", "2026-06-08.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir daily: %v", err)
	}
	body := "---\nschema_version: pinax.note.v1\nnote_id: note_daily\ntitle: Daily\ntags: []\nfolder: daily\nkind: daily\n---\n\n# Daily\n\n用户内容\n"
	writeFile(t, path, body)
	projection, err := svc.DailyShow(ctx, DailyRequest{VaultPath: root, Date: "2026-06-08"})
	if err != nil {
		t.Fatalf("daily show: %v", err)
	}
	if projection.Facts["path"] != "daily/2026-06-08.md" {
		t.Fatalf("daily facts = %#v", projection.Facts)
	}
	if got := readFile(t, path); got != body {
		t.Fatalf("existing daily rewritten:\n%s", got)
	}
}

func TestLegacyNotesDailyCompat(t *testing.T) {
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
	legacyBody := "---\nschema_version: pinax.note.v1\nnote_id: note_legacy_daily\ntitle: Legacy Daily\ntags: []\nfolder: daily\nkind: daily\n---\n\n# Legacy Daily\n"
	writeFile(t, legacyPath, legacyBody)
	projection, err := svc.DailyShow(ctx, DailyRequest{VaultPath: root, Date: "2026-06-08"})
	if err != nil {
		t.Fatalf("daily show legacy: %v", err)
	}
	if projection.Facts["path"] != "notes/daily/2026-06-08.md" || fileExistsApp(filepath.Join(root, "daily", "2026-06-08.md")) {
		t.Fatalf("legacy daily facts = %#v", projection.Facts)
	}
	if got := readFile(t, legacyPath); got != legacyBody {
		t.Fatalf("legacy daily rewritten:\n%s", got)
	}
}

func TestDailyCaptureManagedBlock(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-08")
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	projection, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Capture", Slug: "capture", Body: "body", Tags: []string{"pinax"}})
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	if projection.Facts["daily_index"] != "daily/2026-06-08.md" || projection.Facts["daily_index_status"] != "updated" {
		t.Fatalf("daily capture facts = %#v", projection.Facts)
	}
	if fileExistsApp(filepath.Join(root, "notes", "daily", "2026-06-08.md")) {
		t.Fatalf("daily capture should not create legacy notes/daily index")
	}
	daily := readFile(t, filepath.Join(root, "daily", "2026-06-08.md"))
	if !strings.Contains(daily, "<!-- pinax:managed name=daily-captures -->") || !strings.Contains(daily, "capture.md") || !strings.Contains(daily, "#pinax") {
		t.Fatalf("daily capture block not updated:\n%s", daily)
	}
}

func TestDailyCaptureLegacyMissingBlock(t *testing.T) {
	t.Setenv("PINAX_TEST_NOW", "2026-06-08")
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	dailyPath := filepath.Join(root, "daily", "2026-06-08.md")
	if err := os.MkdirAll(filepath.Dir(dailyPath), 0o755); err != nil {
		t.Fatalf("mkdir daily: %v", err)
	}
	dailyBody := "---\nschema_version: pinax.note.v1\nnote_id: note_daily\ntitle: Daily\ntags: []\nfolder: daily\nkind: daily\n---\n\n# Daily\n\n旧正文\n"
	writeFile(t, dailyPath, dailyBody)
	projection, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Capture", Slug: "capture", Body: "body"})
	if err != nil {
		t.Fatalf("create note with missing daily block should keep note: %v", err)
	}
	if projection.Status != "partial" || projection.Facts["daily_index_status"] != "managed_block_missing" || len(projection.Actions) == 0 {
		t.Fatalf("missing block projection = %#v", projection)
	}
	if got := readFile(t, dailyPath); got != dailyBody {
		t.Fatalf("daily missing block should not be rewritten:\n%s", got)
	}
}
