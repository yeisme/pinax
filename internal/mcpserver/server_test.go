package mcpserver

import (
	"context"
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
