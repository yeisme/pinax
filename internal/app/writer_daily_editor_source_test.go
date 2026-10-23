package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriterEditorSourceDailyLosslessAndGates(t *testing.T) {
	for _, rel := range []string{"daily/2026-08-20.md", "notes/daily/2026-08-20.md"} {
		t.Run(rel, func(t *testing.T) {
			root := t.TempDir()
			body := "\n\t中文 **journal**  \r\n\n---\nbody separator\n\n"
			content := editorSourceFixture(t, root, rel, "daily-editor", body)
			content = strings.Replace(content, "kind: reference", "kind: daily", 1)
			path := filepath.Join(root, rel)
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			source := editorSourceRequest(root, "daily-editor", content)
			source.Session.Scopes = append(source.Session.Scopes, ReaderScopeMetadata)
			req := WriterDailyEditorSourceRequest{Source: source, Date: "2026-08-20", Timezone: "UTC"}
			svc := NewService()
			if _, err := svc.WriterEditorSource(context.Background(), source); err == nil {
				t.Fatal("ordinary source admitted system daily")
			}
			result, err := svc.WriterDailyEditorSource(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if result.Body != body || result.BodyDigest != collaborationRevision(body) || !result.Editable || !result.FrontmatterProtected || result.ProtectedMetadata.Kind != "daily" || result.Revision != source.Revision {
				t.Fatal("daily source was normalized or identity lost")
			}
			encoded, err := json.Marshal(result)
			if err != nil || strings.Contains(string(encoded), "never-return-this-field") || strings.Contains(string(encoded), root) {
				t.Fatal("private metadata leaked")
			}
			for _, scopes := range [][]string{{ReaderScopeBody, ReaderScopeMetadata}, {WriterScopeEditor, ReaderScopeMetadata}, {WriterScopeEditor, ReaderScopeBody}} {
				bad := req
				bad.Source.Session.Scopes = scopes
				_, err := svc.WriterDailyEditorSource(context.Background(), bad)
				requireEditorSourceError(t, err, "insufficient_scope")
			}
			for _, kind := range []string{"id", "date", "revision"} {
				bad := req
				switch kind {
				case "id":
					bad.Source.ObjectID = "other"
				case "date":
					bad.Date = "2026-08-21"
				case "revision":
					bad.Source.Revision = "sha256:" + strings.Repeat("0", 64)
				}
				if _, err := svc.WriterDailyEditorSource(context.Background(), bad); err == nil {
					t.Fatal("identity fence failed", kind)
				}
			}
			unchanged, err := os.ReadFile(path)
			if err != nil || string(unchanged) != content {
				t.Fatal("source changed")
			}
			if _, err = os.Stat(filepath.Join(root, ".pinax")); !os.IsNotExist(err) {
				t.Fatal("editor read initialized vault")
			}
			if err = os.WriteFile(path, []byte(content+"changed"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err = svc.WriterDailyEditorSource(context.Background(), req)
			requireEditorSourceError(t, err, "revision_conflict")
		})
	}
}

func TestWriterEditorSourceDailyBudgetAndNonDaily(t *testing.T) {
	for _, kind := range []string{"large", "empty", "wrong-kind", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			body := ""
			if kind == "large" {
				body = strings.Repeat("a", 65536)
			}
			content := editorSourceFixture(t, root, "daily/2026-08-20.md", "daily-editor", body)
			if kind != "wrong-kind" {
				content = strings.Replace(content, "kind: reference", "kind: daily", 1)
			}
			if err := os.WriteFile(filepath.Join(root, "daily/2026-08-20.md"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if kind == "duplicate" {
				writeAppFixture(t, filepath.Join(root, "notes/copy.md"), content)
			}
			source := editorSourceRequest(root, "daily-editor", content)
			source.Session.Scopes = append(source.Session.Scopes, ReaderScopeMetadata)
			result, err := NewService().WriterDailyEditorSource(context.Background(), WriterDailyEditorSourceRequest{Source: source, Date: "2026-08-20", Timezone: "UTC"})
			switch kind {
			case "large":
				requireEditorSourceError(t, err, "body_too_large")
			case "empty":
				if err != nil || result.Body != "" {
					t.Fatal("empty source changed", err)
				}
			default:
				if err == nil {
					t.Fatal("non-unique/non-daily admitted")
				}
			}
		})
	}
}
