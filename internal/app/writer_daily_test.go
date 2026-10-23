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
	"github.com/yeisme/pinax/internal/records"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
	pinaxversion "github.com/yeisme/pinax/internal/version"
	"github.com/yeisme/pinax/internal/version/versiontest"
)

func dailyWriterRequestFixture(root, id string) WriterDailyRequest {
	return WriterDailyRequest{VaultPath: root, OperationID: id, Date: "2026-08-20", Timezone: "Asia/Shanghai", Session: ReaderSessionInfo{SessionID: "daily-fixture", BindingID: "daily-binding", Scopes: []string{WriterScopeEditor, ReaderScopeBody, ReaderScopeMetadata}}, PreviewSecret: []byte("synthetic-daily-preview-secret-only")}
}

func TestWriterDailyProposalAndConfirmedReceipt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	svc := NewService()
	req := dailyWriterRequestFixture(root, "daily-one")
	preview, err := svc.WriterDailyPropose(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 0 {
		t.Fatal("proposal mutated the vault")
	}
	if preview.Path != "daily/2026-08-20.md" || !preview.FrontmatterProtected || preview.Before != "" || !strings.Contains(preview.After, req.Date) {
		t.Fatal("daily proposal identity drift")
	}
	encoded, _ := json.Marshal(preview)
	var payload any
	if json.Unmarshal(encoded, &payload) != nil || catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterDailyProposalV1Alpha1, payload) != nil {
		t.Fatal("invalid proposal schema")
	}
	view, err := svc.WriterDailyApply(ctx, req, preview, "reviewed_proposal")
	if err != nil || view.Status != operation.StatusSucceeded {
		t.Fatalf("daily apply: %s %v", view.Status, err)
	}
	content, err := os.ReadFile(filepath.Join(root, preview.Path))
	if err != nil {
		t.Fatal(err)
	}
	_, body := splitFrontmatter(string(content))
	if body != preview.After || view.RevisionAfter != collaborationRevision(string(content)) || !strings.HasPrefix(view.ReceiptRef, "snapshot:") {
		t.Fatal("receipt or exact body mismatch")
	}
	note := parseNote(preview.Path, string(content))
	if note.ID != preview.ObjectID || note.Kind != "daily" {
		t.Fatal("daily object changed")
	}
	day, err := svc.ReaderDaily(ctx, ReaderDailyRequest{VaultPath: root, Session: req.Session, Date: req.Date, Timezone: req.Timezone})
	if err != nil || day.Note == nil || day.Note.ObjectID != preview.ObjectID {
		t.Fatalf("created daily lookup failed: %v", err)
	}
	replayed, err := NewService().WriterDailyApply(ctx, req, preview, "reviewed_proposal")
	first, _ := json.Marshal(view)
	second, _ := json.Marshal(replayed)
	if err != nil || string(first) != string(second) {
		t.Fatalf("original receipt did not replay: %v", err)
	}
	store, err := operation.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	row, err := store.Get(ctx, req.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{string(row.ResultJSON), string(row.ReceiptJSON)} {
		if strings.Contains(value, "Today's Focus") || strings.Contains(value, root) {
			t.Fatal("ledger contains body or absolute root")
		}
	}
	journals, err := filepath.Glob(filepath.Join(root, "daily", "*.md"))
	if err != nil || len(journals) != 1 {
		t.Fatal("creation also modified another day")
	}
}

