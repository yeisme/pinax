package sdkruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/inputrequests"
	"github.com/yeisme/pinax/internal/mcpserver"
)

// startRuntime 在进程内启动 candidate 运行时并返回已初始化的官方 SDK
// 客户端会话，覆盖 initialize → 发现 → 调用的完整协议链路。
func startRuntime(t *testing.T, options mcpserver.ServerOptions) (*mcp.ClientSession, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	server := New(svc, root, options)
	t1, t2 := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(ctx, t1) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "sdkruntime-test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, root
}

func writeFixture(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func toolNames(tools []*mcp.Tool) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, tool := range tools {
		names[tool.Name] = true
	}
	return names
}

func resourceURIs(resources []*mcp.Resource) map[string]bool {
	uris := make(map[string]bool, len(resources))
	for _, resource := range resources {
		uris[resource.URI] = true
	}
	return uris
}

func TestSDKRuntimeDiscoveryMatchesRegistry(t *testing.T) {
	session, _ := startRuntime(t, mcpserver.ServerOptions{})
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools.Tools) != 20 {
		t.Fatalf("tools = %d, want 20 (registry projection)", len(tools.Tools))
	}
	names := toolNames(tools.Tools)
	for _, want := range []string{"pinax.search", "pinax.note.read", "pinax.query.run", "pinax.agent.handoff_read"} {
		if !names[want] {
			t.Fatalf("missing tool %s in %#v", want, names)
		}
	}
	for _, want := range []string{"pinax.interaction.search", "pinax.input.prepare"} {
		if names[want] {
			t.Fatalf("unexpected tool %s without collaboration/input opt-in", want)
		}
	}
	var search *mcp.Tool
	for _, tool := range tools.Tools {
		if tool.Name == "pinax.search" {
			search = tool
		}
	}
	if search == nil || search.Annotations == nil {
		t.Fatalf("pinax.search annotations missing")
	}
	if !search.Annotations.ReadOnlyHint || search.Annotations.DestructiveHint == nil || *search.Annotations.DestructiveHint {
		t.Fatalf("pinax.search annotations = %#v, want readonly/non-destructive", search.Annotations)
	}
	if search.OutputSchema == nil {
		t.Fatalf("pinax.search output schema missing")
	}

	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	uris := resourceURIs(resources.Resources)
	for _, want := range []string{"pinax://manifest", "pinax://readiness", "pinax://vault/current", "pinax://vault/graph", "pinax://organize/plan"} {
		if !uris[want] {
			t.Fatalf("missing concrete resource %s in %#v", want, uris)
		}
	}
	for _, want := range []string{"pinax://note/{note_id}", "pinax://input/capabilities", "pinax://interaction/capabilities"} {
		if uris[want] {
			t.Fatalf("unexpected resource %s leaked into concrete list", want)
		}
	}

	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("list resource templates: %v", err)
	}
	templateURIs := make(map[string]bool, len(templates.ResourceTemplates))
	for _, template := range templates.ResourceTemplates {
		templateURIs[template.URITemplate] = true
	}
	for _, want := range []string{"pinax://note/{note_id}", "pinax://search/{query}", "pinax://project/{slug}/board", "pinax://sync/job/{run_id}"} {
		if !templateURIs[want] {
			t.Fatalf("missing resource template %s in %#v", want, templateURIs)
		}
	}
}

func TestSDKRuntimeCallToolSearchAndReadResource(t *testing.T) {
	session, root := startRuntime(t, mcpserver.ServerOptions{})
	writeFixture(t, root, "notes/pinax.md", "# Pinax MCP\n\n只读查询 fixture。\n")

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pinax.search", Arguments: map[string]any{"query": "只读"}})
	if err != nil {
		t.Fatalf("call pinax.search: %v", err)
	}
	if result.IsError {
		t.Fatalf("pinax.search returned tool error")
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["status"] != "success" {
		t.Fatalf("structured result = %#v", result.StructuredContent)
	}
	if len(result.Content) == 0 {
		t.Fatalf("text content missing")
	}

	read, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: "pinax://vault/current"})
	if err != nil {
		t.Fatalf("read resource: %v", err)
	}
	if len(read.Contents) == 0 || read.Contents[0].URI != "pinax://vault/current" || read.Contents[0].Text == "" {
		t.Fatalf("resource contents = %#v", read.Contents)
	}
}

