package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yeisme/pinax/internal/operation"
)

func TestWriterDailyRelocatedIdentityReadOnly(t *testing.T) {
	root := t.TempDir()
	svc := NewService()
	req := dailyWriterRequestFixture(root, "relocated-day")
	preview, err := svc.WriterDailyPropose(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	view, err := svc.WriterDailyApply(context.Background(), req, preview, "reviewed_proposal")
	if err != nil || view.Status != operation.StatusSucceeded {
		t.Fatal("creation failed", err)
	}
	relocated := "notes/archive/original-day.md"
	if err = os.MkdirAll(filepath.Dir(filepath.Join(root, relocated)), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(root, preview.Path), filepath.Join(root, relocated)); err != nil {
		t.Fatal(err)
	}
	template := "---\nschema_version: pinax.template.v2\nkind: journal_template\nname: journal.daily\nengine: go-template\noutput:\n  path_pattern: changed/{{ .Date }}.md\n---\n{{ .Date }}\n"
	writeAppFixture(t, filepath.Join(root, ".pinax/templates/journal.daily.md"), template)
	snapshot := func() map[string]string {
		result := map[string]string{}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			if entry.IsDir() {
				result[rel] = "directory"
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[rel] = collaborationRevision(string(raw))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := snapshot()
	read := ReaderDailyRequest{VaultPath: root, Session: req.Session, Date: req.Date, Timezone: req.Timezone}
	for range 2 {
		day, err := NewService().ReaderDaily(context.Background(), read)
		if err != nil || day.Note == nil || day.Note.ObjectID != preview.ObjectID {
			t.Fatal("original identity lost after relocation", err)
		}
		projection, err := svc.ReaderDailyDocument(context.Background(), ReaderDailyDocumentRequest{Document: ReaderDocumentRequest{VaultPath: root, Session: req.Session, ObjectID: day.Note.ObjectID, Revision: day.Note.Revision, CursorSecret: readerCursorSecret()}, Date: req.Date, Timezone: req.Timezone})
		if err != nil {
			t.Fatal("relocated body unavailable", err)
		}
		page := projection.Data.(map[string]any)["document"].(ReaderDocumentPage)
		if page.ObjectID != preview.ObjectID || page.Revision != day.Note.Revision || len(page.Nodes) == 0 {
			t.Fatal("relocated body identity mismatch")
		}
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("lookup changed source tree including SQLite sidecars")
	}
	req.OperationID = "no-duplicate"
	if _, err = svc.WriterDailyPropose(context.Background(), req); err == nil {
		t.Fatal("relocated journal allowed second proposal")
	}
	// A receipt never bypasses current note visibility.
	if err = os.Rename(filepath.Join(root, relocated), filepath.Join(root, ".hidden-original.md")); err != nil {
		t.Fatal(err)
	}
	day, err := svc.ReaderDaily(context.Background(), read)
	if err != nil || day.Note != nil || day.Status != "missing" {
		t.Fatal("hidden historical object disclosed", err)
	}
	if err = os.Rename(filepath.Join(root, ".hidden-original.md"), filepath.Join(root, relocated)); err != nil {
		t.Fatal(err)
	}
	navigationNote(t, root, "changed/"+req.Date+".md", "different-day", "Another", "daily", "daily", "synthetic")
	_, err = svc.ReaderDaily(context.Background(), read)
	requireEditorSourceError(t, err, "reader_unsupported")
}
