package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchDefaultOutputShowsResultPathAndSnippet(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Demo Note", "--body", "这个 demo 片段应该直接出现在搜索结果里。", "--slug", "demo-note", "--vault", root, "--json")

	out := runCLI(t, "search", "demo", "--vault", root)
	for _, want := range []string{"demo-note.md", "Demo Note", "demo 片段"} {
		if !strings.Contains(out, want) {
			t.Fatalf("search default output missing %q:\n%s", want, out)
		}
	}
}

func TestDatabaseSchemaAndViewRegistryV2CLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "A", "--body", "priority:: 2", "--status", "active", "--vault", root, "--json")
	inferOut := runCLI(t, "database", "schema", "infer", "--vault", root, "--json")
	var inferEnvelope map[string]any
	if err := json.Unmarshal([]byte(inferOut), &inferEnvelope); err != nil {
		t.Fatalf("schema infer json invalid: %v\n%s", err, inferOut)
	}
	if inferEnvelope["command"] != "database.schema.infer" || inferEnvelope["facts"].(map[string]any)["properties"] == "0" || !strings.Contains(inferOut, "priority") {
		t.Fatalf("schema infer envelope = %#v", inferEnvelope)
	}
	if fileExists(filepath.Join(root, ".pinax", "schema-overrides.json")) {
		t.Fatalf("schema infer wrote overrides asset")
	}
	setOut := runCLI(t, "database", "schema", "set", "status", "--type", "select", "--values", "active,done", "--vault", root, "--json")
	if !strings.Contains(setOut, "database.schema.set") || !strings.Contains(readCLIFile(t, filepath.Join(root, ".pinax", "schema-overrides.json")), "status") {
		t.Fatalf("schema set failed:\n%s", setOut)
	}
	checkOut := runCLI(t, "database", "schema", "set", "published", "--type", "checkbox", "--vault", root, "--json")
	if !strings.Contains(checkOut, `"type":"checkbox"`) {
		t.Fatalf("schema set checkbox failed:\n%s", checkOut)
	}
	listOut := runCLI(t, "database", "schema", "list", "--vault", root, "--json")
	if !strings.Contains(listOut, "database.schema.list") || !strings.Contains(listOut, "published") || !strings.Contains(listOut, "checkbox") {
		t.Fatalf("schema list output invalid:\n%s", listOut)
	}
	showOut := runCLI(t, "database", "schema", "show", "published", "--vault", root, "--json")
	if !strings.Contains(showOut, "database.schema.show") || !strings.Contains(showOut, `"property":"published"`) || !strings.Contains(showOut, `"type":"checkbox"`) {
		t.Fatalf("schema show output invalid:\n%s", showOut)
	}
	viewOut := runCLI(t, "database", "view", "save", "sql-active", "--query", "SELECT title FROM notes LIMIT 20", "--kind", "table", "--column", "title", "--vault", root, "--json")
	if !strings.Contains(viewOut, "database.view.save") {
		t.Fatalf("database query view save output = %s", viewOut)
	}
	views := readCLIFile(t, filepath.Join(root, ".pinax", "views.json"))
	for _, want := range []string{"pinax.views.v3", "sql-active", "SELECT title FROM notes LIMIT 20", "columns", "language"} {
		if !strings.Contains(views, want) {
			t.Fatalf("views registry missing %q:\n%s", want, views)
		}
	}
}

func TestDatabaseViewV3DataviewRenderCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Active", "--body", "priority:: 2", "--status", "active", "--tags", "pinax", "--vault", root, "--json")
	out := runCLI(t, "database", "view", "save", "dv-active", "--language", "dataview", "--query", `TABLE title, status FROM #pinax LIMIT 5`, "--kind", "table", "--group-by", "status", "--calendar-field", "due", "--board-column", "status", "--vault", root, "--json")
	if !strings.Contains(out, "database.view.save") || !strings.Contains(out, "dataview") {
		t.Fatalf("database view save dataview output = %s", out)
	}
	views := readCLIFile(t, filepath.Join(root, ".pinax", "views.json"))
	for _, want := range []string{"pinax.views.v3", "dv-active", "dataview", "group_by", "calendar_field", "board_column"} {
		if !strings.Contains(views, want) {
			t.Fatalf("v3 views registry missing %q:\n%s", want, views)
		}
	}
	renderOut := runCLI(t, "database", "view", "render", "dv-active", "--vault", root, "--json")
	if !strings.Contains(renderOut, "database.view.render") || !strings.Contains(renderOut, "Active") {
		t.Fatalf("database view render output = %s", renderOut)
	}
}

func TestDatabaseViewDisplayRenderCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Active", "--body", "due:: 2026-06-21", "--status", "active", "--vault", root, "--json")
	runCLI(t, "note", "new", "Done", "--body", "due:: 2026-06-22", "--status", "done", "--vault", root, "--json")
	boardOut := runCLI(t, "database", "view", "save", "board-status", "--display", "board", "--query", "SELECT title, status FROM notes LIMIT 10", "--board-column", "status", "--vault", root, "--json")
	if !strings.Contains(boardOut, "database.view.save") || !strings.Contains(boardOut, `"display":"board"`) {
		t.Fatalf("board view save output invalid:\n%s", boardOut)
	}
	views := readCLIFile(t, filepath.Join(root, ".pinax", "views.json"))
	if !strings.Contains(views, `"kind": "board"`) || strings.Contains(views, `"rows"`) {
		t.Fatalf("view registry should save config only:\n%s", views)
	}
	boardRender := runCLI(t, "database", "view", "render", "board-status", "--vault", root, "--json")
	for _, want := range []string{`"command":"database.view.render"`, `"display":"board"`, `"board_columns":"2"`, "Active", "Done"} {
		if !strings.Contains(boardRender, want) {
			t.Fatalf("board render missing %s:\n%s", want, boardRender)
		}
	}
	boardAgent := runCLI(t, "database", "view", "render", "board-status", "--vault", root, "--agent")
	for _, want := range []string{"command=database.view.render", "fact.database.view=board-status", "fact.database.display=board", "fact.database_tab.name=board-status"} {
		if !strings.Contains(boardAgent, want) {
			t.Fatalf("board render agent missing %s:\n%s", want, boardAgent)
		}
	}

	calendarOut := runCLI(t, "database", "view", "save", "calendar-due", "--display", "calendar", "--query", "SELECT title, due FROM notes LIMIT 10", "--calendar-field", "due", "--vault", root, "--json")
	if !strings.Contains(calendarOut, `"display":"calendar"`) {
		t.Fatalf("calendar view save output invalid:\n%s", calendarOut)
	}
	calendarRender := runCLI(t, "database", "view", "render", "calendar-due", "--vault", root, "--json")
	for _, want := range []string{`"display":"calendar"`, `"calendar_events":"2"`, "2026-06-21", "2026-06-22"} {
		if !strings.Contains(calendarRender, want) {
			t.Fatalf("calendar render missing %s:\n%s", want, calendarRender)
		}
	}
	badOut := runCLI(t, "database", "view", "save", "bad-calendar", "--display", "calendar", "--query", "SELECT title FROM notes LIMIT 10", "--vault", root, "--json")
	if !strings.Contains(badOut, `"display":"calendar"`) {
		t.Fatalf("bad calendar save output invalid:\n%s", badOut)
	}
	failed, err := runCLIExpectError("database", "view", "render", "bad-calendar", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, `"code":"calendar_field_required"`) {
		t.Fatalf("bad calendar should fail with calendar_field_required err=%v out=%s", err, failed)
	}
}

func TestQueryRunOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Active", "--body", "priority:: 2", "--status", "active", "--tags", "pinax", "--vault", root, "--json")
	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	failed, err := runCLIExpectError("query", "run", `SELECT title FROM notes LIMIT 5`, "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "index_required") {
		t.Fatalf("query run without index err=%v out=%s", err, failed)
	}
	out := runCLI(t, "query", "run", `SELECT title, status FROM notes WHERE status = "active" LIMIT 5`, "--lazy-index", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("query run json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "query.run" || facts["rows"] != "1" || facts["columns"] != "title,status" || facts["index_loaded"] != "lazy_rebuild" {
		t.Fatalf("query run envelope = %#v", envelope)
	}
}

func TestDataviewRunOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Active", "--body", "priority:: 2", "--status", "active", "--tags", "pinax", "--vault", root, "--json")
	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	out := runCLI(t, "dataview", "run", `TABLE title, status FROM #pinax WHERE status = "active" LIMIT 5`, "--lazy-index", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("dataview run json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "dataview.run" || facts["rows"] != "1" || facts["columns"] != "title,status" || facts["index_loaded"] != "lazy_rebuild" {
		t.Fatalf("dataview run envelope = %#v", envelope)
	}
}

func TestDataviewExplainOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out := runCLI(t, "dataview", "explain", `LIST FROM #pinax LIMIT 5`, "--vault", root, "--agent")
	for _, want := range []string{"command=dataview.explain", "fact.source=notes", "fact.columns=title", "fact.limit=5"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dataview explain agent missing %q:\n%s", want, out)
		}
	}
}

func TestQueryExplainOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out := runCLI(t, "query", "explain", `SELECT title, status FROM notes WHERE status = "active" LIMIT 10`, "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("query explain json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "query.explain" || envelope["mode"] != "json" || facts["source"] != "notes" || facts["columns"] != "title,status" || facts["limit"] != "10" {
		t.Fatalf("query explain envelope = %#v", envelope)
	}
	agent := runCLI(t, "query", "explain", "SELECT title FROM notes LIMIT 5", "--vault", root, "--agent")
	for _, want := range []string{"command=query.explain", "fact.source=notes", "fact.columns=title", "fact.limit=5"} {
		if !strings.Contains(agent, want) {
			t.Fatalf("query explain agent missing %q:\n%s", want, agent)
		}
	}
}

func TestDatabaseViewQueryCompletionAndHelp(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Active Note", "--body", "body", "--tags", "pinax", "--kind", "reference", "--status", "active", "--vault", root, "--json")
	runCLI(t, "view", "save", "active-notes", "--status", "active", "--vault", root, "--json")

	viewCompletion := runCLI(t, "__complete", "view", "show", "--vault", root, "")
	for _, want := range []string{"active-notes\tview", "ShellCompDirectiveNoFileComp"} {
		if !strings.Contains(viewCompletion, want) {
			t.Fatalf("view show completion missing %q:\n%s", want, viewCompletion)
		}
	}

	databaseViewCompletion := runCLI(t, "__complete", "database", "view", "show", "--vault", root, "")
	if !strings.Contains(databaseViewCompletion, "active-notes\tview") || !strings.Contains(databaseViewCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("database view completion = %s", databaseViewCompletion)
	}

	noteCompletion := runCLI(t, "__complete", "note", "show", "--vault", root, "")
	if !strings.Contains(noteCompletion, "Active Note\tnote") || !strings.Contains(noteCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("note show completion = %s", noteCompletion)
	}

	searchSortCompletion := runCLI(t, "__complete", "search", "query", "--vault", root, "--sort", "")
	if !strings.Contains(searchSortCompletion, "relevance\tsort") || !strings.Contains(searchSortCompletion, "updated\tsort") {
		t.Fatalf("search sort completion = %s", searchSortCompletion)
	}

	noteStatusCompletion := runCLI(t, "__complete", "note", "list", "--vault", root, "--status", "")
	if !strings.Contains(noteStatusCompletion, "active\tstatus") || !strings.Contains(noteStatusCompletion, "done\tstatus") {
		t.Fatalf("note status completion = %s", noteStatusCompletion)
	}

	queryRunSortCompletion := runCLI(t, "__complete", "query", "run", "SELECT title FROM notes", "--vault", root, "--sort", "")
	if !strings.Contains(queryRunSortCompletion, "title\tproperty") || !strings.Contains(queryRunSortCompletion, "updated_at\tproperty") {
		t.Fatalf("query run sort completion = %s", queryRunSortCompletion)
	}

	schemaTypeCompletion := runCLI(t, "__complete", "database", "schema", "set", "status", "--vault", root, "--type", "")
	if !strings.Contains(schemaTypeCompletion, "select\ttype") || !strings.Contains(schemaTypeCompletion, "date\ttype") || !strings.Contains(schemaTypeCompletion, "checkbox\ttype") || !strings.Contains(schemaTypeCompletion, "relation\ttype") || !strings.Contains(schemaTypeCompletion, "formula\ttype") {
		t.Fatalf("database schema type completion = %s", schemaTypeCompletion)
	}

	help := runCLI(t, "query", "--help")
	for _, want := range []string{"index status", "query explain", "query run", "database view save"} {
		if !strings.Contains(help, want) {
			t.Fatalf("query help missing %q:\n%s", want, help)
		}
	}
}

func TestSavedViewsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "note", "new", "工作参考", "--group", "work", "--folder", "refs", "--kind", "reference", "--status", "active", "--tags", "work", "--body", "正文", "--slug", "work-ref", "--vault", root, "--json")
	runCLI(t, "note", "new", "个人草稿", "--folder", "drafts", "--kind", "fleeting", "--status", "draft", "--tags", "personal", "--body", "正文", "--slug", "personal-draft", "--vault", root, "--json")

	saveOut := runCLI(t, "view", "save", "active-work", "--group", "work", "--status", "active", "--kind", "reference", "--sort", "title", "--vault", root, "--json")
	var saveEnvelope map[string]any
	if err := json.Unmarshal([]byte(saveOut), &saveEnvelope); err != nil {
		t.Fatalf("view save json invalid: %v\n%s", err, saveOut)
	}
	if saveEnvelope["command"] != "view.save" || saveEnvelope["facts"].(map[string]any)["view"] != "active-work" {
		t.Fatalf("view save envelope = %#v", saveEnvelope)
	}
	viewsPath := filepath.Join(root, ".pinax", "views.json")
	viewsContent := readCLIFile(t, viewsPath)
	if !strings.Contains(viewsContent, "active-work") || !strings.Contains(viewsContent, "pinax.views.v1") {
		t.Fatalf("views asset invalid:\n%s", viewsContent)
	}

	listOut := runCLI(t, "view", "list", "--vault", root, "--json")
	if !strings.Contains(listOut, "view.list") || !strings.Contains(listOut, "active-work") {
		t.Fatalf("view list output invalid:\n%s", listOut)
	}
	showOut := runCLI(t, "view", "show", "active-work", "--vault", root, "--json")
	var showEnvelope map[string]any
	if err := json.Unmarshal([]byte(showOut), &showEnvelope); err != nil {
		t.Fatalf("view show json invalid: %v\n%s", err, showOut)
	}
	showFacts := showEnvelope["facts"].(map[string]any)
	if showEnvelope["command"] != "view.show" || showFacts["view"] != "active-work" || showFacts["returned"] != "1" || !strings.Contains(showOut, "工作参考") || strings.Contains(showOut, "个人草稿") {
		t.Fatalf("view show envelope = %#v out=%s", showEnvelope, showOut)
	}

	dbSaveOut := runCLI(t, "database", "view", "save", "db-active", "--group", "work", "--status", "active", "--kind", "reference", "--sort", "title", "--vault", root, "--json")
	var dbSaveEnvelope map[string]any
	if err := json.Unmarshal([]byte(dbSaveOut), &dbSaveEnvelope); err != nil {
		t.Fatalf("database view save json invalid: %v\n%s", err, dbSaveOut)
	}
	if dbSaveEnvelope["command"] != "database.view.save" || dbSaveEnvelope["facts"].(map[string]any)["view"] != "db-active" {
		t.Fatalf("database view save envelope = %#v", dbSaveEnvelope)
	}

	dbListOut := runCLI(t, "database", "view", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=database.view.list", "fact.views=2"} {
		if !strings.Contains(dbListOut, want) {
			t.Fatalf("database view list missing %q:\n%s", want, dbListOut)
		}
	}

	dbShowOut := runCLI(t, "database", "view", "show", "db-active", "--vault", root, "--json")
	var dbShowEnvelope map[string]any
	if err := json.Unmarshal([]byte(dbShowOut), &dbShowEnvelope); err != nil {
		t.Fatalf("database view show json invalid: %v\n%s", err, dbShowOut)
	}
	if dbShowEnvelope["command"] != "database.view.show" || dbShowEnvelope["facts"].(map[string]any)["returned"] != "1" || !strings.Contains(dbShowOut, "工作参考") {
		t.Fatalf("database view show envelope = %#v out=%s", dbShowEnvelope, dbShowOut)
	}

	dbDeleteFailed, err := runCLIExpectError("database", "view", "delete", "db-active", "--vault", root, "--json")
	if err == nil || !strings.Contains(dbDeleteFailed, "approval_required") {
		t.Fatalf("database view delete without approval err=%v out=%s", err, dbDeleteFailed)
	}
	dbDeleteOut := runCLI(t, "database", "view", "delete", "db-active", "--yes", "--vault", root, "--json")
	if !strings.Contains(dbDeleteOut, "database.view.delete") || strings.Contains(readCLIFile(t, viewsPath), "db-active") {
		t.Fatalf("database view delete failed:\n%s\nviews=%s", dbDeleteOut, readCLIFile(t, viewsPath))
	}

	failed, err := runCLIExpectError("view", "delete", "active-work", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("view delete without approval err=%v out=%s", err, failed)
	}
	deleteOut := runCLI(t, "view", "delete", "active-work", "--yes", "--vault", root, "--json")
	if !strings.Contains(deleteOut, "view.delete") || strings.Contains(readCLIFile(t, viewsPath), "active-work") {
		t.Fatalf("view delete failed:\n%s\nviews=%s", deleteOut, readCLIFile(t, viewsPath))
	}
	if !fileExists(filepath.Join(root, "notes", "work", "refs", "work-ref.md")) {
		t.Fatalf("view delete removed note")
	}
}

func TestSearchLinkTargetCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nSource mentions [[Beta]] and [[Missing Target]].\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nkind: reference\n---\n\n# Beta\n\nTarget note.\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	for _, target := range []string{"Beta", "notes/beta.md", "note_beta", "Missing Target"} {
		out := runCLI(t, "search", "Source", "--link-target", target, "--vault", root, "--json")
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("search link target json invalid for %q: %v\n%s", target, err, out)
		}
		facts := envelope["facts"].(map[string]any)
		resultPath := firstSearchResultPath(envelope)
		if envelope["command"] != "note.search" || facts["filter.link_target"] != target || facts["returned"] != "1" || resultPath != "notes/alpha.md" {
			t.Fatalf("search link target %q envelope = %#v out=%s", target, envelope, out)
		}
	}

	failed, err := runCLIExpectError("search", "Source", "--link-target", "   ", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "invalid_link_filter") {
		t.Fatalf("invalid link filter err=%v out=%s", err, failed)
	}
}

func TestSearchLinkTargetAmbiguousCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "Source.md"), pinaxNoteFixture("note_source", "Source", "[]", "Source links to [[Shared]].\n"))
	writeCLIFixture(t, filepath.Join(root, "First.md"), pinaxNoteFixture("note_first", "Shared", "[]", "first\n"))
	writeCLIFixture(t, filepath.Join(root, "Second.md"), pinaxNoteFixture("note_second", "Shared", "[]", "second\n"))

	failed, err := runCLIExpectError("search", "Source", "--link-target", "Shared", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "link_target_ambiguous") || !strings.Contains(failed, "First.md") || !strings.Contains(failed, "Second.md") {
		t.Fatalf("ambiguous link target err=%v out=%s", err, failed)
	}
}

func TestIndexSearchDatabaseAndFiltersCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "note", "new", "认证方案", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--tags", "auth,design", "--body", "OAuth 登录设计 [[Identity]]", "--slug", "auth-design", "--vault", root, "--json")

	initOut := runCLI(t, "index", "init", "--vault", root, "--json")
	var initEnvelope map[string]any
	if err := json.Unmarshal([]byte(initOut), &initEnvelope); err != nil {
		t.Fatalf("index init json invalid: %v\n%s", err, initOut)
	}
	if initEnvelope["command"] != "index.init" || initEnvelope["status"] != "success" {
		t.Fatalf("index init envelope = %#v", initEnvelope)
	}
	initFacts := initEnvelope["facts"].(map[string]any)
	if initFacts["schema_version"] == "" || initFacts["index_status"] == "" || initFacts["path"] != ".pinax/index.sqlite" {
		t.Fatalf("index init facts = %#v", initFacts)
	}

	rebuildOut := runCLI(t, "index", "rebuild", "--vault", root, "--json")
	var rebuildEnvelope map[string]any
	if err := json.Unmarshal([]byte(rebuildOut), &rebuildEnvelope); err != nil {
		t.Fatalf("index rebuild json invalid: %v\n%s", err, rebuildOut)
	}
	rebuildFacts := rebuildEnvelope["facts"].(map[string]any)
	for _, key := range []string{"notes", "tags", "links", "tokens", "dimensions", "schema_version"} {
		if value, ok := rebuildFacts[key]; !ok || value == "" {
			t.Fatalf("index rebuild facts missing %s: %#v", key, rebuildFacts)
		}
	}

	statusOut := runCLI(t, "index", "status", "--vault", root, "--json")
	var statusEnvelope map[string]any
	if err := json.Unmarshal([]byte(statusOut), &statusEnvelope); err != nil {
		t.Fatalf("index status json invalid: %v\n%s", err, statusOut)
	}
	statusFacts := statusEnvelope["facts"].(map[string]any)
	if statusEnvelope["command"] != "index.status" || statusFacts["index_status"] != "fresh" || statusFacts["schema_version"] == "" {
		t.Fatalf("index status envelope = %#v", statusEnvelope)
	}

	searchOut := runCLI(t, "search", "认证", "--tag", "auth", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--limit", "5", "--vault", root, "--json")
	var searchEnvelope map[string]any
	if err := json.Unmarshal([]byte(searchOut), &searchEnvelope); err != nil {
		t.Fatalf("search json invalid: %v\n%s", err, searchOut)
	}
	searchFacts := searchEnvelope["facts"].(map[string]any)
	for _, want := range []string{"engine", "index_status", "total", "returned", "filter.tag", "filter.group", "filter.folder", "filter.kind", "filter.status"} {
		if searchFacts[want] == "" {
			t.Fatalf("search facts missing %s: %#v", want, searchFacts)
		}
	}
	if searchFacts["engine"] != "index" || searchFacts["index_status"] != "fresh" || searchFacts["returned"] != "1" {
		t.Fatalf("search facts = %#v", searchFacts)
	}
	data := searchEnvelope["data"].(map[string]any)
	results := data["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("search results = %#v", data)
	}
	result := results[0].(map[string]any)
	if result["score"] == nil || len(result["matched_fields"].([]any)) == 0 || result["snippet"] == "" {
		t.Fatalf("search result missing score/matches/snippet: %#v", result)
	}

	runCLI(t, "note", "new", "Auth A", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--tags", "auth", "--body", "认证 checklist", "--slug", "a-auth", "--vault", root, "--json")
	sortedOut := runCLI(t, "search", "认证", "--tag", "auth", "--sort", "title", "--limit", "1", "--vault", root, "--json")
	var sortedEnvelope map[string]any
	if err := json.Unmarshal([]byte(sortedOut), &sortedEnvelope); err != nil {
		t.Fatalf("sorted search json invalid: %v\n%s", err, sortedOut)
	}
	sortedFacts := sortedEnvelope["facts"].(map[string]any)
	if sortedFacts["sort"] != "title" || sortedFacts["returned"] != "1" {
		t.Fatalf("sorted search facts = %#v", sortedFacts)
	}
	sortedResults := sortedEnvelope["data"].(map[string]any)["results"].([]any)
	sortedNote := sortedResults[0].(map[string]any)["note"].(map[string]any)
	if sortedNote["title"] != "Auth A" {
		t.Fatalf("sorted search first note = %#v", sortedNote)
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "work", "architecture", "auth-design.md"), strings.Replace(readCLIFile(t, filepath.Join(root, "notes", "work", "architecture", "auth-design.md")), "OAuth", "OAuth2", 1))
	staleOut := runCLI(t, "index", "status", "--vault", root, "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("stale status json invalid: %v\n%s", err, staleOut)
	}
	if staleEnvelope["facts"].(map[string]any)["index_status"] != "stale" {
		t.Fatalf("expected stale index after note change: %#v", staleEnvelope)
	}

	staleSearchOut := runCLI(t, "search", "认证", "--allow-stale", "--vault", root, "--json")
	var staleSearchEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleSearchOut), &staleSearchEnvelope); err != nil {
		t.Fatalf("stale search json invalid: %v\n%s", err, staleSearchOut)
	}
	if staleSearchEnvelope["status"] != "partial" || staleSearchEnvelope["facts"].(map[string]any)["engine"] != "index" || staleSearchEnvelope["facts"].(map[string]any)["index_status"] != "stale" {
		t.Fatalf("stale search envelope = %#v", staleSearchEnvelope)
	}
	if len(staleSearchEnvelope["actions"].([]any)) == 0 {
		t.Fatalf("stale search missing rebuild action: %#v", staleSearchEnvelope)
	}

	invalidDate, err := runCLIExpectError("search", "认证", "--updated-after", "not-a-date", "--vault", root, "--json")
	if err == nil || !strings.Contains(invalidDate, "invalid_date_filter") {
		t.Fatalf("invalid date filter err=%v out=%s", err, invalidDate)
	}

	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("remove index for fallback search: %v", err)
	}
	fallbackOut := runCLI(t, "search", "认证", "--vault", root, "--json")
	var fallbackEnvelope map[string]any
	if err := json.Unmarshal([]byte(fallbackOut), &fallbackEnvelope); err != nil {
		t.Fatalf("fallback search json invalid: %v\n%s", err, fallbackOut)
	}
	fallbackFacts := fallbackEnvelope["facts"].(map[string]any)
	if fallbackFacts["engine"] != "index" || fallbackFacts["index_status"] != "fresh" || fallbackFacts["index_loaded"] != "lazy_rebuild" || fallbackFacts["returned"] != "2" {
		t.Fatalf("lazy search facts = %#v", fallbackFacts)
	}
	if !fileExists(filepath.Join(root, ".pinax", "index.sqlite")) {
		t.Fatalf("lazy search did not recreate index.sqlite")
	}
}

func TestIndexLookupContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "yeisme.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_yeisme\ntitle: Yeisme Note\ntags: []\n---\n\n# Yeisme Note\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "unmanaged-yeisme.md"), "# Unmanaged Yeisme\n\nraw markdown\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "diagram.md"), "# Diagram\n\nunmanaged diagram markdown\n")
	assetSource := filepath.Join(root, "diagram.png")
	if err := os.WriteFile(assetSource, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 'd', 'i', 'a', 'g'}, 0o644); err != nil {
		t.Fatalf("write asset source: %v", err)
	}
	runCLI(t, "asset", "add", assetSource, "--vault", root, "--json")
	runCLI(t, "index", "refresh", "--vault", root, "--json")

	lookupOut := runCLI(t, "index", "lookup", "yeisme", "--scope", "all", "--vault", root, "--json")
	var lookupEnvelope map[string]any
	if err := json.Unmarshal([]byte(lookupOut), &lookupEnvelope); err != nil {
		t.Fatalf("index lookup json invalid: %v\n%s", err, lookupOut)
	}
	lookupFacts := lookupEnvelope["facts"].(map[string]any)
	if lookupEnvelope["command"] != "index.lookup" || lookupFacts["candidates"] != "2" || lookupFacts["index_status"] == "" {
		t.Fatalf("index lookup envelope = %#v", lookupEnvelope)
	}
	if !strings.Contains(lookupOut, `"object_kind":"note"`) || !strings.Contains(lookupOut, `"object_kind":"file"`) || !strings.Contains(lookupOut, `"managed_status":"registered"`) || !strings.Contains(lookupOut, `"managed_status":"adoptable"`) {
		t.Fatalf("index lookup missing note/file candidates:\n%s", lookupOut)
	}

	assetOut := runCLI(t, "index", "lookup", "diagram", "--kind", "asset", "--scope", "all", "--vault", root, "--agent")
	for _, want := range []string{"command=index.lookup", "fact.kind=asset", "fact.candidates=1", "candidate.1.object_kind=asset", "candidate.1.managed_status=managed", "candidate.1.path=assets/diagram.png"} {
		if !strings.Contains(assetOut, want) {
			t.Fatalf("index lookup asset agent missing %q:\n%s", want, assetOut)
		}
	}

	ambiguousOut := runCLI(t, "index", "lookup", "diagram", "--scope", "all", "--vault", root, "--json")
	var ambiguousEnvelope map[string]any
	if err := json.Unmarshal([]byte(ambiguousOut), &ambiguousEnvelope); err != nil {
		t.Fatalf("index lookup ambiguous json invalid: %v\n%s", err, ambiguousOut)
	}
	ambiguousFacts := ambiguousEnvelope["facts"].(map[string]any)
	if ambiguousFacts["candidates"] != "2" || !strings.Contains(ambiguousOut, `"object_kind":"asset"`) || !strings.Contains(ambiguousOut, `"object_kind":"file"`) {
		t.Fatalf("index lookup ambiguous envelope = %#v\n%s", ambiguousEnvelope, ambiguousOut)
	}

	searchOut := runCLI(t, "search", "Unmanaged Yeisme", "--vault", root, "--json")
	var searchEnvelope map[string]any
	if err := json.Unmarshal([]byte(searchOut), &searchEnvelope); err != nil {
		t.Fatalf("search json invalid: %v\n%s", err, searchOut)
	}
	if searchEnvelope["facts"].(map[string]any)["returned"] != "0" {
		t.Fatalf("unmanaged markdown entered ordinary search: %#v", searchEnvelope)
	}
}

func TestIndexDefaultSummaryAndMachineContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Index Test", "--body", "body", "--slug", "index-contract", "--vault", root, "--json")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove index: %v", err)
	}

	humanOut := runCLI(t, "index", "--vault", root)
	for _, want := range []string{"Status", "Local index", "Next step", "pinax index refresh --vault"} {
		if !strings.Contains(humanOut, want) {
			t.Fatalf("index summary missing %q:\n%s", want, humanOut)
		}
	}
	if fileExists(indexPath) {
		t.Fatalf("default index summary created index.sqlite")
	}

	jsonOut := runCLI(t, "index", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("index summary json invalid: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "index.summary" || envelope["status"] != "partial" || envelope["mode"] != "json" {
		t.Fatalf("index summary envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	for _, key := range []string{"index_status", "path", "schema_version", "notes", "recommended_action", "writes"} {
		if _, ok := facts[key]; !ok {
			t.Fatalf("index summary facts missing %s: %#v", key, facts)
		}
	}
	if facts["index_status"] != "missing" || facts["writes"] != "false" || !strings.Contains(fmt.Sprint(facts["recommended_action"]), "pinax index refresh --vault") {
		t.Fatalf("index summary facts = %#v", facts)
	}
	if fileExists(indexPath) {
		t.Fatalf("json index summary created index.sqlite")
	}
	statusOut := runCLI(t, "index", "status", "--vault", root, "--json")
	var statusEnvelope map[string]any
	if err := json.Unmarshal([]byte(statusOut), &statusEnvelope); err != nil {
		t.Fatalf("index status json invalid: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "pinax index refresh --vault") || strings.Contains(statusOut, "delete .pinax/index.sqlite") || strings.Contains(statusOut, "edit .pinax/index.sqlite") {
		t.Fatalf("index status next action unsafe or missing refresh:\n%s", statusOut)
	}

	agentOut := runCLI(t, "index", "--vault", root, "--agent")
	for _, want := range []string{"command=index.summary", "status=partial", "fact.index_status=missing", "fact.writes=false", "action.refresh="} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("index summary agent missing %q:\n%s", want, agentOut)
		}
	}
	for _, localized := range []string{"状态", "推荐下一步", "本地索引"} {
		if strings.Contains(agentOut, localized) {
			t.Fatalf("agent output contains localized prose %q:\n%s", localized, agentOut)
		}
	}
	if fileExists(indexPath) {
		t.Fatalf("agent index summary created index.sqlite")
	}
}

func TestIndexRefreshContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Managed Note", "--body", "managed body", "--slug", "managed", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "unmanaged.md"), "# Unmanaged\n\nraw markdown\n")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove index: %v", err)
	}

	missingOut := runCLI(t, "index", "refresh", "--vault", root, "--json")
	var missingEnvelope map[string]any
	if err := json.Unmarshal([]byte(missingOut), &missingEnvelope); err != nil {
		t.Fatalf("refresh missing json invalid: %v\n%s", err, missingOut)
	}
	missingFacts := missingEnvelope["facts"].(map[string]any)
	if missingEnvelope["command"] != "index.refresh" || missingEnvelope["status"] != "success" || missingFacts["index_status"] != "fresh" || missingFacts["scanned"] != "1" || missingFacts["indexed"] != "1" || missingFacts["failed"] != "0" {
		t.Fatalf("refresh missing envelope = %#v", missingEnvelope)
	}
	if !fileExists(indexPath) {
		t.Fatalf("refresh did not create missing index")
	}
	unmanagedSearch := runCLI(t, "search", "Unmanaged", "--vault", root, "--json")
	var unmanagedEnvelope map[string]any
	if err := json.Unmarshal([]byte(unmanagedSearch), &unmanagedEnvelope); err != nil {
		t.Fatalf("unmanaged search json invalid: %v\n%s", err, unmanagedSearch)
	}
	if unmanagedEnvelope["facts"].(map[string]any)["returned"] != "0" {
		t.Fatalf("unmanaged markdown entered index: %#v", unmanagedEnvelope)
	}

	freshOut := runCLI(t, "index", "refresh", "--vault", root, "--json")
	var freshEnvelope map[string]any
	if err := json.Unmarshal([]byte(freshOut), &freshEnvelope); err != nil {
		t.Fatalf("refresh fresh json invalid: %v\n%s", err, freshOut)
	}
	freshFacts := freshEnvelope["facts"].(map[string]any)
	if freshFacts["skipped"] != "1" || freshFacts["changed"] != "0" || freshFacts["indexed"] != "0" {
		t.Fatalf("refresh fresh facts = %#v", freshFacts)
	}

	changedSinceOut, changedSinceErr := runCLIExpectError("index", "refresh", "--changed-since", "rev_1", "--vault", root, "--json")
	if changedSinceErr == nil {
		t.Fatalf("changed-since refresh unexpectedly succeeded:\n%s", changedSinceOut)
	}
	var changedSinceEnvelope map[string]any
	if err := json.Unmarshal([]byte(changedSinceOut), &changedSinceEnvelope); err != nil {
		t.Fatalf("changed-since refresh json invalid: %v\n%s", err, changedSinceOut)
	}
	changedSinceError := changedSinceEnvelope["error"].(map[string]any)
	if changedSinceEnvelope["command"] != "index.refresh" || changedSinceEnvelope["status"] != "failed" || changedSinceError["code"] != "version_changed_paths_unavailable" {
		t.Fatalf("changed-since refresh envelope = %#v", changedSinceEnvelope)
	}

	managedPath := filepath.Join(root, "managed.md")
	writeCLIFixture(t, managedPath, strings.Replace(readCLIFile(t, managedPath), "managed body", "managed body changed", 1))
	staleOut := runCLI(t, "index", "refresh", "--vault", root, "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("refresh stale json invalid: %v\n%s", err, staleOut)
	}
	staleFacts := staleEnvelope["facts"].(map[string]any)
	if staleFacts["changed"] != "1" || staleFacts["indexed"] != "1" || staleFacts["index_status"] != "fresh" {
		t.Fatalf("refresh stale facts = %#v", staleFacts)
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "broken.md"), "---\nschema_version: pinax.note.v1\ntitle: Broken\n---\n\n# Broken\n")
	partialOut := runCLI(t, "index", "refresh", "--vault", root, "--json")
	var partialEnvelope map[string]any
	if err := json.Unmarshal([]byte(partialOut), &partialEnvelope); err != nil {
		t.Fatalf("refresh partial json invalid: %v\n%s", err, partialOut)
	}
	partialFacts := partialEnvelope["facts"].(map[string]any)
	if partialEnvelope["command"] != "index.refresh" || partialEnvelope["status"] != "partial" || partialFacts["failed"] != "1" || partialFacts["index_status"] != "partial" {
		t.Fatalf("refresh partial envelope = %#v", partialEnvelope)
	}
	if !strings.Contains(partialOut, "notes/broken.md") || !strings.Contains(partialOut, "pinax index doctor --vault") || !strings.Contains(partialOut, "pinax index rebuild --vault") {
		t.Fatalf("refresh partial missing evidence/actions:\n%s", partialOut)
	}
	partialAgent := runCLI(t, "index", "refresh", "--vault", root, "--agent")
	for _, want := range []string{"command=index.refresh", "status=partial", "fact.failed=1", "fact.index_status=partial", "action.doctor=", "action.rebuild="} {
		if !strings.Contains(partialAgent, want) {
			t.Fatalf("refresh partial agent missing %q:\n%s", want, partialAgent)
		}
	}
}

func TestIndexExplainCommandCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Explain Note", "--body", "explain body", "--slug", "explain", "--vault", root, "--json")

	help := runCLI(t, "index", "--help")
	if !strings.Contains(help, "explain") || !strings.Contains(help, "lookup") || !strings.Contains(help, "changed-since") {
		t.Fatalf("index help missing 7.6 commands/flags:\n%s", help)
	}

	out := runCLI(t, "index", "explain", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("index explain json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "index.explain" || facts["path"] != ".pinax/index.sqlite" || facts["index_status"] == "" {
		t.Fatalf("index explain envelope = %#v\n%s", envelope, out)
	}
	if !strings.Contains(out, "pinax search <query> --vault") && !strings.Contains(out, "pinax index refresh --vault") {
		t.Fatalf("index explain missing runnable action:\n%s", out)
	}
}

func TestIndexDoctorContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	runCLI(t, "note", "new", "Doctor Note", "--body", "doctor body", "--slug", "doctor", "--vault", root, "--json")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove index: %v", err)
	}

	missingOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var missingEnvelope map[string]any
	if err := json.Unmarshal([]byte(missingOut), &missingEnvelope); err != nil {
		t.Fatalf("doctor missing json invalid: %v\n%s", err, missingOut)
	}
	missingFacts := missingEnvelope["facts"].(map[string]any)
	if missingEnvelope["command"] != "index.doctor" || missingEnvelope["status"] != "partial" || missingFacts["issue_codes"] != "index_missing" || missingFacts["issues.total"] != "1" {
		t.Fatalf("doctor missing envelope = %#v", missingEnvelope)
	}
	if !strings.Contains(missingOut, "pinax index refresh --vault") || strings.Contains(missingOut, "delete .pinax/index.sqlite") {
		t.Fatalf("doctor missing action unsafe/missing:\n%s", missingOut)
	}

	runCLI(t, "index", "refresh", "--vault", root, "--json")
	freshOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var freshEnvelope map[string]any
	if err := json.Unmarshal([]byte(freshOut), &freshEnvelope); err != nil {
		t.Fatalf("doctor fresh json invalid: %v\n%s", err, freshOut)
	}
	freshFacts := freshEnvelope["facts"].(map[string]any)
	if freshEnvelope["status"] != "success" || freshFacts["index_status"] != "fresh" || freshFacts["issues.total"] != "0" {
		t.Fatalf("doctor fresh envelope = %#v\n%s", freshEnvelope, freshOut)
	}

	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	managedPath := filepath.Join(root, "doctor.md")
	writeCLIFixture(t, managedPath, strings.Replace(readCLIFile(t, managedPath), "doctor body", "doctor body changed", 1))
	staleOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("doctor stale json invalid: %v\n%s", err, staleOut)
	}
	staleFacts := staleEnvelope["facts"].(map[string]any)
	if staleFacts["issue_codes"] != "index_stale" || !strings.Contains(staleOut, "changed_note=doctor.md") || !strings.Contains(staleOut, "pinax index refresh --vault") {
		t.Fatalf("doctor stale envelope = %#v\n%s", staleEnvelope, staleOut)
	}

	if err := os.WriteFile(indexPath, []byte("not sqlite"), 0o644); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}
	corruptOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var corruptEnvelope map[string]any
	if err := json.Unmarshal([]byte(corruptOut), &corruptEnvelope); err != nil {
		t.Fatalf("doctor corrupt json invalid: %v\n%s", err, corruptOut)
	}
	corruptFacts := corruptEnvelope["facts"].(map[string]any)
	if corruptFacts["issue_codes"] != "index_unreadable" || !strings.Contains(corruptOut, "pinax index repair --vault") || strings.Contains(corruptOut, "delete .pinax/index.sqlite") {
		t.Fatalf("doctor corrupt envelope = %#v\n%s", corruptEnvelope, corruptOut)
	}
	corruptAgent := runCLI(t, "index", "doctor", "--vault", root, "--agent")
	for _, want := range []string{"command=index.doctor", "status=partial", "fact.issue_codes=index_unreadable", "issue.1.code=index_unreadable", "action.repair="} {
		if !strings.Contains(corruptAgent, want) {
			t.Fatalf("doctor corrupt agent missing %q:\n%s", want, corruptAgent)
		}
	}
}

func TestIndexRepairContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Repair Note", "--body", "repair body", "--slug", "repair", "--vault", root, "--json")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.WriteFile(indexPath, []byte("not sqlite"), 0o644); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}

	dryRunOut := runCLI(t, "index", "repair", "--vault", root, "--kind", "recreate", "--dry-run", "--json")
	var dryRunEnvelope map[string]any
	if err := json.Unmarshal([]byte(dryRunOut), &dryRunEnvelope); err != nil {
		t.Fatalf("repair dry-run json invalid: %v\n%s", err, dryRunOut)
	}
	dryRunFacts := dryRunEnvelope["facts"].(map[string]any)
	if dryRunEnvelope["command"] != "index.repair" || dryRunFacts["dry_run"] != "true" || dryRunFacts["writes"] != "false" || dryRunFacts["operations"] != "1" || !strings.Contains(dryRunOut, "recreate") {
		t.Fatalf("repair dry-run envelope = %#v\n%s", dryRunEnvelope, dryRunOut)
	}
	if got := readCLIFile(t, indexPath); got != "not sqlite" {
		t.Fatalf("dry-run modified index: %q", got)
	}

	failedOut, err := runCLIExpectError("index", "repair", "--vault", root, "--kind", "recreate", "--json")
	if err == nil || !strings.Contains(failedOut, "approval_required") {
		t.Fatalf("repair without approval err=%v out=%s", err, failedOut)
	}
	if got := readCLIFile(t, indexPath); got != "not sqlite" {
		t.Fatalf("approval failure modified index: %q", got)
	}

	applyOut := runCLI(t, "index", "repair", "--vault", root, "--kind", "recreate", "--yes", "--json")
	var applyEnvelope map[string]any
	if err := json.Unmarshal([]byte(applyOut), &applyEnvelope); err != nil {
		t.Fatalf("repair apply json invalid: %v\n%s", err, applyOut)
	}
	applyFacts := applyEnvelope["facts"].(map[string]any)
	if applyFacts["dry_run"] != "false" || applyFacts["writes"] != "true" || applyFacts["index_status"] != "fresh" || applyFacts["operations"] != "1" {
		t.Fatalf("repair apply facts = %#v", applyFacts)
	}
	if !strings.Contains(applyOut, ".pinax/index-backups/") {
		t.Fatalf("repair apply missing backup evidence:\n%s", applyOut)
	}
	doctorOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var doctorEnvelope map[string]any
	if err := json.Unmarshal([]byte(doctorOut), &doctorEnvelope); err != nil {
		t.Fatalf("doctor after repair json invalid: %v\n%s", err, doctorOut)
	}
	if doctorEnvelope["status"] != "success" || doctorEnvelope["facts"].(map[string]any)["index_status"] != "fresh" {
		t.Fatalf("doctor after repair = %#v", doctorEnvelope)
	}
}

func TestIndexMachineOutputContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Machine Note", "--body", "machine body", "--slug", "machine", "--vault", root, "--json")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove index: %v", err)
	}

	jsonStdout, jsonStderr, jsonErr := runCLISeparate("index", "doctor", "--vault", root, "--json")
	if jsonErr != nil || jsonStderr != "" {
		t.Fatalf("index doctor json err=%v stderr=%q stdout=%s", jsonErr, jsonStderr, jsonStdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonStdout), &envelope); err != nil {
		t.Fatalf("index doctor stdout is not JSON only: %v\n%s", err, jsonStdout)
	}
	if strings.Contains(jsonStdout, "\n\n") || strings.Contains(jsonStdout, "状态") || envelope["command"] != "index.doctor" {
		t.Fatalf("index doctor json contract invalid:\n%s", jsonStdout)
	}

	agentOut := runCLI(t, "index", "doctor", "--vault", root, "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=index.doctor", "status=partial", "fact.issue_codes=index_missing", "issue.1.code=index_missing"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("index doctor agent missing %q:\n%s", want, agentOut)
		}
	}
	for _, forbidden := range []string{"状态", "重点", "本地索引", "推荐下一步"} {
		if strings.Contains(agentOut, forbidden) {
			t.Fatalf("index doctor agent contains localized prose %q:\n%s", forbidden, agentOut)
		}
	}
	assertMachineOutputClean(t, agentOut)

	eventsOut := runCLI(t, "index", "repair", "--vault", root, "--kind", "recreate", "--dry-run", "--events")
	eventLines := strings.Split(strings.TrimSpace(eventsOut), "\n")
	if len(eventLines) != 2 {
		t.Fatalf("index repair events line count = %d\n%s", len(eventLines), eventsOut)
	}
	for i, line := range eventLines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("index repair event %d invalid: %v\n%s", i, err, eventsOut)
		}
	}
	if !strings.Contains(eventLines[0], `"type":"start"`) || !strings.Contains(eventLines[1], `"type":"end"`) || strings.Contains(eventsOut, "Status") || strings.Contains(eventsOut, "状态") {
		t.Fatalf("index repair events contract invalid:\n%s", eventsOut)
	}

	explainOut := runCLI(t, "index", "--vault", root, "--explain")
	for _, want := range []string{"Conclusion:", "Evidence:", "Confidence:", "Recommended next step:"} {
		if !strings.Contains(explainOut, want) {
			t.Fatalf("index explain missing %q:\n%s", want, explainOut)
		}
	}
	if !strings.Contains(explainOut, ".pinax/index.sqlite") || !strings.Contains(explainOut, "pinax index refresh --vault") {
		t.Fatalf("index explain missing evidence/action:\n%s", explainOut)
	}
}
