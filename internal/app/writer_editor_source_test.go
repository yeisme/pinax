package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/domain"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

func editorSourceFixture(t *testing.T, root, path, id, body string) string {
	t.Helper()
	content := "---\nschema_version: pinax.note.v1\nnote_id: " + id + "\ntitle: Editor fixture\nkind: reference\ntags: [reading, 中文]\nprivate_custom: never-return-this-field\n---\n" + body
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return content
}

func editorSourceRequest(root, id, content string) WriterEditorSourceRequest {
	return WriterEditorSourceRequest{VaultPath: root, ObjectID: id, Revision: collaborationRevision(content), Session: ReaderSessionInfo{SessionID: "editor-fixture", BindingID: "binding-fixture", Scopes: []string{WriterScopeEditor, ReaderScopeBody}}}
}

func requireEditorSourceError(t *testing.T, err error, code string) {
	t.Helper()
	var commandErr *domain.CommandError
	if !errors.As(err, &commandErr) || commandErr.Code != code {
		t.Fatalf("expected %s, got %v", code, err)
	}
}

func TestWriterEditorSourceLosslessReadOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	body := "\n\t中文 **正文**  \r\n\n---\nnot frontmatter\n\n"
	content := editorSourceFixture(t, root, "notes/editor.md", "note_editor", body)
	source, err := NewService().WriterEditorSource(context.Background(), editorSourceRequest(root, "note_editor", content))
	if err != nil {
		t.Fatal(err)
	}
	if source.Body != body || source.BodyDigest != collaborationRevision(body) || !source.Editable || !source.FrontmatterProtected || source.Budget.BodyBytes != len(body) || source.Budget.SourceBytes != len(content) {
		t.Fatal("source was normalized or budget/identity drifted")
	}
	encoded, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), root) || strings.Contains(string(encoded), "never-return-this-field") || strings.Contains(source.Body, "schema_version:") {
		t.Fatal("editor source leaked path or protected frontmatter")
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := catalogschema.ValidatePayload(catalogschema.Default(), catalogschema.WriterEditorSourceV1Alpha1, decoded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("editor read created owner state")
	}
	unchanged, err := os.ReadFile(filepath.Join(root, "notes/editor.md"))
	if err != nil || string(unchanged) != content {
		t.Fatal("editor read changed source")
	}
}

func TestWriterEditorSourceScopesRevisionsAndBudgets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	content := editorSourceFixture(t, root, "notes/editor.md", "note_editor", " body ")
	svc := NewService()
	for _, scopes := range [][]string{nil, {ReaderScopeBody}, {WriterScopeEditor}, {ReaderScopeMetadata, WriterScopeCapture}, {"read", "write", "admin"}} {
		req := editorSourceRequest(root, "note_editor", content)
		req.Session.Scopes = scopes
		_, err := svc.WriterEditorSource(context.Background(), req)
		requireEditorSourceError(t, err, "insufficient_scope")
	}
	req := editorSourceRequest(root, "note_editor", "stale")
	_, err := svc.WriterEditorSource(context.Background(), req)
	requireEditorSourceError(t, err, "revision_conflict")
	req.Revision = "not-a-revision"
	_, err = svc.WriterEditorSource(context.Background(), req)
	requireEditorSourceError(t, err, "writer_request_invalid")
	content = editorSourceFixture(t, root, "notes/editor.md", "note_editor", strings.Repeat("字", catalogschema.WriterMaxBodyBytes/3))
	_, err = svc.WriterEditorSource(context.Background(), editorSourceRequest(root, "note_editor", content))
	requireEditorSourceError(t, err, "body_too_large")
	content = editorSourceFixture(t, root, "notes/editor.md", "note_editor", "bad\x00body")
	_, err = svc.WriterEditorSource(context.Background(), editorSourceRequest(root, "note_editor", content))
	requireEditorSourceError(t, err, "body_invalid")
	content = editorSourceFixture(t, root, "notes/editor.md", "note_editor", string([]byte{0xff}))
	_, err = svc.WriterEditorSource(context.Background(), editorSourceRequest(root, "note_editor", content))
	requireEditorSourceError(t, err, "body_invalid")
}

func TestWriterEditorSourceBoundaryAndEmpty(t *testing.T) {
	t.Parallel()
	for _, size := range []int{0, catalogschema.WriterMaxBodyBytes} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			root := t.TempDir()
			prefix := editorSourceFixture(t, root, "notes/editor.md", "note_editor", "")
			body := ""
			if size > 0 {
				body = strings.Repeat("a", size-len(prefix))
			}
			content := editorSourceFixture(t, root, "notes/editor.md", "note_editor", body)
			source, err := NewService().WriterEditorSource(context.Background(), editorSourceRequest(root, "note_editor", content))
			if err != nil || source.Body != body || !source.Editable {
				t.Fatalf("exact boundary failed: %v", err)
			}
		})
	}
}

func TestWriterEditorSourceVisibilityIdentityAndSymlinks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	content := editorSourceFixture(t, root, "notes/editor.md", "note_editor", "visible")
	_ = editorSourceFixture(t, root, ".hidden/secret.md", "secret_hidden", "private")
	_ = editorSourceFixture(t, root, "notes/.hidden.md", "secret_hidden_file", "private")
	outside := t.TempDir()
	_ = editorSourceFixture(t, outside, "secret.md", "secret_outside", "outside")
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(root, "notes/link.md")); err != nil {
		t.Skip("symlink permission unavailable")
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	svc := NewService()
	for _, id := range []string{"secret_hidden", "secret_hidden_file", "secret_outside", "missing"} {
		_, err := svc.WriterEditorSource(context.Background(), editorSourceRequest(root, id, content))
		requireEditorSourceError(t, err, "note_not_found")
	}
	_ = editorSourceFixture(t, root, "notes/duplicate.md", "note_editor", "duplicate")
	_, err := svc.WriterEditorSource(context.Background(), editorSourceRequest(root, "note_editor", content))
	requireEditorSourceError(t, err, "note_identity_ambiguous")
	opened, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = opened.Close() }()
	if _, err := readWriterEditorCandidate(opened, "notes/link.md"); err == nil {
		t.Fatal("symlink opened by direct reader")
	}
	if _, err := readWriterEditorCandidate(opened, "linked/secret.md"); err == nil {
		t.Fatal("symlink ancestry opened by direct reader")
	}
}

func TestWriterEditorSourceCRLFDoesNotExposeMetadataAsBody(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	content := editorSourceFixture(t, root, "notes/editor.md", "note_editor", "body\n")
	content = strings.ReplaceAll(content, "\n", "\r\n")
	if err := os.WriteFile(filepath.Join(root, "notes/editor.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := NewService().WriterEditorSource(context.Background(), editorSourceRequest(root, "note_editor", content))
	if err != nil || source.Body != "body\r\n" || source.Editable || source.ReadonlyReason != "frontmatter_unsupported" {
		t.Fatalf("unsupported writer frontmatter must stay read-only: %v", err)
	}
}
