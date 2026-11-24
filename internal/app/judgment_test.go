package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/inboxjudgment"
)

// judgment_test.go 覆盖宿主接线：默认休眠、显式 dry-run 零调用、启用但未
// 配置执行 profile 时 fail closed，以及候选选择只使用确定性证据。

func judgmentFixtureVault(t *testing.T) (context.Context, string, *Service) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Judgment"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeAppFixture(t, filepath.Join(root, "notes", "inbox", "a.md"), "---\nschema_version: pinax.note.v1\ntitle: Inbox A\nstatus: inbox\nkind: inbox\ntags:\n  - deep-work\n---\n# Inbox A\ninbox excerpt about scheduling\n")
	writeAppFixture(t, filepath.Join(root, "notes", "ref", "b.md"), "---\nschema_version: pinax.note.v1\ntitle: Ref B\nstatus: active\nkind: reference\ntags:\n  - deep-work\n---\n# Ref B\nrelated b\n")
	writeAppFixture(t, filepath.Join(root, "notes", "ref", "c.md"), "---\nschema_version: pinax.note.v1\ntitle: Ref C\nstatus: active\nkind: reference\ntags:\n  - deep-work\n---\n# Ref C\nrelated c\n")
	writeAppFixture(t, filepath.Join(root, "notes", "ref", "d.md"), "---\nschema_version: pinax.note.v1\ntitle: Ref D\nstatus: active\nkind: reference\ntags:\n  - unrelated\n---\n# Ref D\nunrelated d\n")
	return ctx, root, svc
}

func TestInboxJudgmentDormantByDefault(t *testing.T) {
	ctx, root, svc := judgmentFixtureVault(t)
	projection, err := svc.InboxJudgment(ctx, InboxJudgmentRequest{VaultPath: root})
	if err != nil {
		t.Fatalf("dormant judgment must not fail: %v", err)
	}
	if projection.Command != InboxJudgmentCommand {
		t.Fatalf("command = %q", projection.Command)
	}
	if projection.Facts["judgment_mode"] != inboxjudgment.InboxJudgmentModeOff || projection.Facts["provider_dispatch"] != "none" {
		t.Fatalf("dormant projection must report off with zero dispatch: %+v", projection.Facts)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "api", "judgment.sqlite")); !os.IsNotExist(statErr) {
		t.Fatalf("dormant judgment must not open the receipt store: %v", statErr)
	}
}

func TestInboxJudgmentDryRunSkipsProviderAndCredentials(t *testing.T) {
	ctx, root, svc := judgmentFixtureVault(t)
	t.Setenv("PINAX_JUDGMENT_ENABLED", "true")
	t.Setenv("PINAX_JUDGMENT_MODE", "shadow")
	t.Setenv(JudgmentEnvProfile, "review")
	t.Setenv(JudgmentEnvProvider, "openai_decisions")
	t.Setenv(JudgmentEnvModel, "gpt-6-luna")
	t.Setenv(JudgmentEnvCredentialSource, "env")
	t.Setenv(JudgmentEnvCredentialEnv, "PINAX_JUDGMENT_TEST_CREDENTIAL")

	projection, err := svc.InboxJudgment(ctx, InboxJudgmentRequest{VaultPath: root, DryRun: true})
	if err != nil {
		t.Fatalf("dry run must succeed without a provider: %v", err)
	}
	if projection.Facts["judgment_status"] != "planned" || projection.Facts["provider_dispatch"] != "none" {
		t.Fatalf("dry run must stay offline: %+v", projection.Facts)
	}
	if projection.Facts["candidates"] != "2" {
		t.Fatalf("candidates = %q, want the two tag-related notes", projection.Facts["candidates"])
	}
	if projection.Facts["projection_digest"] == "" {
		t.Fatal("dry run must expose the deterministic projection digest")
	}
	if _, statErr := os.Stat(filepath.Join(root, ".pinax", "api", "judgment.sqlite")); !os.IsNotExist(statErr) {
		t.Fatalf("dry run must not open the receipt store: %v", statErr)
	}
}

func TestInboxJudgmentRequiresExplicitProfile(t *testing.T) {
	ctx, root, svc := judgmentFixtureVault(t)
	t.Setenv("PINAX_JUDGMENT_ENABLED", "true")
	t.Setenv("PINAX_JUDGMENT_MODE", "assist")
	for _, key := range []string{JudgmentEnvProfile, JudgmentEnvProvider, JudgmentEnvModel, JudgmentEnvCredentialSource} {
		t.Setenv(key, "")
	}
	_, err := svc.InboxJudgment(ctx, InboxJudgmentRequest{VaultPath: root})
	var commandErr *domain.CommandError
	if !errors.As(err, &commandErr) || commandErr.Code != "judgment_profile_not_configured" {
		t.Fatalf("enabled judgment without a profile must fail closed: %v", err)
	}
}

func TestInboxJudgmentRejectsNonInboxNote(t *testing.T) {
	ctx, root, svc := judgmentFixtureVault(t)
	t.Setenv("PINAX_JUDGMENT_ENABLED", "true")
	t.Setenv("PINAX_JUDGMENT_MODE", "shadow")
	t.Setenv(JudgmentEnvProfile, "review")
	t.Setenv(JudgmentEnvProvider, "openai_decisions")
	t.Setenv(JudgmentEnvModel, "gpt-6-luna")
	t.Setenv(JudgmentEnvCredentialSource, "env")
	t.Setenv(JudgmentEnvCredentialEnv, "PINAX_JUDGMENT_TEST_CREDENTIAL")
	_, err := svc.InboxJudgment(ctx, InboxJudgmentRequest{VaultPath: root, NoteRef: "notes/ref/b.md", DryRun: true})
	var commandErr *domain.CommandError
	if !errors.As(err, &commandErr) || commandErr.Code != "note_not_inbox" {
		t.Fatalf("judgment must only evaluate intake notes: %v", err)
	}
}

func TestInboxJudgmentCandidatesUseDeterministicEvidence(t *testing.T) {
	inbox := domain.Note{ID: "note_inbox", Path: "notes/inbox/a.md", Tags: []string{"deep-work"}, ContentDigest: "sha256:aaa", Body: "inbox body"}
	notes := []domain.Note{
		inbox,
		{ID: "note_shared", Path: "notes/ref/b.md", Tags: []string{"deep-work", "other"}, ContentDigest: "sha256:bbb", Body: "b"},
		{ID: "note_duplicate", Path: "notes/ref/c.md", Tags: []string{}, ContentDigest: "sha256:aaa", Body: "duplicate"},
		{ID: "note_unrelated", Path: "notes/ref/d.md", Tags: []string{"else"}, ContentDigest: "sha256:ddd", Body: "d"},
	}
	candidates := judgmentCandidates(notes, inbox, 8)
	if len(candidates) != 2 {
		t.Fatalf("candidates = %d, want the tag match and the exact duplicate only", len(candidates))
	}
	if candidates[0].NoteID != "note_duplicate" {
		t.Fatalf("exact content duplicates must rank first, got %q", candidates[0].NoteID)
	}
	if candidates[1].NoteID != "note_shared" || candidates[1].BaselineRank != 2 {
		t.Fatalf("tag match must keep its baseline rank, got %+v", candidates[1])
	}
	if limited := judgmentCandidates(notes, inbox, 1); len(limited) != 1 || limited[0].NoteID != "note_duplicate" {
		t.Fatalf("limit must bound the candidate set: %+v", limited)
	}
}
