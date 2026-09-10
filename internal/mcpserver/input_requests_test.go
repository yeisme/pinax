package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/inputrequests"
	intake "github.com/yeisme/runtime-plane/pkg/inputintake"
)

func TestInputIntakeMCPToHTTPPreviewAndConfirmedImport(t *testing.T) {
	ctx := context.Background()
	vault := t.TempDir()
	application := app.NewService()
	if _, e := application.InitVault(ctx, app.InitVaultRequest{VaultPath: vault, Title: "Input fixture"}); e != nil {
		t.Fatal(e)
	}
	httpServer := httptest.NewUnstartedServer(nil)
	base := "http://" + httpServer.Listener.Addr().String()
	service, close, e := inputrequests.Open(vault, base, application)
	if e != nil {
		t.Fatal(e)
	}
	defer close()
	httpServer.Config.Handler = service
	httpServer.Start()
	defer httpServer.Close()
	readonly := NewServer(application, vault)
	req := Request{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: map[string]any{"name": "pinax.input.prepare", "arguments": map[string]any{"purpose": "markdown", "idempotency_key": "task"}}}
	if _, e = readonly.Handle(ctx, req); e == nil {
		t.Fatal("old connection gained write capability")
	}
	// Exercise real stdio JSON-RPC framing for the new action, without a product CLI.
	frames := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{}}` + "\n"
	encoded, _ := json.Marshal(req)
	frames += string(encoded) + "\n"
	var output bytes.Buffer
	if e = ServeWithOptions(ctx, application, vault, strings.NewReader(frames), &output, ServerOptions{Input: service}); e != nil {
		t.Fatal(e)
	}
	decoder := json.NewDecoder(&output)
	var response Response
	if e = decoder.Decode(&response); e != nil {
		t.Fatal(e)
	}
	if e = decoder.Decode(&response); e != nil {
		t.Fatal(e)
	}
	if response.Error != nil {
		t.Fatal(response.Error.Code, response.Error.Message)
	}
	data, _ := response.Result["structuredContent"].(map[string]any)
	id, _ := data["input_request_id"].(string)
	content, _ := response.Result["content"].([]any)
	link := ""
	for _, c := range content {
		m, _ := c.(map[string]any)
		u, _ := m["uri"].(string)
		if strings.Contains(u, "#grant=") {
			link = u
		}
	}
	if id == "" || link == "" {
		t.Fatal("missing safe request projection or transient transfer descriptor")
	}
	client, e := intake.NewTransferClient(base, link, httpServer.Client())
	if e != nil {
		t.Fatal(e)
	}
	body := []byte("# Imported reference\n\nCreative material.\n")
	if _, e = client.Bind(ctx, intake.File{Name: "reference.md", MIME: "text/markdown", Size: int64(len(body))}); e != nil {
		t.Fatal(e)
	}
	if _, e = client.Put(ctx, int64(len(body)), bytes.NewReader(body)); e != nil {
		t.Fatal(e)
	}
	ready, e := client.Complete(ctx)
	if e != nil || ready.Receipt.DomainState != "requires_preview_confirmation" {
		t.Fatal("bad receipt", e)
	}
	target := filepath.Join(vault, "notes", "reference.md")
	if _, e = os.Stat(target); !os.IsNotExist(e) {
		t.Fatal("upload implicitly imported note")
	}
	who := intake.Identity{Actor: "stdio-owner", Project: "vault"}
	preview, e := service.Preview(ctx, who, id, "skip", "", false, "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.Preview(ctx, who, id, "skip", "", true, "wrong"); e == nil {
		t.Fatal("unconfirmed preview accepted")
	}
	done, e := service.Preview(ctx, who, id, "skip", "", true, preview.Digest)
	if e != nil || done.State != "adopted" {
		t.Fatal(e)
	}
	repeated, e := service.Preview(ctx, who, id, "skip", "", true, preview.Digest)
	if e != nil || repeated.State != "adopted" {
		t.Fatal("adoption replay changed result", e)
	}
	file, e := os.ReadFile(target)
	if e != nil || !bytes.Contains(file, []byte("Creative material.")) {
		t.Fatal("existing Markdown writer was not used")
	}
	if _, e = service.Preview(ctx, who, id, "overwrite", "", true, preview.Digest); e == nil {
		t.Fatal("reused input with different adoption intent")
	}
}