func TestWriterDailyDeniedTamperDriftAndExisting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, name := range []string{"scope", "date", "timezone", "tamper", "binding", "session", "authorization", "template", "target", "legacy", "symlink", "budget", "queries"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			svc := NewService()
			req := dailyWriterRequestFixture(root, "daily-reject")
			preview, err := svc.WriterDailyPropose(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			authorization := "reviewed_proposal"
			want := "preview_mismatch"
			switch name {
			case "scope":
				req.Session.Scopes = []string{WriterScopeEditor, ReaderScopeBody}
				want = "insufficient_scope"
			case "date":
				req.Date = "2026-02-30"
				_, err = svc.WriterDailyPropose(ctx, req)
				requireEditorSourceError(t, err, "reader_request_invalid")
				return
			case "timezone":
				req.Timezone = "Local"
				_, err = svc.WriterDailyPropose(ctx, req)
				requireEditorSourceError(t, err, "reader_request_invalid")
				return
			case "tamper":
				preview.After += "not reviewed"
			case "binding":
				req.Session.BindingID = "other-binding"
			case "session":
				req.Session.SessionID = "other-session"
			case "authorization":
				authorization = ""
				want = "authorization_required"
			case "template", "budget", "queries":
				if err := os.MkdirAll(filepath.Join(root, ".pinax/templates"), 0o700); err != nil {
					t.Fatal(err)
				}
				text := builtInTemplates()["journal.daily"] + "\nchanged"
				if name == "budget" {
					text = strings.Repeat("字", 23000)
				}
				if name == "queries" {
					text = "---\nschema_version: pinax.template.v2\nengine: go-template\nqueries:\n  hidden:\n    sql: SELECT secret\n---\n# {{ .Date }}\n"
				}
				if err := os.WriteFile(filepath.Join(root, ".pinax/templates/journal.daily.md"), []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
				if name != "template" {
					_, err = svc.WriterDailyPropose(ctx, req)
					if err == nil {
						t.Fatal("unsafe template accepted")
					}
					return
				}
			case "target", "legacy":
				path := preview.Path
				if name == "legacy" {
					path = "notes/daily/" + req.Date + ".md"
				}
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, path), []byte(dailyProposalContent(preview)), 0o600); err != nil {
					t.Fatal(err)
				}
				want = "daily_exists"
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "daily")); err != nil {
					t.Skip("symlinks unavailable")
				}
				want = "unsafe_path"
			}
			_, err = svc.WriterDailyApply(ctx, req, preview, authorization)
			requireEditorSourceError(t, err, want)
		})
	}
}

func TestWriterDailyConcurrentFirstCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	one := dailyWriterRequestFixture(root, "daily-concurrent-one")
	two := dailyWriterRequestFixture(root, "daily-concurrent-two")
	svc := NewService()
	p1, err := svc.WriterDailyPropose(ctx, one)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := svc.WriterDailyPropose(ctx, two)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	statuses := make(chan string, 2)
	for _, item := range []struct {
		req WriterDailyRequest
		p   WriterDailyProposal
	}{{one, p1}, {two, p2}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			view, err := NewService().WriterDailyApply(ctx, item.req, item.p, "reviewed_proposal")
			if err != nil {
				statuses <- domain.ErrorCode(err)
			} else {
				statuses <- string(view.Status)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(statuses)
	success := 0
	for status := range statuses {
		if status == "succeeded" {
			success++
		} else if status != "lock_held" && status != "daily_exists" {
			t.Fatalf("unexpected concurrent outcome: %s", status)
		}
	}
	if success != 1 {
		t.Fatalf("successful creations = %d", success)
	}
	files, err := os.ReadDir(filepath.Join(root, "daily"))
	if err != nil || len(files) != 1 || files[0].Name() != one.Date+".md" {
		t.Fatal("duplicate or partial daily publication")
	}
}

type dailySnapshotHook struct {
	pinaxversion.VersionBackend
	hook func()
}

func (b dailySnapshotHook) Snapshot(ctx context.Context, req pinaxversion.SnapshotRequest) (pinaxversion.Snapshot, error) {
	snapshot, err := b.VersionBackend.Snapshot(ctx, req)
	if err == nil {
		b.hook()
	}
	return snapshot, err
}

func TestWriterDailySnapshotFailureAndLastMomentNoClobber(t *testing.T) {
	t.Parallel()
	for _, failure := range []bool{true, false} {
		root := t.TempDir()
		ctx := context.Background()
		req := dailyWriterRequestFixture(root, "daily-failure")
		preview, err := NewService().WriterDailyPropose(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		var backend pinaxversion.VersionBackend = &versiontest.FakeBackend{SnapshotErr: errors.New("fixture failure")}
		if !failure {
			backend = dailySnapshotHook{VersionBackend: pinaxversion.NewLocalBackend(), hook: func() {
				if err := os.MkdirAll(filepath.Join(root, "daily"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, preview.Path), []byte("externally created"), 0o600); err != nil {
					t.Fatal(err)
				}
			}}
		}
		view, err := NewServiceWithVersionBackend(backend).WriterDailyApply(ctx, req, preview, "reviewed_proposal")
		if err != nil {
			t.Fatal(err)
		}
		if failure {
			if view.Status != operation.StatusReconcileRequired {
				t.Fatal("snapshot failure did not preserve operation")
			}
			if _, err := os.Stat(filepath.Join(root, preview.Path)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("published without snapshot")
			}
		} else {
			data, err := os.ReadFile(filepath.Join(root, preview.Path))
			if err != nil || string(data) != "externally created" || view.Status != operation.StatusFailed {
				t.Fatal("publish overwrote external target")
			}
		}
	}
}

func TestWriterDailyInterruptedApplyReconcile(t *testing.T) {
	t.Parallel()
	for _, proof := range []string{"record", "no-record", "changed"} {
		t.Run(proof, func(t *testing.T) {
			root := t.TempDir()
			ctx := context.Background()
			svc := NewService()
			req := dailyWriterRequestFixture(root, "daily-interrupted")
			preview, err := svc.WriterDailyPropose(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			access := WriterOperationAccess(req.Session.SessionID, req.Session.BindingID, req.Session.Scopes)
			store, err := operation.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			_, err = store.CreateAccepted(ctx, operation.CreateRequest{OperationID: req.OperationID, IdempotencyKey: req.OperationID, CapabilityID: writerDailyApplyCapability, BindingID: "rest.writer.daily.apply", RequestDigest: preview.PreviewDigest, PrincipalDigest: access.Access.PrincipalDigest, ScopeDigest: access.Access.ScopeDigest})
			if err != nil {
				t.Fatal(err)
			}
			content := dailyProposalContent(preview)
			result, _ := json.Marshal(dailyOperationResult{Date: req.Date, Timezone: req.Timezone, NoteID: preview.ObjectID, Path: preview.Path, ContentDigest: collaborationRevision(content)})
			snapshot, err := svc.VersionSnapshot(ctx, SnapshotRequest{VaultPath: root, Message: "Fixture crash window"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.StartApplyingWithOutcome(ctx, req.OperationID, operation.Outcome{Result: result, ReceiptRef: "snapshot:" + snapshot.Facts["snapshot_id"]})
			if err != nil {
				t.Fatal(err)
			}
			if err := publishDailyWithoutReplace(root, preview.Path, []byte(content)); err != nil {
				t.Fatal(err)
			}
			if proof != "no-record" {
				if _, err := appendNoteRecordEvent(ctx, root, domain.RecordEventNoteCreated, "writer.daily:"+req.OperationID, parseNote(preview.Path, content), ""); err != nil {
					t.Fatal(err)
				}
			}
			if proof == "changed" {
				if err := os.WriteFile(filepath.Join(root, preview.Path), []byte(content+"external edit"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.RequireReconcile(ctx, req.OperationID, operation.Outcome{ErrorCode: "outcome_unknown", ErrorMessage: "Fixture interruption"}); err != nil {
				t.Fatal(err)
			}
			_, err = svc.OperationReconcile(ctx, OperationRequest{VaultPath: root, OperationID: req.OperationID, Access: OperationAccess{OwnerLocal: true}})
			if err != nil {
				t.Fatal(err)
			}
			row, err := store.Get(ctx, req.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			want := operation.StatusReconcileRequired
			if proof == "record" {
				want = operation.StatusSucceeded
			}
			if want == operation.StatusSucceeded && operation.ViewFromRow(row).Error != nil {
				t.Fatal("resolved receipt retained ambiguity error")
			}
			if row.Status != want {
				t.Fatalf("reconcile = %s, want %s", row.Status, want)
			}
			view, err := svc.WriterDailyApply(ctx, req, preview, "reviewed_proposal")
			if err != nil || view.Status != want {
				t.Fatalf("replay repeated interrupted creation: %v", err)
			}
			events, err := records.NewService(root).Replay(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if proof == "no-record" && len(events.Records) != 0 {
				t.Fatal("reconcile invented a record")
			}
		})
	}
}
