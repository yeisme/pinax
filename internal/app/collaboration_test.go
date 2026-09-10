package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	"github.com/yeisme/pinax/internal/operation"
	"github.com/yeisme/pinax/internal/version/versiontest"
)

var collaborationTestPolicy = CollaborationPolicy{AllowBody: true, AllowWrite: true}

func collaborationFixture(t *testing.T) (*Service, string, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	if _, err := svc.InitVault(ctx, InitVaultRequest{VaultPath: root, Title: "Collaboration fixture"}); err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Example", Body: "# Original\n\nText to preserve.\n", Dir: "index"})
	if err != nil {
		t.Fatal(err)
	}
	return svc, root, p.Facts["path"]
}

func TestCollaborationWorkflowReplayAndRecovery(t *testing.T) {
	s, root, path := collaborationFixture(t)
	ctx := context.Background()
	for _, change := range []CollaborationChange{
		{Action: "append", NoteRef: path, Body: "\nAppended once.\n"},
		{Action: "replace", NoteRef: path, Body: "# Revised\n\nReviewed text.\n"},
		{Action: "tags", NoteRef: path, TagOperation: "add", Tags: []string{"reviewed"}},
		{Action: "archive", NoteRef: path},
		{Action: "create", Title: "Created in chat", Body: "# New\n\nNew body.\n"},
	} {
		t.Run(change.Action, func(t *testing.T) {
			p, err := s.CollaborationPreview(ctx, root, change, collaborationTestPolicy)
			if err != nil {
				t.Fatal(err)
			}
			if p.Digest == "" || p.Change.OperationID == "" {
				t.Fatal("missing proposal binding")
			}
			v, err := s.CollaborationApply(ctx, root, p.Change, p.Digest, "explicit_instruction", collaborationTestPolicy)
			if err != nil || v.Status != operation.StatusSucceeded {
				t.Fatalf("save status=%s err=%v", v.Status, err)
			}
			// Reconnect and replay after the note revision changed: no second domain write.
			replay, err := NewService().CollaborationApply(ctx, root, p.Change, p.Digest, "explicit_instruction", collaborationTestPolicy)
			if err != nil || replay.RevisionAfter != v.RevisionAfter || replay.CompletedAt != v.CompletedAt {
				t.Fatal("replay changed outcome", err)
			}
			got, err := NewService().CollaborationStatus(ctx, root, p.Change.OperationID)
			if err != nil || got.Status != operation.StatusSucceeded {
				t.Fatal("status not durable", err)
			}
			raw, _ := json.Marshal(got)
			if strings.Contains(string(raw), root) || (change.Body != "" && strings.Contains(string(raw), strings.TrimSpace(change.Body))) {
				t.Fatal("ledger contains content or absolute path")
			}
			if change.Action != "create" {
				n, revision, err := s.CollaborationVersion(ctx, root, p.Change.OperationID, collaborationTestPolicy)
				if err != nil || revision == "" || n.Path != path {
					t.Fatal("snapshot not readable", err)
				}
			}
		})
	}
	n, _, err := s.CollaborationRead(ctx, root, path, "read", collaborationTestPolicy)
	if err != nil || n.Status != "archived" || !strings.Contains(n.Body, "Reviewed text.") {
		t.Fatal("final note incorrect", err)
	}
}

func TestCollaborationPreviewCancelConflictAndPermissions(t *testing.T) {
	s, root, path := collaborationFixture(t)
	ctx := context.Background()
	original, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	c := CollaborationChange{Action: "replace", NoteRef: path, Body: "Replacement"}
	if _, _, err := s.CollaborationRead(ctx, root, path, "read", CollaborationPolicy{}); domain.ErrorCode(err) != "body_disabled" {
		t.Fatal("body permission bypass")
	}
	if _, _, err := s.CollaborationRead(ctx, root, path, "", collaborationTestPolicy); domain.ErrorCode(err) != "intent_required" {
		t.Fatal("intent bypass")
	}
	p, err := s.CollaborationPreview(ctx, root, c, collaborationTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "api", "operations.sqlite")); !os.IsNotExist(err) {
		t.Fatal("preview created ledger")
	}
	after, _ := os.ReadFile(filepath.Join(root, path))
	if string(after) != string(original) {
		t.Fatal("preview modified note")
	}
	if _, err := s.CollaborationApply(ctx, root, p.Change, p.Digest, "explicit_instruction", CollaborationPolicy{}); domain.ErrorCode(err) != "write_disabled" {
		t.Fatal("write permission bypass")
	}
	if _, err := s.CollaborationApply(ctx, root, p.Change, p.Digest, "human_click", collaborationTestPolicy); domain.ErrorCode(err) != "authorization_required" {
		t.Fatal("invented human proof accepted")
	}
	changed := p.Change
	changed.Body = "Different content"
	if _, err := s.CollaborationApply(ctx, root, changed, p.Digest, "reviewed_proposal", collaborationTestPolicy); domain.ErrorCode(err) != "preview_mismatch" {
		t.Fatal("changed proposal accepted")
	}
	external := string(original) + "\nExternal update.\n"
	if err := os.WriteFile(filepath.Join(root, path), []byte(external), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CollaborationApply(ctx, root, p.Change, p.Digest, "reviewed_proposal", collaborationTestPolicy); domain.ErrorCode(err) != "revision_conflict" {
		t.Fatalf("conflict not detected: %v", err)
	}
	actual, _ := os.ReadFile(filepath.Join(root, path))
	if string(actual) != external {
		t.Fatal("overwrote external edit")
	}
}