// TestSDKRuntimeUnknownToolUsesSDKRejection 官方 SDK 对未注册工具在
// server 侧以 -32602 invalid params 拒绝（legacy runtime 为 -32601）。
// 该差异已记录在 change design 的差异表，默认切换评审（4.4）需复核。
func TestSDKRuntimeUnknownToolUsesSDKRejection(t *testing.T) {
	session, _ := startRuntime(t, mcpserver.ServerOptions{})
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pinax.organize.apply"})
	if err == nil {
		t.Fatalf("unknown tool unexpectedly succeeded")
	}
	var wire *jsonrpc.Error
	if !errors.As(err, &wire) {
		t.Fatalf("error type = %T, want jsonrpc.Error", err)
	}
	if wire.Code != -32602 {
		t.Fatalf("error code = %d, want -32602 (SDK unknown-tool rejection)", wire.Code)
	}
}

func TestSDKRuntimeInvalidArgumentsKeepErrorCode(t *testing.T) {
	session, _ := startRuntime(t, mcpserver.ServerOptions{})
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pinax.search", Arguments: map[string]any{"query": 42}})
	if err == nil {
		t.Fatalf("invalid arguments unexpectedly succeeded")
	}
	var wire *jsonrpc.Error
	if !errors.As(err, &wire) || wire.Code != -32602 {
		t.Fatalf("error = %v, want code -32602", err)
	}
}

func TestSDKRuntimeCollaborationPreviewApplyAndStatus(t *testing.T) {
	session, root := startRuntime(t, mcpserver.ServerOptions{
		Collaboration: true,
		NotePolicy:    app.CollaborationPolicy{AllowBody: true, AllowWrite: true},
	})
	writeFixture(t, root, "notes/pinax.md", "---\nschema_version: pinax.note.v1\nnote_id: note_pinax\ntitle: Pinax\n---\n\n原始正文。\n")

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := toolNames(tools.Tools)
	for _, want := range []string{"pinax.interaction.search", "pinax.interaction.read", "pinax.interaction.preview", "pinax.interaction.apply", "pinax.interaction.status", "pinax.interaction.version"} {
		if !names[want] {
			t.Fatalf("missing collaboration tool %s", want)
		}
	}

	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if !resourceURIs(resources.Resources)["pinax://interaction/capabilities"] {
		t.Fatalf("collaboration capabilities resource missing")
	}

	bodyRead, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pinax.interaction.read", Arguments: map[string]any{
		"note_ref": "notes/pinax.md", "display": "body", "intent": "edit",
	}})
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	bodyData, _ := bodyRead.StructuredContent.(map[string]any)
	revision := searchNestedString(bodyData, "revision")
	if revision == "" {
		t.Fatalf("body read missing revision: %#v", bodyData)
	}

	preview, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pinax.interaction.preview", Arguments: map[string]any{
		"change": map[string]any{"action": "append", "note_ref": "notes/pinax.md", "expected_revision": revision, "body": "\n追加段落。\n"},
	}})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	previewData, _ := preview.StructuredContent.(map[string]any)
	previewPayload, _ := previewData["data"].(map[string]any)
	previewChange, _ := previewPayload["change"].(map[string]any)
	digest, _ := previewPayload["preview_digest"].(string)
	if previewChange == nil || digest == "" {
		t.Fatalf("preview result missing change/digest: %#v", previewData)
	}

	// apply 必须原样使用 preview 返回的规范化 change 与 digest。
	apply, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pinax.interaction.apply", Arguments: map[string]any{
		"change":         previewChange,
		"preview_digest": digest,
		"authorization":  "explicit_instruction",
	}})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	applyData, _ := apply.StructuredContent.(map[string]any)
	if applyData["status"] != "success" && applyData["status"] != "succeeded" {
		t.Fatalf("apply status = %#v", applyData)
	}
	operationID := searchNestedString(applyData, "operation_id")
	if operationID == "" {
		t.Fatalf("apply result missing operation id: %#v", applyData)
	}

	// 持久写入恢复契约：超时/重连后用同一 operation_id 查询，不重放。
	status, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pinax.interaction.status", Arguments: map[string]any{"operation_id": operationID}})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	statusData, _ := status.StructuredContent.(map[string]any)
	if statusData["status"] != "success" {
		t.Fatalf("status result = %#v", statusData)
	}

	body, err := os.ReadFile(filepath.Join(root, "notes/pinax.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "追加段落") {
		t.Fatalf("applied body missing appended paragraph: %q", string(body))
	}
}

