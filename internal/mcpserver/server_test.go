package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/app"
	"github.com/yeisme/pinax/internal/domain"
)

func TestReadonlyMCPListsAndCallsTools(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	server := NewServer(svc, root)

	tools, err := server.Handle(ctx, Request{ID: 1, Method: "tools/list"})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if !containsTool(tools.Tools, "pinax.search") || containsTool(tools.Tools, "pinax.organize.apply") {
		t.Fatalf("tools = %#v", tools.Tools)
	}

	resources, err := server.Handle(ctx, Request{ID: 2, Method: "resources/list"})
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	if !containsResource(resources.Resources, "pinax://vault/current") {
		t.Fatalf("resources = %#v", resources.Resources)
	}

	writeMCPFixture(t, root, "notes/pinax.md", "# Pinax MCP\n\n只读查询。\n")
	search, err := server.Handle(ctx, Request{ID: 3, Method: "tools/call", Params: map[string]any{"name": "pinax.search", "arguments": map[string]any{"query": "只读"}}})
	if err != nil {
		t.Fatalf("tools/call search: %v", err)
	}
	if search.Result == nil || search.Result["status"] != "success" {
		t.Fatalf("search result = %#v", search.Result)
	}

	if _, err := server.Handle(ctx, Request{ID: 4, Method: "tools/call", Params: map[string]any{"name": "pinax.organize.apply"}}); err == nil {
		t.Fatalf("write tool unexpectedly succeeded")
	}
}

func TestReadonlyMCPProjectBoardTool(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateProject(ctx, app.ProjectRequest{VaultPath: root, Slug: "research", Name: "研究", NotesPrefix: "research"}); err != nil {
		t.Fatalf("create project: %v", err)
	}
	writeMCPFixture(t, root, "research/task.md", "---\nschema_version: pinax.note.v1\nnote_id: note_task\ntitle: Board Task\nproject: research\nkind: task\nstatus: active\n---\n\nsecret body should not leak as body field\n")
	server := NewServer(svc, root)

	resources, err := server.Handle(ctx, Request{ID: 20, Method: "resources/list"})
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	if !containsResource(resources.Resources, "pinax://project/{slug}/board") {
		t.Fatalf("resources = %#v", resources.Resources)
	}
	tools, err := server.Handle(ctx, Request{ID: 21, Method: "tools/list"})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if !containsTool(tools.Tools, "pinax.project.board") {
		t.Fatalf("tools = %#v", tools.Tools)
	}
	resp, err := server.Handle(ctx, Request{ID: 22, Method: "tools/call", Params: map[string]any{"name": "pinax.project.board", "arguments": map[string]any{"project": "research"}}})
	if err != nil {
		t.Fatalf("project board tool: %v", err)
	}
	if resp.Result["status"] == "failed" || !strings.Contains(fmt.Sprint(resp.Result), "research") || strings.Contains(fmt.Sprint(resp.Result), `body`) {
		t.Fatalf("project board result = %#v", resp.Result)
	}
}

func TestReadonlyMCPNoteReadUsesBoundedDisplay(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeMCPFixture(t, root, "notes/secret.md", "---\nschema_version: pinax.note.v1\nnote_id: note_secret\ntitle: Secret\nkind: task\nstatus: active\n---\n\nsecret body should not leak through MCP note read\n")
	server := NewServer(svc, root)

	resp, err := server.Handle(ctx, Request{ID: 23, Method: "tools/call", Params: map[string]any{"name": "pinax.note.read", "arguments": map[string]any{"note_id": "note_secret"}}})
	if err != nil {
		t.Fatalf("note read tool: %v", err)
	}
	data := resp.Result["data"].(map[string]any)
	note := data["note"].(domain.NoteDisplay)
	if resp.Result["status"] != "success" || note.Display != domain.NoteDisplayCard || note.Body != "" || note.Excerpt == "" {
		t.Fatalf("note read result leaked body or missed bounded display: %#v", resp.Result)
	}
}

