package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
	pinaxversion "github.com/yeisme/pinax/internal/version"
)

func writeDailyTemplateFixture(t *testing.T, root, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".pinax/templates"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pinax/templates/journal.daily.md"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWriterDailyEmptyTemplateAndWireBudget(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", " ", strings.Repeat("<", 24000), "自定义\n\n"} {
		root := t.TempDir()
		ctx := context.Background()
		req := dailyWriterRequestFixture(root, "daily-template")
		writeDailyTemplateFixture(t, root, body)
		preview, err := NewService().WriterDailyPropose(ctx, req)
		if len(body) > 20000 {
			requireEditorSourceError(t, err, "body_too_large")
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		view, err := NewService().WriterDailyApply(ctx, req, preview, "reviewed_proposal")
		if err != nil || view.Status != operation.StatusSucceeded {
			t.Fatalf("empty template apply: %v %s", err, view.Status)
		}
		raw, err := os.ReadFile(filepath.Join(root, preview.Path))
		if err != nil {
			t.Fatal(err)
		}
		_, actual := splitFrontmatter(string(raw))
		if actual != preview.After {
			t.Fatal("empty template preview differs from saved body")
		}
	}
}

func TestWriterDailyUnsupportedTemplateMetadataIsNotBody(t *testing.T) {
	t.Parallel()
	for _, text := range []string{strings.ReplaceAll(builtInTemplates()["journal.daily"], "\n", "\r\n"), "\ufeff" + builtInTemplates()["journal.daily"], "---\nschema_version: unsupported\nprivate_key: fixture-only\n---\nbody"} {
		root := t.TempDir()
		writeDailyTemplateFixture(t, root, text)
		_, err := NewService().WriterDailyPropose(context.Background(), dailyWriterRequestFixture(root, "daily-metadata"))
		requireEditorSourceError(t, err, "template_unsupported")
		if strings.Contains(err.Error(), "fixture-only") {
			t.Fatal("template error leaked metadata")
		}
	}
}