func TestSDKRuntimeConcurrentToolCalls(t *testing.T) {
	session, root := startRuntime(t, mcpserver.ServerOptions{})
	writeFixture(t, root, "notes/pinax.md", "# Pinax MCP\n\n并发 fixture。\n")
	const calls = 8
	var wg sync.WaitGroup
	failures := make(chan error, calls)
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "pinax.search", Arguments: map[string]any{"query": "并发"}})
			if err != nil {
				failures <- err
				return
			}
			if structured, ok := result.StructuredContent.(map[string]any); !ok || structured["status"] != "success" {
				failures <- errors.New("concurrent search returned non-success result")
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatalf("concurrent call: %v", err)
	}
}

func TestSDKRuntimeCancelledRequestReturnsError(t *testing.T) {
	session, _ := startRuntime(t, mcpserver.ServerOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pinax.search", Arguments: map[string]any{"query": "x"}}); err == nil {
		t.Fatalf("cancelled call unexpectedly succeeded")
	}
}

// TestSDKRuntimeStdioServeFraming 走完整 stdio 帧路径：Serve 与官方 SDK
// 客户端经 IOTransport 对接，验证 candidate 的进程级 stdio 边界。
func TestSDKRuntimeStdioServeFraming(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeFixture(t, root, "notes/pinax.md", "# Pinax MCP\n\nstdio fixture。\n")

	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()
	go func() {
		if err := Serve(ctx, svc, root, serverReader, serverWriter, mcpserver.ServerOptions{}); err != nil && ctx.Err() == nil {
			t.Errorf("serve: %v", err)
		}
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientReader, Writer: clientWriter}, nil)
	if err != nil {
		t.Fatalf("connect over stdio: %v", err)
	}
	defer func() { _ = session.Close() }()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "pinax.search", Arguments: map[string]any{"query": "stdio"}})
	if err != nil {
		t.Fatalf("call over stdio: %v", err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["status"] != "success" {
		t.Fatalf("stdio structured result = %#v", result.StructuredContent)
	}
	_ = clientWriter.Close()
	_ = clientReader.Close()
}

func searchNestedString(value any, key string) string {
	switch typed := value.(type) {
	case map[string]any:
		if found, ok := typed[key].(string); ok {
			return found
		}
		for _, nested := range typed {
			if found := searchNestedString(nested, key); found != "" {
				return found
			}
		}
	case []any:
		for _, nested := range typed {
			if found := searchNestedString(nested, key); found != "" {
				return found
			}
		}
	}
	return ""
}

func TestSDKRuntimeInputIntakeToolsWired(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Input vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	input, closeInput, err := inputrequests.Open(root, "http://127.0.0.1:9", svc)
	if err != nil {
		t.Fatalf("open input: %v", err)
	}
	defer closeInput()

	server := New(svc, root, mcpserver.ServerOptions{Input: input})
	t1, t2 := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(ctx, t1) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "sdkruntime-input-test", Version: "v0.0.1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := toolNames(tools.Tools)
	for _, want := range []string{"pinax.input.prepare", "pinax.input.status", "pinax.input.renew", "pinax.input.abort", "pinax.input.preview", "pinax.input.apply"} {
		if !names[want] {
			t.Fatalf("missing input tool %s", want)
		}
	}

	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if !resourceURIs(resources.Resources)["pinax://input/capabilities"] {
		t.Fatalf("input capabilities resource missing")
	}
	caps, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "pinax://input/capabilities"})
	if err != nil {
		t.Fatalf("read input capabilities: %v", err)
	}
	if len(caps.Contents) == 0 || !strings.Contains(caps.Contents[0].Text, "pinax.input.prepare") {
		t.Fatalf("input capabilities content = %#v", caps.Contents)
	}
}