func TestReadonlyMCPBrainToolsAreBoundedAndReadonly(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeMCPFixture(t, root, "notes/alice.md", "# Alice Meeting\n\nAlice needs roadmap and budget updates.\n\nSECRET_BODY_SENTINEL Authorization: Bearer raw_provider_payload should not leak.\n")
	server := NewServer(svc, root)

	tools, err := server.Handle(ctx, Request{ID: 30, Method: "tools/list"})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	for _, name := range []string{"pinax.brain.context", "pinax.brain.answer", "pinax.brain.sources", "pinax.brain.maintenance_plan"} {
		tool, ok := toolByName(tools.Tools, name)
		if !ok {
			t.Fatalf("missing %s in tools: %#v", name, tools.Tools)
		}
		if !tool.Readonly || tool.BodyExposure != "bounded_projection" || tool.Scope != "local_vault" {
			t.Fatalf("brain tool metadata for %s = %#v", name, tool)
		}
	}

	answer, err := server.Handle(ctx, Request{ID: 31, Method: "tools/call", Params: map[string]any{"name": "pinax.brain.answer", "arguments": map[string]any{"question": "Alice roadmap budget", "body_exposure": "full"}}})
	if err != nil {
		t.Fatalf("brain answer tool: %v", err)
	}
	text := fmt.Sprint(answer.Result)
	for _, forbidden := range []string{"SECRET_BODY_SENTINEL", "Authorization", "Bearer", "raw_provider_payload"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("brain answer MCP leaked %q: %s", forbidden, text)
		}
	}
	if answer.Result["command"] != "brain.answer" || !strings.Contains(text, "pinax.agent_brain.answer.v1") || !strings.Contains(text, "bounded_projection") {
		t.Fatalf("brain answer result = %#v", answer.Result)
	}

	sources, err := server.Handle(ctx, Request{ID: 32, Method: "tools/call", Params: map[string]any{"name": "pinax.brain.sources", "arguments": map[string]any{"question": "Alice roadmap budget"}}})
	if err != nil {
		t.Fatalf("brain sources tool: %v", err)
	}
	if sources.Result["status"] != "success" || !strings.Contains(fmt.Sprint(sources.Result), "sources") {
		t.Fatalf("brain sources result = %#v", sources.Result)
	}

	maintenance, err := server.Handle(ctx, Request{ID: 33, Method: "tools/call", Params: map[string]any{"name": "pinax.brain.maintenance_plan", "arguments": map[string]any{}}})
	if err != nil {
		t.Fatalf("brain maintenance plan tool: %v", err)
	}
	if maintenance.Result["status"] != "success" || !strings.Contains(fmt.Sprint(maintenance.Result), "pinax proof loop run") {
		t.Fatalf("brain maintenance plan result = %#v", maintenance.Result)
	}
}

func TestGraphContextBounds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	body := "# Hub\n\n"
	for i := 0; i < 40; i++ {
		title := "Target " + twoDigits(i)
		body += "[[" + title + "]]\n"
		writeMCPFixture(t, root, filepath.Join("notes", "target-"+twoDigits(i)+".md"), "# "+title+"\n\ntarget\n")
	}
	body += "\nsecret-token raw prompt hidden system prompt\n"
	writeMCPFixture(t, root, "notes/hub.md", body)

	server := NewServer(svc, root)
	resp, err := server.Handle(ctx, Request{ID: 10, Method: "tools/call", Params: map[string]any{"name": "pinax.note.context", "arguments": map[string]any{"note_ref": "Hub"}}})
	if err != nil {
		t.Fatalf("graph context: %v", err)
	}
	if resp.Result["status"] != "partial" {
		t.Fatalf("graph context status = %#v", resp.Result)
	}
	facts := resp.Result["facts"].(map[string]any)
	if facts["truncated"] != "true" || facts["links.total"] == facts["links.returned"] {
		t.Fatalf("graph context facts missing truncation: %#v", facts)
	}
	links := resp.Result["links"].(map[string]any)["links"].([]domain.NoteLink)
	if len(links) == 0 || len(links) > 20 {
		t.Fatalf("bounded links len = %d", len(links))
	}
	if out := fmt.Sprint(resp.Result); strings.Contains(out, "secret-token") || strings.Contains(out, "raw prompt") || strings.Contains(out, "hidden system prompt") || strings.Contains(out, "# Hub") {
		t.Fatalf("graph context leaked note body:\n%s", out)
	}
	if !strings.Contains(fmt.Sprint(resp.Result["next_action"]), "pinax note links") {
		t.Fatalf("graph context next action missing: %#v", resp.Result)
	}
}

