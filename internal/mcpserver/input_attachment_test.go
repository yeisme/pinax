package mcpserver

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/inputrequests"
	"github.com/yeisme/pinax/pkg/inputcontract"
	intake "github.com/yeisme/runtime-plane/pkg/inputintake"
)

func TestInputAttachmentMCPUploadPreviewApplyRecovery(t *testing.T) {
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(t.Context(), app.InitVaultRequest{VaultPath: root, Title: "Attachment input"}); err != nil {
		t.Fatal(err)
	}
	created, err := svc.CreateNote(t.Context(), app.CreateNoteRequest{VaultPath: root, Title: "Target", Body: "before"})
	if err != nil {
		t.Fatal(err)
	}
	note, revision, err := svc.CollaborationRead(t.Context(), root, created.Facts["path"], "edit", app.CollaborationPolicy{AllowBody: true, AllowWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewUnstartedServer(nil)
	base := "http://" + httpServer.Listener.Addr().String()
	service, close, err := inputrequests.Open(root, base, svc)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	httpServer.Config.Handler = service
	httpServer.Start()
	defer httpServer.Close()
	server := NewServerWithOptions(svc, root, ServerOptions{Input: service})
	call := func(name string, value any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(value)
		var args map[string]any
		if err := json.Unmarshal(raw, &args); err != nil {
			t.Fatal(err)
		}
		response, err := server.Handle(t.Context(), Request{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: map[string]any{"name": name, "arguments": args}})
		if err != nil || response.Error != nil {
			t.Fatalf("%s: %v %#v", name, err, response.Error)
		}
		result, _ := response.Result["structuredContent"].(map[string]any)
		return result
	}
	who := intake.Identity{Actor: "stdio-owner", Project: "vault"}
	access, err := service.Prepare(t.Context(), who, intake.Prepare{Purpose: "attachment", IdempotencyKey: "attachment-input"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := intake.NewTransferClient(base, access.TransferURL, httpServer.Client())
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("Uploaded attachment fixture")
	if _, err := client.Bind(t.Context(), intake.File{Name: "fixture.txt", MIME: "text/plain", Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Put(t.Context(), int64(len(body)), bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(t.Context()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(root, note.Path))
	req := inputcontract.AttachmentRequest{InputRequestID: access.Request.ID, OperationID: "mcp-attachment", ObjectID: note.ID, Revision: revision, Placement: "per-note"}
	if _, err := service.AttachmentPreview(t.Context(), intake.Identity{Actor: "other", Project: "vault"}, req); err == nil {
		t.Fatal("another actor accessed staged bytes")
	}
	previewMap := call("pinax.input.attachment.preview", req)
	raw, _ := json.Marshal(previewMap)
	var proposal inputcontract.AttachmentProposal
	if err := json.Unmarshal(raw, &proposal); err != nil {
		t.Fatal(err)
	}
	afterPreview, _ := os.ReadFile(filepath.Join(root, note.Path))
	if string(before) != string(afterPreview) {
		t.Fatal("preview mutated note")
	}
	result := call("pinax.input.attachment.apply", inputcontract.AttachmentApply{Proposal: proposal, Confirm: true})
	if result["status"] != "succeeded" {
		t.Fatalf("apply status=%v", result["status"])
	}
	after, _ := os.ReadFile(filepath.Join(root, note.Path))
	if !strings.Contains(string(after), proposal.Reference) {
		t.Fatal("reference missing")
	}
	// Recovery no longer needs the upload bytes or a live transfer ticket.
	if err := os.Remove(filepath.Join(root, ".pinax", "input-files", access.Request.ID)); err != nil {
		t.Fatal(err)
	}
	status := call("pinax.input.attachment.status", map[string]any{"operation_id": req.OperationID})
	if status["status"] != "succeeded" {
		t.Fatal("original receipt unavailable without upload bytes")
	}
	if _, err := service.AttachmentOperation(t.Context(), intake.Identity{Actor: "other", Project: "vault"}, req.OperationID, true); err == nil {
		t.Fatal("another actor inspected operation")
	}
	bad := map[string]any{"input_request_id": req.InputRequestID, "operation_id": "path-attack", "object_id": req.ObjectID, "revision": revision, "placement": "per-note", "source_path": "/tmp/private"}
	if _, err := server.Handle(t.Context(), Request{JSONRPC: "2.0", ID: 2, Method: "tools/call", Params: map[string]any{"name": "pinax.input.attachment.preview", "arguments": bad}}); err == nil {
		t.Fatal("arbitrary path accepted")
	}
}
