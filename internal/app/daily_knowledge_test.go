package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
)

func TestDailyKnowledgeSavedFailureKeepsIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(context.Background(), InitVaultRequest{VaultPath: root, Title: "Fixture"}); err != nil {
		t.Fatal(err)
	}
	// 已保存正文后，损坏的索引投影使后续步骤失败；不能丢掉原对象身份。
	index := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(index); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(index, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := svc.InboxCapture(context.Background(), CreateNoteRequest{VaultPath: root, Title: "Saved", Body: "retained original"})
	if err == nil {
		t.Fatal("expected index failure")
	}
	if got.Facts["note_id"] == "" || got.Facts["readback_status"] != "verified" || got.Facts["writes"] != "true" {
		t.Fatalf("lost original operation: %+v", got)
	}
	stored, readErr := os.ReadFile(filepath.Join(root, got.Facts["path"]))
	if readErr != nil || !strings.Contains(string(stored), "retained original") {
		t.Fatal("saved original missing")
	}
}

func TestDailyKnowledgeIntakeEligibility(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		note domain.Note
		want bool
	}{
		{domain.Note{Status: "inbox", Path: "notes/inbox/a.md"}, true},
		{domain.Note{Status: "active", Path: "notes/index/a.md"}, true},
		{domain.Note{Status: "active", Path: "notes/index/a.md", Project: "research"}, false},
		{domain.Note{Status: "archived", Path: "notes/index/a.md"}, false},
		{domain.Note{Status: "discarded", Path: "notes/index/a.md"}, false},
		{domain.Note{Status: "system", Path: "notes/index/a.md"}, false},
		{domain.Note{Status: "active", Path: "notes/index-other/a.md"}, false},
	} {
		if got := isDailyIntakeNote(test.note); got != test.want {
			t.Fatalf("%+v: got %v", test.note, got)
		}
	}
}

func TestDailyKnowledgeReviewDeduplicatesIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	svc := NewService()
	ctx := context.Background()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Fixture"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b"} {
		writeAppFixture(t, filepath.Join(root, "notes", "index", name+".md"), boundNoteFixture(boundObjectID, "Same"))
	}
	got, err := svc.InboxReview(ctx, InboxReviewRequest{VaultPath: root, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got.Facts["total"] != "1" {
		t.Fatalf("duplicate identity counted twice: %+v", got.Facts)
	}
}