func TestCollaborationCreateDestinationAndIdentityConflict(t *testing.T) {
	s, root, _ := collaborationFixture(t)
	ctx := context.Background()
	normalized, err := s.CollaborationPreview(ctx, root, CollaborationChange{Action: "create", Title: "Whitespace", Body: "  normalized body  \n"}, collaborationTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CollaborationApply(ctx, root, normalized.Change, normalized.Digest, "explicit_instruction", collaborationTestPolicy); err != nil {
		t.Fatal(err)
	}
	note, _, err := s.CollaborationRead(ctx, root, normalized.Path, "read", collaborationTestPolicy)
	if err != nil || note.Body != normalized.After {
		t.Fatal("create preview did not match stored body", err)
	}
	p, err := s.CollaborationPreview(ctx, root, CollaborationChange{Action: "create", Title: "Same name", Body: "First"}, collaborationTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: "Same name", Body: "External", Dir: "index"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CollaborationApply(ctx, root, p.Change, p.Digest, "explicit_instruction", collaborationTestPolicy); domain.ErrorCode(err) != "revision_conflict" {
		t.Fatal("destination changed without review", err)
	}
	p, err = s.CollaborationPreview(ctx, root, CollaborationChange{Action: "create", Title: "Unique", Body: "First"}, collaborationTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CollaborationApply(ctx, root, p.Change, p.Digest, "explicit_instruction", collaborationTestPolicy); err != nil {
		t.Fatal(err)
	}
	p.Change.Body = "Second"
	if _, err := s.CollaborationApply(ctx, root, p.Change, collaborationDigest(root, p.Change), "explicit_instruction", collaborationTestPolicy); !operation.IsCode(err, operation.CodeIdempotencyConflict) {
		t.Fatal("operation identity reused", err)
	}
}

func TestCollaborationUnknownOutcomeNeverReapplies(t *testing.T) {
	s, root, path := collaborationFixture(t)
	ctx := context.Background()
	p, err := s.CollaborationPreview(ctx, root, CollaborationChange{Action: "append", NoteRef: path, Body: "Once"}, collaborationTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	failing := NewServiceWithVersionBackend(&versiontest.FakeBackend{SnapshotErr: errors.New("private filesystem path")})
	v, err := failing.CollaborationApply(ctx, root, p.Change, p.Digest, "reviewed_proposal", collaborationTestPolicy)
	if err != nil || v.Status != operation.StatusReconcileRequired {
		t.Fatal("missing uncertain outcome", err)
	}
	v, err = s.CollaborationApply(ctx, root, p.Change, p.Digest, "reviewed_proposal", collaborationTestPolicy)
	if err != nil || v.Status != operation.StatusReconcileRequired {
		t.Fatal("unsafe retry", err)
	}
	n, _, err := s.CollaborationRead(ctx, root, path, "read", collaborationTestPolicy)
	if err != nil || strings.Contains(n.Body, "Once") {
		t.Fatal("reapplied uncertain write", err)
	}
}

func TestCollaborationConcurrentPreviewsDoNotLoseUpdate(t *testing.T) {
	s, root, path := collaborationFixture(t)
	ctx := context.Background()
	p1, err := s.CollaborationPreview(ctx, root, CollaborationChange{Action: "append", NoteRef: path, Body: "A"}, collaborationTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CollaborationPreview(ctx, root, CollaborationChange{Action: "append", NoteRef: path, Body: "B"}, collaborationTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan string, 2)
	for _, p := range []CollaborationPreview{p1, p2} {
		wg.Add(1)
		go func(p CollaborationPreview) {
			defer wg.Done()
			v, err := NewService().CollaborationApply(ctx, root, p.Change, p.Digest, "explicit_instruction", collaborationTestPolicy)
			if err != nil {
				results <- domain.ErrorCode(err)
			} else {
				results <- string(v.Status)
			}
		}(p)
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for code := range results {
		if code == "succeeded" {
			succeeded++
		} else if code != "lock_held" && code != "revision_conflict" {
			t.Fatal("unexpected concurrent result", code)
		}
	}
	if succeeded != 1 {
		t.Fatal("expected exactly one save")
	}
}

func TestCollaborationPreservesFrontmatterAndRejectsSymlinks(t *testing.T) {
	s, root, path := collaborationFixture(t)
	ctx := context.Background()
	content, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "---\n", "---\ncustom:\n  nested: preserved\n", 1))
	if err := os.WriteFile(filepath.Join(root, path), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(root, path), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := s.CollaborationPreview(ctx, root, CollaborationChange{Action: "replace", NoteRef: path, Body: "\n# Replacement\n\n````\n"}, collaborationTestPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CollaborationApply(ctx, root, p.Change, p.Digest, "explicit_instruction", collaborationTestPolicy); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, path))
	if !strings.Contains(string(got), "nested: preserved") || !strings.Contains(string(got), "# Replacement") {
		t.Fatal("lost unrelated frontmatter or body")
	}
	info, _ := os.Stat(filepath.Join(root, path))
	if info.Mode().Perm() != 0o600 {
		t.Fatal("changed note permissions")
	}
	out := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(out, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if err := collaborationSafePath(root, "escape/new.md"); domain.ErrorCode(err) != "unsafe_path" {
		t.Fatal("followed external symlink")
	}
	if err := collaborationSafePath(root, "../outside.md"); err == nil {
		t.Fatal("accepted escaping path")
	}
}

func TestCollaborationLimitsPaginationAndCheckpoint(t *testing.T) {
	s, root, path := collaborationFixture(t)
	ctx := context.Background()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CollaborationRead(ctx, relative, path, "read", collaborationTestPolicy); err != nil {
		t.Fatal("relative vault read failed", err)
	}
	if _, err := s.CollaborationPreview(ctx, root, CollaborationChange{Action: "append", NoteRef: path, Body: strings.Repeat("x", CollaborationMaxBodyBytes+1)}, collaborationTestPolicy); domain.ErrorCode(err) != "body_invalid" {
		t.Fatal("body limit not enforced")
	}
	for _, title := range []string{"Example B", "Example C"} {
		if _, err := s.CreateNote(ctx, CreateNoteRequest{VaultPath: root, Title: title, Body: "page fixture", Dir: "index"}); err != nil {
			t.Fatal(err)
		}
	}
	p1, err := s.CollaborationSearch(ctx, root, "Example", 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CollaborationSearch(ctx, root, "Example", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(p1.Notes) != 1 || len(p2.Notes) != 1 || p1.Notes[0].Path == p2.Notes[0].Path || !p1.HasMore || !p1.Truncated {
		t.Fatal("pagination did not advance")
	}
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	c := CollaborationChange{OperationID: "checkpoint-test", Action: "append", NoteRef: path}
	r := collaborationOperation(root, c, collaborationDigest(root, c))
	if _, err := store.CreateAccepted(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckpointApplying(ctx, c.OperationID, operation.Outcome{}); !operation.IsCode(err, operation.CodeIllegalTransition) {
		t.Fatal("checkpoint advanced an unclaimed operation")
	}
	if _, err := store.StartApplying(ctx, c.OperationID); err != nil {
		t.Fatal(err)
	}
	if err := store.CheckpointApplying(ctx, c.OperationID, operation.Outcome{ReceiptRef: "snapshot:test", ResourceRef: "pinax://note/test", Result: json.RawMessage(`{"path":"index/test.md"}`)}); err != nil {
		t.Fatal(err)
	}
	v, err := NewService().CollaborationStatus(ctx, root, c.OperationID)
	if err != nil || v.ReceiptRef != "snapshot:test" || v.Status != operation.StatusApplying {
		t.Fatal("reconnect lost pre-write recovery reference", err)
	}
}
