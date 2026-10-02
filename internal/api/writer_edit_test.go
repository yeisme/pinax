package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yeisme/pinax/internal/app"
	catalogschema "github.com/yeisme/pinax/internal/transportcatalog/schema"
)

// TestWriterEdit covers the W03 supervised writer proposal lane: propose
// prepares an exact preview without writing; apply accepts only the
// unchanged preview digest with a caller-asserted authorization (never a
// verified approval), snapshots before mutating existing notes, conflicts
// explicitly on expected_revision drift, and replays the same receipt
// under the same operation identity. Fixture vault only.

func newEditorTestServer(t *testing.T, root string) (*httptest.Server, string, string) {
	t.Helper()
	sessions, err := NewReaderSessionStore()
	if err != nil {
		t.Fatal(err)
	}
	editorBearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	readerBearer, err := app.GenerateReaderBearer()
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-editor",
		BindingID: "binding-editor",
		Scopes:    []string{app.ReaderScopeBody, app.WriterScopeEditor},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, editorBearer); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Create(ReaderSession{
		SessionID: "sess-reader2",
		BindingID: "binding-reader2",
		Scopes:    []string{app.ReaderScopeBody},
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, readerBearer); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServerWithOptions(app.NewService(), root, ServerOptions{ReaderSessions: sessions}).Handler())
	t.Cleanup(server.Close)
	return server, editorBearer, readerBearer
}

func writerPost(t *testing.T, server *httptest.Server, bearer, path, body string) (int, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	envelope := map[string]any{}
	if decodeErr := json.Unmarshal(raw, &envelope); decodeErr != nil {
		t.Fatalf("writer response is not JSON: %v (%s)", decodeErr, raw)
	}
	return response.StatusCode, envelope
}

func writerData(t *testing.T, envelope map[string]any) map[string]any {
	t.Helper()
	data, _ := envelope["data"].(map[string]any)
	return data
}

func proposeBody(change string) string {
	encoded, _ := json.Marshal(map[string]any{"change": json.RawMessage(change)})
	return string(encoded)
}

func applyBody(change, digest, authorization string) string {
	encoded, _ := json.Marshal(map[string]any{"change": json.RawMessage(change), "preview_digest": digest, "authorization": authorization})
	return string(encoded)
}

// applyBodyFromPreview embeds the change EXACTLY as the preview resolved
// it (canonical note_ref, filled expected_revision): apply accepts only
// the unchanged preview, mirroring the MCP collaboration flow.
func applyBodyFromPreview(previewChange map[string]any, digest, authorization string) string {
	encoded, _ := json.Marshal(map[string]any{"change": previewChange, "preview_digest": digest, "authorization": authorization})
	return string(encoded)
}

func TestWriterEditPropose(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, editorBearer, readerBearer := newEditorTestServer(t, root)

	// Propose an append against the existing alpha note: readonly, exact
	// before/after, sha256 preview digest.
	change := `{"action": "append", "operation_id": "wedit-1", "note_ref": "notes/alpha.md", "body": "\n追加段落。"}`
	status, envelope := writerPost(t, server, editorBearer, "/v1/writer/proposals", proposeBody(change))
	if status != http.StatusOK {
		t.Fatalf("propose status = %d, envelope = %v", status, envelope)
	}
	data := writerData(t, envelope)
	digest, _ := data["preview_digest"].(string)
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("preview_digest = %q", digest)
	}
	if after, _ := data["after"].(string); !strings.Contains(after, "追加段落。") {
		t.Fatalf("preview after = %q", after)
	}
	if before, _ := data["before"].(string); !strings.Contains(before, "Alpha") {
		t.Fatalf("preview before = %q", before)
	}
	// The served preview must conform to the frozen closed schema.
	assertWriterDataConformance(t, envelope, catalogschema.WriterProposalPreviewV1Alpha1)

	// Propose never writes: the note body on disk stays unchanged.
	if status, _ := writerPost(t, server, editorBearer, "/v1/writer/proposals", proposeBody(change)); status != http.StatusOK {
		t.Fatalf("second propose status = %d", status)
	}

	// Invalid change: create without title fails closed with 400.
	status, _ = writerPost(t, server, editorBearer, "/v1/writer/proposals", proposeBody(`{"action": "create"}`))
	if status != http.StatusBadRequest {
		t.Fatalf("invalid change status = %d, want 400", status)
	}

	// Reader-only session: insufficient_scope.
	status, _ = writerPost(t, server, readerBearer, "/v1/writer/proposals", proposeBody(change))
	if status != http.StatusForbidden {
		t.Fatalf("reader-only propose status = %d, want 403", status)
	}

	// Closed payload: unknown field violates the frozen contract.
	status, _ = writerPost(t, server, editorBearer, "/v1/writer/proposals", `{"change": {"action": "create", "title": "t"}, "yes": true}`)
	if status != http.StatusBadRequest {
		t.Fatalf("closed propose payload status = %d, want 400", status)
	}
}

