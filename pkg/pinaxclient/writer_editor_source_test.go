package pinaxclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yeisme/pinax/internal/app"
)

func TestWriterEditorSourceClientRoundTrip(t *testing.T) {
	t.Parallel()
	server, bearer := writerClientServer(t, app.ReaderScopeMetadata, app.ReaderScopeBody, app.WriterScopeEditor)
	client := writerClient(t, server.URL, bearer)
	ctx := context.Background()
	notes, err := client.ReaderNotes(ctx, ReaderPageQuery{})
	if err != nil {
		t.Fatal(err)
	}
	var id, revision string
	for _, note := range notes.Items {
		if note.Title == "Alpha" {
			id, revision = note.ObjectID, note.Revision
		}
	}
	if id == "" {
		t.Fatal("fixture note was not discovered")
	}
	source, err := client.WriterEditorSource(ctx, id, revision)
	if err != nil {
		t.Fatal(err)
	}
	if !source.Editable || source.Body != "\nAlpha body zq-writer-client.\n" {
		t.Fatal("editor source changed whitespace")
	}
	body := source.Body + "\n\t中文编辑  \n"
	preview, err := client.WriterPropose(ctx, WriterChange{Action: "replace", OperationID: "editor-source-test", NoteRef: id, ExpectedRevision: source.Revision, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Before != source.Body || preview.After != body {
		t.Fatal("preview differs from exact source")
	}
	receipt, err := client.WriterApply(ctx, WriterApplyRequest{Change: preview.Change, PreviewDigest: preview.PreviewDigest, Authorization: "reviewed_proposal"})
	if err != nil || receipt.Status != "succeeded" {
		t.Fatalf("confirmed apply failed: %v", err)
	}
	updated, err := client.WriterEditorSource(ctx, id, receipt.RevisionAfter)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Body != body || updated.ProtectedMetadata.NoteID != source.ProtectedMetadata.NoteID || updated.ProtectedMetadata.Title != source.ProtectedMetadata.Title {
		t.Fatal("save lost body or metadata")
	}
	if _, err := client.WriterEditorSource(ctx, id, revision); err == nil {
		t.Fatal("stale editor source accepted")
	}
	replayed, err := client.WriterApply(ctx, WriterApplyRequest{Change: preview.Change, PreviewDigest: preview.PreviewDigest, Authorization: "reviewed_proposal"})
	if err != nil || replayed.RevisionAfter != receipt.RevisionAfter {
		t.Fatal("same apply did not replay receipt")
	}
}

func editorClientPayload() map[string]any {
	body := "\nbody\n"
	digest := sha256.Sum256([]byte(body))
	return map[string]any{
		"schema_version": WriterEditorSourceSchemaV1Alpha1, "object_id": "note-one", "revision": "sha256:" + strings.Repeat("a", 64),
		"body": body, "body_digest": "sha256:" + hex.EncodeToString(digest[:]), "editable": true, "readonly_reason": "none", "frontmatter_protected": true,
		"protected_metadata": map[string]any{"note_id": "note-one", "title": "Fixture", "tags": []string{}, "kind": "reference", "status": "active"},
		"budget":             map[string]any{"max_source_bytes": 65536, "max_body_bytes": 65536, "source_bytes": 128, "body_bytes": len(body)},
	}
}

func TestWriterEditorSourceClientRejectsInvalidResponses(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"schema", "object", "revision", "digest", "metadata", "budget", "readonly", "unknown-field"} {
		t.Run(name, func(t *testing.T) {
			data := editorClientPayload()
			switch name {
			case "schema":
				data["schema_version"] = "future"
			case "object":
				data["object_id"] = "other"
			case "revision":
				data["revision"] = "sha256:" + strings.Repeat("b", 64)
			case "digest":
				data["body_digest"] = "sha256:" + strings.Repeat("b", 64)
			case "metadata":
				data["protected_metadata"].(map[string]any)["note_id"] = "other"
			case "budget":
				data["budget"].(map[string]any)["body_bytes"] = 1
			case "readonly":
				data["readonly_reason"] = "frontmatter_unsupported"
			case "unknown-field":
				data["vault_path"] = "not-allowed"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/v1/writer/notes/note-one/editor-source" {
					t.Error("wrong request route")
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "command": "writer.editor_source", "data": data})
			}))
			defer server.Close()
			client := writerClient(t, server.URL, "synthetic-fixture-token")
			if _, err := client.WriterEditorSource(context.Background(), "note-one", "sha256:"+strings.Repeat("a", 64)); err == nil {
				t.Fatal("invalid source response accepted")
			}
		})
	}
}

func TestWriterEditorSourceClientInvalidRequestsStayLocal(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	client := writerClient(t, server.URL, "synthetic-fixture-token")
	for _, request := range [][2]string{{"", "sha256:" + strings.Repeat("a", 64)}, {"note-one", ""}, {"../note", "sha256:" + strings.Repeat("a", 64)}, {"note-one", "latest"}} {
		if _, err := client.WriterEditorSource(context.Background(), request[0], request[1]); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached transport")
	}
}