func TestWriterDailyOriginalIdentityAndVaultIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	req := dailyWriterRequestFixture(root, "daily-identity")
	svc := NewService()
	p1, err := svc.WriterDailyPropose(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := svc.WriterDailyPropose(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	other := req
	other.VaultPath = t.TempDir()
	if _, err := svc.WriterDailyApply(ctx, other, p1, "reviewed_proposal"); err == nil {
		t.Fatal("proposal crossed vaults")
	}
	other = req
	other.PreviewSecret = []byte("different-private-session-secret")
	if _, err := svc.WriterDailyApply(ctx, other, p1, "reviewed_proposal"); err == nil {
		t.Fatal("proposal survived secret rotation")
	}
	view, err := svc.WriterDailyApply(ctx, req, p1, "reviewed_proposal")
	if err != nil || view.Status != operation.StatusSucceeded {
		t.Fatalf("first apply failed: %v", err)
	}
	if _, err := svc.WriterDailyApply(ctx, req, p2, "reviewed_proposal"); !operation.IsCode(err, operation.CodeIdempotencyConflict) {
		t.Fatalf("new body identity reused original operation: %v", err)
	}
	other = req
	other.Date = "2026-08-21"
	p3, err := svc.WriterDailyPropose(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WriterDailyApply(ctx, other, p3, "reviewed_proposal"); !operation.IsCode(err, operation.CodeIdempotencyConflict) {
		t.Fatal("same operation created another day")
	}
}

func TestWriterDailyMetadataLinksFailClosed(t *testing.T) {
	t.Parallel()
	for _, rel := range []string{".pinax/events.jsonl", ".pinax/last_snapshot", ".pinax/records/events.jsonl", ".pinax/version/objects/tmp", ".pinax/api/operations.sqlite"} {
		t.Run(rel, func(t *testing.T) {
			root := t.TempDir()
			ctx := context.Background()
			req := dailyWriterRequestFixture(root, "daily-link")
			svc := NewService()
			preview, err := svc.WriterDailyPropose(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("unchanged"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Join(root, rel)), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, rel)); err != nil {
				t.Skip("symlinks unavailable")
			}
			_, err = svc.WriterDailyApply(ctx, req, preview, "reviewed_proposal")
			requireEditorSourceError(t, err, "unsafe_path")
			raw, err := os.ReadFile(outside)
			if err != nil || string(raw) != "unchanged" {
				t.Fatal("metadata link wrote outside vault")
			}
		})
	}
}

func TestWriterDailyReconcileDoesNotInterfereWithApply(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := context.Background()
	req := dailyWriterRequestFixture(root, "daily-busy")
	svc := NewService()
	preview, err := svc.WriterDailyPropose(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	backend := dailySnapshotHook{VersionBackend: pinaxversion.NewLocalBackend(), hook: func() {
		if _, err := svc.OperationReconcile(ctx, OperationRequest{VaultPath: root, OperationID: req.OperationID, Access: OperationAccess{OwnerLocal: true}}); err == nil {
			t.Fatal("reconcile took over active apply")
		}
	}}
	view, err := NewServiceWithVersionBackend(backend).WriterDailyApply(ctx, req, preview, "reviewed_proposal")
	if err != nil || view.Status != operation.StatusSucceeded {
		t.Fatalf("apply interrupted by reconciliation: %v", err)
	}
}

func TestWriterDailyPathChangeCannotDuplicateDate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	req := dailyWriterRequestFixture(root, "daily-path-one")
	preview, err := svc.WriterDailyPropose(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.WriterDailyApply(ctx, req, preview, "reviewed_proposal")
	if err != nil || view.Status != operation.StatusSucceeded {
		t.Fatal("initial daily failed", err)
	}
	writeDailyTemplateFixture(t, root, strings.Replace(builtInTemplates()["journal.daily"], "daily/{{ .Date }}.md", "journal/{{ .Date }}.md", 1))
	req.OperationID = "daily-path-two"
	// Original identity lookup now rejects at proposal time, before apply.
	_, err = svc.WriterDailyPropose(ctx, req)
	requireEditorSourceError(t, err, "daily_exists")
	if _, err := os.Stat(filepath.Join(root, "journal", req.Date+".md")); !os.IsNotExist(err) {
		t.Fatal("path change duplicated a calendar date")
	}
}

func TestWriterDailyLedgerPaginationAndUnresolvedFence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	store, err := operation.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	for i := 0; i < 202; i++ {
		id := fmt.Sprintf("daily-page-%03d", i)
		req := operation.CreateRequest{OperationID: id, IdempotencyKey: id, CapabilityID: writerDailyApplyCapability, BindingID: "rest.writer.daily.apply", PrincipalDigest: operation.IdentityDigest("fixture"), ScopeDigest: operation.IdentityDigest("fixture-scopes"), RequestDigest: operation.IdentityDigest(id)}
		if _, err := store.CreateAccepted(ctx, req); err != nil {
			t.Fatal(err)
		}
		date := "2026-08-21"
		if i == 201 {
			date = "2026-08-20"
		}
		result, _ := json.Marshal(dailyOperationResult{Date: date})
		if _, err := store.StartApplyingWithOutcome(ctx, id, operation.Outcome{Result: result}); err != nil {
			t.Fatal(err)
		}
		if i < 201 {
			if _, err := store.CompleteFailed(ctx, id, operation.Outcome{ErrorCode: "fixture_rejection"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := dailyLedgerAvailable(ctx, store, "2026-08-22"); err != nil {
		t.Fatal(err)
	}
	requireEditorSourceError(t, dailyLedgerAvailable(ctx, store, "2026-08-20"), "reconcile_required")
}

func TestWriterDailyTemplateChangedDuringSnapshot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := context.Background()
	req := dailyWriterRequestFixture(root, "daily-late-template")
	svc := NewService()
	preview, err := svc.WriterDailyPropose(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	backend := dailySnapshotHook{VersionBackend: pinaxversion.NewLocalBackend(), hook: func() { writeDailyTemplateFixture(t, root, "changed after confirmation") }}
	view, err := NewServiceWithVersionBackend(backend).WriterDailyApply(ctx, req, preview, "reviewed_proposal")
	if err != nil || view.Status != operation.StatusFailed || view.Error == nil || view.Error.Code != "preview_mismatch" {
		t.Fatalf("changed template was published: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, preview.Path)); !os.IsNotExist(err) {
		t.Fatal("template drift created file")
	}
	replay, err := svc.WriterDailyApply(ctx, req, preview, "reviewed_proposal")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(view)
	after, _ := json.Marshal(replay)
	if string(before) != string(after) {
		t.Fatal("terminal rejection did not replay")
	}
}