func TestWriterEditApply(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, editorBearer, _ := newEditorTestServer(t, root)

	change := `{"action": "append", "operation_id": "wedit-apply-1", "note_ref": "notes/alpha.md", "body": "\n应用后的段落。"}`
	_, envelope := writerPost(t, server, editorBearer, "/v1/writer/proposals", proposeBody(change))
	data := writerData(t, envelope)
	digest, _ := data["preview_digest"].(string)
	if digest == "" {
		t.Fatal("propose returned no digest")
	}
	resolvedChange, _ := data["change"].(map[string]any)
	if resolvedChange == nil {
		t.Fatal("propose returned no resolved change")
	}
	if resolved, _ := resolvedChange["expected_revision"].(string); !strings.HasPrefix(resolved, "sha256:") {
		t.Fatalf("resolved change must pin the current revision, got %q", resolved)
	}

	// Tampered digest: rejected, no partial application.
	status, tamperedEnvelope := writerPost(t, server, editorBearer, "/v1/writer/proposals:apply",
		applyBodyFromPreview(resolvedChange, "sha256:"+strings.Repeat("00", 32), "reviewed_proposal"))
	if status != http.StatusConflict {
		t.Fatalf("tampered digest status = %d, envelope = %v", status, tamperedEnvelope)
	}
	assertWriterErrorInTable(t, tamperedEnvelope, "writer.apply")

	// Authorization values are caller assertions, never host approval: the
	// schema rejects any value claiming verified approval.
	status, _ = writerPost(t, server, editorBearer, "/v1/writer/proposals:apply", applyBodyFromPreview(resolvedChange, digest, "host_approved"))
	if status != http.StatusBadRequest {
		t.Fatalf("host_approved authorization status = %d, want 400", status)
	}

	// Happy path: unchanged resolved preview + reviewed_proposal assertion.
	status, applyEnvelope := writerPost(t, server, editorBearer, "/v1/writer/proposals:apply", applyBodyFromPreview(resolvedChange, digest, "reviewed_proposal"))
	if status != http.StatusOK {
		t.Fatalf("apply status = %d, envelope = %v", status, applyEnvelope)
	}
	envelope = applyEnvelope
	if got := writerFact(t, envelope, "operation_id"); got != "wedit-apply-1" {
		t.Fatalf("apply operation_id = %q", got)
	}
	if got := writerFact(t, envelope, "operation_status"); got != "succeeded" {
		t.Fatalf("apply status fact = %q", got)
	}
	if got := writerFact(t, envelope, "receipt_ref"); !strings.HasPrefix(got, "snapshot:") {
		t.Fatalf("non-create apply must snapshot first, receipt_ref = %q", got)
	}
	if revision := writerFact(t, envelope, "revision_after"); !strings.HasPrefix(revision, "sha256:") {
		t.Fatalf("revision_after = %q", revision)
	}
	// The served apply receipt must conform to the frozen closed schema.
	assertWriterDataConformance(t, envelope, catalogschema.WriterApplyReceiptV1Alpha1)

	// The note on disk carries the appended body.
	if !vaultNoteContains(t, root, "notes/alpha.md", "应用后的段落。") {
		t.Fatal("applied body missing from the vault note")
	}

	// Replay with the same identity returns the same receipt, no second write.
	status, replay := writerPost(t, server, editorBearer, "/v1/writer/proposals:apply", applyBodyFromPreview(resolvedChange, digest, "reviewed_proposal"))
	if status != http.StatusOK {
		t.Fatalf("apply replay status = %d", status)
	}
	if got := writerFact(t, replay, "operation_id"); got != "wedit-apply-1" {
		t.Fatalf("replay operation_id = %q", got)
	}
	if got := writerFact(t, replay, "operation_status"); got != "succeeded" {
		t.Fatalf("replay status fact = %q", got)
	}

	// Missing operation identity: apply without change.operation_id fails
	// closed with operation_identity_required.
	noIdentity := `{"action": "append", "note_ref": "notes/alpha.md", "body": "\n无身份。"}`
	_, proposeEnvelope := writerPost(t, server, editorBearer, "/v1/writer/proposals", proposeBody(noIdentity))
	noIDDigest, _ := writerData(t, proposeEnvelope)["preview_digest"].(string)
	status, _ = writerPost(t, server, editorBearer, "/v1/writer/proposals:apply", applyBody(noIdentity, noIDDigest, "explicit_instruction"))
	if status != http.StatusBadRequest {
		t.Fatalf("identity-less apply status = %d, want 400", status)
	}
}

func TestWriterEditRevisionConflict(t *testing.T) {
	t.Parallel()
	root := readerHTTPVault(t)
	server, editorBearer, _ := newEditorTestServer(t, root)

	// Propose with the CURRENT revision pinned: preview resolves note_ref
	// to the canonical path and fills expected_revision.
	change := `{"action": "append", "operation_id": "wedit-conflict-1", "note_ref": "notes/alpha.md", "body": "\n过期提案。"}`
	_, envelope := writerPost(t, server, editorBearer, "/v1/writer/proposals", proposeBody(change))
	data := writerData(t, envelope)
	digest, _ := data["preview_digest"].(string)
	resolvedChange, _ := data["change"].(map[string]any)
	if resolvedChange == nil || digest == "" {
		t.Fatal("propose returned no resolved change or digest")
	}

	// The note changes after the proposal was prepared: expected_revision
	// drift must fail explicitly with 409 revision_conflict, never a blind
	// overwrite.
	if err := appendVaultNote(t, root, "notes/alpha.md", "\n外部编辑。"); err != nil {
		t.Fatal(err)
	}
	status, conflict := writerPost(t, server, editorBearer, "/v1/writer/proposals:apply", applyBodyFromPreview(resolvedChange, digest, "explicit_instruction"))
	if status != http.StatusConflict {
		t.Fatalf("revision drift status = %d, envelope = %v", status, conflict)
	}
	assertWriterErrorInTable(t, conflict, "writer.apply")
	if !vaultNoteContains(t, root, "notes/alpha.md", "外部编辑。") || vaultNoteContains(t, root, "notes/alpha.md", "过期提案。") {
		t.Fatal("conflicting apply must not overwrite the externally edited note")
	}
}

func vaultNoteContains(t *testing.T, root, rel, marker string) bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read note %s: %v", rel, err)
	}
	return strings.Contains(string(raw), marker)
}

func appendVaultNote(t *testing.T, root, rel, addition string) error {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = file.WriteString(addition)
	return err
}