func TestGraphContextTruncation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeMCPFixture(t, root, "notes/source.md", "# Source\n\n[[Shared]]\n")
	for i := 0; i < 8; i++ {
		writeMCPFixture(t, root, filepath.Join("notes", "shared-"+twoDigits(i)+".md"), "# Shared\n\nbody\n")
	}

	server := NewServer(svc, root)
	resp, err := server.Handle(ctx, Request{ID: 11, Method: "tools/call", Params: map[string]any{"name": "pinax.note.context", "arguments": map[string]any{"note_ref": "Source"}}})
	if err != nil {
		t.Fatalf("graph context: %v", err)
	}
	facts := resp.Result["facts"].(map[string]any)
	if facts["truncated"] != "true" || facts["candidates.truncated"] != "true" {
		t.Fatalf("graph context candidate truncation facts = %#v", facts)
	}
	links := resp.Result["links"].(map[string]any)["links"].([]domain.NoteLink)
	if len(links) != 1 || len(links[0].Candidates) == 0 || len(links[0].Candidates) > 3 {
		t.Fatalf("candidate bound not applied: %#v", links)
	}
}

func twoDigits(i int) string {
	if i < 10 {
		return "0" + fmt.Sprint(i)
	}
	return fmt.Sprint(i)
}

func writeMCPFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	if strings.EqualFold(filepath.Ext(rel), ".md") && !strings.HasPrefix(content, "---\n") {
		title := mcpFixtureTitle(rel, content)
		id := "note_" + strings.NewReplacer("/", "_", "\\", "_", ".", "_", " ", "_").Replace(strings.ToLower(rel))
		content = "---\nschema_version: pinax.note.v1\nnote_id: " + id + "\ntitle: " + title + "\ntags: []\n---\n\n" + content
	}
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func mcpFixtureTitle(rel, content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
}

func containsTool(tools []Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func toolByName(tools []Tool, name string) (Tool, bool) {
	for _, tool := range tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

func containsResource(resources []Resource, uri string) bool {
	for _, resource := range resources {
		if resource.URI == uri {
			return true
		}
	}
	return false
}

func TestReadonlyMCPQueryAndDatabaseView(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	if _, err := svc.CreateNote(ctx, app.CreateNoteRequest{VaultPath: root, Title: "Active", Body: "priority:: 2\n", Status: "active"}); err != nil {
		t.Fatalf("create note: %v", err)
	}
	if _, err := svc.RebuildIndex(ctx, app.VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	if _, err := svc.SaveView(ctx, app.ViewRequest{VaultPath: root, Name: "active", Status: "active"}); err != nil {
		t.Fatalf("save view: %v", err)
	}
	if _, err := svc.SaveDatabaseView(ctx, app.ViewRequest{VaultPath: root, Name: "active-tab", Display: "list", Query: `SELECT title, status FROM notes WHERE status = "active" LIMIT 10`}); err != nil {
		t.Fatalf("save database view: %v", err)
	}
	server := NewServer(svc, root)
	tools, err := server.Handle(ctx, Request{ID: 20, Method: "tools/list"})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if !containsTool(tools.Tools, "pinax.query.run") || !containsTool(tools.Tools, "pinax.database.view.show") || !containsTool(tools.Tools, "pinax.database.view.render") {
		t.Fatalf("tools missing query/view: %#v", tools.Tools)
	}
	query, err := server.Handle(ctx, Request{ID: 21, Method: "tools/call", Params: map[string]any{"name": "pinax.query.run", "arguments": map[string]any{"sql": "SELECT title FROM notes WHERE status = \"active\" LIMIT 5"}}})
	if err != nil {
		t.Fatalf("query run: %v", err)
	}
	if query.Result["status"] != "success" || !strings.Contains(fmt.Sprint(query.Result), "Active") {
		t.Fatalf("query result = %#v", query.Result)
	}
	view, err := server.Handle(ctx, Request{ID: 22, Method: "tools/call", Params: map[string]any{"name": "pinax.database.view.show", "arguments": map[string]any{"name": "active"}}})
	if err != nil {
		t.Fatalf("view show: %v", err)
	}
	if view.Result["status"] != "success" || !strings.Contains(fmt.Sprint(view.Result), "Active") {
		t.Fatalf("view result = %#v", view.Result)
	}
	rendered, err := server.Handle(ctx, Request{ID: 23, Method: "tools/call", Params: map[string]any{"name": "pinax.database.view.render", "arguments": map[string]any{"name": "active-tab"}}})
	if err != nil {
		t.Fatalf("database view render: %v", err)
	}
	if rendered.Result["status"] != "success" || !strings.Contains(fmt.Sprint(rendered.Result), "database_tab") || !strings.Contains(fmt.Sprint(rendered.Result), "database.display:list") {
		t.Fatalf("database view render result = %#v", rendered.Result)
	}
}

// TestMCPReleaseCoreFrame drives the stdio JSON-RPC frame protocol end to end
// (initialize -> tools/list -> bounded read -> write rejection) so the release
// MCP experience has automated evidence that does not depend on a real MCP
// client, network, token, or user vault.
func TestMCPReleaseCoreFrame(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Release Frame"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	writeMCPFixture(t, root, "notes/release.md", "# Release\n\nbounded read frame.\n")
	if _, err := svc.RebuildIndex(ctx, app.VaultRequest{VaultPath: root}); err != nil {
		t.Fatalf("rebuild index: %v", err)
	}

	// Build a newline-delimited JSON-RPC frame input and drive Serve().
	frames := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"pinax.search","arguments":{"query":"bounded"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"pinax.repair.apply"}}`,
	}
	var in strings.Builder
	for _, f := range frames {
		in.WriteString(f + "\n")
	}
	var out strings.Builder
	if err := Serve(ctx, svc, root, strings.NewReader(in.String()), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}

	responses := parseMCPFrameResponses(t, out.String())
	if len(responses) != len(frames) {
		t.Fatalf("expected %d frame responses, got %d: %#v", len(frames), len(responses), out.String())
	}

	// Frame 1: initialize declares read_only release posture.
	if responses[0].Result["read_only"] != true {
		t.Fatalf("initialize must declare read_only: %#v", responses[0])
	}

	// Frame 2: tools/list only advertises readonly + plan-preview tools.
	for _, tool := range responses[1].Tools {
		if strings.Contains(tool.Name, "apply") || strings.Contains(tool.Name, ".write") || strings.Contains(tool.Name, "delete") {
			t.Fatalf("release MCP must not advertise write tool %s", tool.Name)
		}
	}
	if !containsTool(responses[1].Tools, "pinax.search") {
		t.Fatalf("tools/list missing bounded read tool: %#v", responses[1].Tools)
	}

	// Frame 3: bounded read tool succeeds.
	if responses[2].Result["status"] != "success" {
		t.Fatalf("bounded read tool should succeed: %#v", responses[2].Result)
	}

	// Frame 4: write attempt is rejected (approval_required), vault untouched.
	if responses[3].Error == nil || responses[3].Error.Code != "approval_required" {
		t.Fatalf("write tool must be rejected with approval_required: %#v", responses[3])
	}
}

// TestMCPReleaseCoreExposesNoDirectWriteTools is the canonical release gate
// that tools/list and resources/list never advertise a direct vault mutation
// surface (Markdown, .pinax/**, Git, provider, Cloud Sync, or remote write).
func TestMCPReleaseCoreExposesNoDirectWriteTools(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	svc := app.NewService()
	if _, err := svc.InitVault(ctx, app.InitVaultRequest{VaultPath: root, Title: "Vault"}); err != nil {
		t.Fatalf("init vault: %v", err)
	}
	server := NewServer(svc, root)

	toolsResp, err := server.Handle(ctx, Request{ID: 1, Method: "tools/list"})
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	forbiddenFragments := []string{"apply", ".write", "delete", "capture", "promote", "discard", "sync.push", "sync.pull", "snapshot.create"}
	for _, tool := range toolsResp.Tools {
		for _, fragment := range forbiddenFragments {
			if strings.Contains(tool.Name, fragment) {
				t.Fatalf("release MCP tool %s matches forbidden write fragment %q", tool.Name, fragment)
			}
		}
	}
	// pinax.git.snapshot_plan is allowed because it only returns a next command.
	if !containsTool(toolsResp.Tools, "pinax.git.snapshot_plan") {
		t.Fatalf("plan-preview tool should be advertised: %#v", toolsResp.Tools)
	}
}

func parseMCPFrameResponses(t *testing.T, out string) []Response {
	t.Helper()
	var responses []Response
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("failed to parse frame response %q: %v", line, err)
		}
		responses = append(responses, resp)
	}
	return responses
}
