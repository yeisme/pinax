package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultRegistryDefaultAndCompletionCLI(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(stateRoot, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(stateRoot, "cache"))

	work := filepath.Join(stateRoot, "work-notes")
	personal := filepath.Join(stateRoot, "personal-notes")
	runCLI(t, "init", work, "--title", "Work", "--json")
	runCLI(t, "init", personal, "--title", "Personal", "--json")
	runCLI(t, "note", "new", "Work Alpha", "--body", "body", "--vault", work, "--json")
	runCLI(t, "note", "new", "Personal Beta", "--body", "body", "--vault", personal, "--json")

	registerOut := runCLI(t, "vault", "register", work, "--name", "work", "--default", "--json")
	assertJSONCommandStatus(t, registerOut, "vault.register", "success")
	runCLI(t, "vault", "register", personal, "--name", "personal", "--json")

	listOut := runCLI(t, "vault", "list", "--json")
	assertJSONCommandStatus(t, listOut, "vault.list", "success")
	if !strings.Contains(listOut, `"default":"work"`) || !strings.Contains(listOut, work) || !strings.Contains(listOut, personal) {
		t.Fatalf("vault list missing registry data: %s", listOut)
	}

	defaultNotes := runCLI(t, "note", "list", "--json")
	if !strings.Contains(defaultNotes, "Work Alpha") || strings.Contains(defaultNotes, "Personal Beta") {
		t.Fatalf("default vault note list did not use work alias: %s", defaultNotes)
	}

	runCLI(t, "vault", "use", "personal", "--json")
	personalNotes := runCLI(t, "note", "list", "--json")
	if !strings.Contains(personalNotes, "Personal Beta") || strings.Contains(personalNotes, "Work Alpha") {
		t.Fatalf("vault use did not switch default vault: %s", personalNotes)
	}

	vaultCompletion := runCLI(t, "__complete", "note", "list", "--vault", "")
	for _, want := range []string{"work\tlocal vault ", "personal\tlocal vault "} {
		if !strings.Contains(vaultCompletion, want) {
			t.Fatalf("vault completion missing %q:\n%s", want, vaultCompletion)
		}
	}
	if strings.Contains(vaultCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("vault completion should keep path completion enabled:\n%s", vaultCompletion)
	}

	pathCompletion := runCLI(t, "__complete", "note", "list", "--vault", filepath.Join(stateRoot, "work"))
	if !strings.Contains(pathCompletion, "work-notes/\tlocal directory") {
		t.Fatalf("vault path completion missing local directory:\n%s", pathCompletion)
	}

	noteCompletion := runCLI(t, "__complete", "note", "show", "--vault", "work", "")
	if !strings.Contains(noteCompletion, "Work Alpha\tnote") || strings.Contains(noteCompletion, "Personal Beta") {
		t.Fatalf("note completion did not resolve work alias:\n%s", noteCompletion)
	}
}

func TestVaultRemoteRefreshCacheCompletionCLI(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(stateRoot, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(stateRoot, "cache"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/vaults" {
			t.Fatalf("unexpected remote path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer remote-secret" {
			t.Fatalf("authorization header = %q", got)
		}
		_, _ = w.Write([]byte(`{"vaults":[{"id":"team","label":"Team Knowledge","workspace":"ws_team","revision":"rev_1"}]}`))
	}))
	defer server.Close()
	t.Setenv("PINAX_REMOTE_SECRET", "remote-secret")
	runCLI(t, "profile", "add", "cloud-work", "--endpoint", server.URL, "--workspace", "ws_team", "--device", "laptop", "--secret-ref", "env://PINAX_REMOTE_SECRET")

	refreshOut := runCLI(t, "vault", "remote", "refresh", "--profile", "cloud-work", "--json")
	assertJSONCommandStatus(t, refreshOut, "vault.remote.refresh", "success")
	if strings.Contains(refreshOut, "remote-secret") || strings.Contains(refreshOut, "Authorization") {
		t.Fatalf("remote refresh leaked secret: %s", refreshOut)
	}

	remoteList := runCLI(t, "vault", "remote", "list", "--profile", "cloud-work", "--json")
	assertJSONCommandStatus(t, remoteList, "vault.remote.list", "success")
	if !strings.Contains(remoteList, "cloud:team") || !strings.Contains(remoteList, "Team Knowledge") || strings.Contains(remoteList, "remote-secret") {
		t.Fatalf("remote list cache output invalid: %s", remoteList)
	}

	completion := runCLI(t, "__complete", "note", "list", "--vault", "cloud:")
	if !strings.Contains(completion, "cloud:team\tremote vault profile=cloud-work workspace=ws_team") {
		t.Fatalf("remote vault completion = %s", completion)
	}

	out, err := runCLIExpectError("note", "new", "Remote Write", "--vault", "cloud:team", "--json")
	if err == nil {
		t.Fatalf("remote selector write command succeeded: %s", out)
	}
	assertJSONErrorCode(t, out, "remote_vault_readonly")
}

func TestLocalVaultCLIJSONAndSafety(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "Inbox Note.md"), "# Inbox Note\n\nbody\n")

	out := runCLI(t, "organize", "plan", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "success" || envelope["mode"] != "json" {
		t.Fatalf("envelope = %#v", envelope)
	}

	errOut, err := runCLIExpectError("organize", "apply", "--vault", root, "--yes", "--json")
	if err == nil {
		t.Fatalf("organize apply without snapshot succeeded: %s", errOut)
	}
	if !strings.Contains(errOut, "snapshot_required") {
		t.Fatalf("expected snapshot_required envelope, got %s", errOut)
	}
}

func TestInitCommandRejectsAlreadyInitializedVault(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out, err := runCLIExpectError("init", root, "--title", "Other", "--json")
	if err == nil {
		t.Fatalf("second init succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("init error json invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "vault.init" || envelope["status"] != "failed" {
		t.Fatalf("init error envelope = %#v", envelope)
	}
	errorData, ok := envelope["error"].(map[string]any)
	if !ok || errorData["code"] != "vault_already_initialized" {
		t.Fatalf("init error data = %#v", envelope["error"])
	}

	events := readCLIFile(t, filepath.Join(root, ".pinax", "events.jsonl"))
	if got := strings.Count(events, `"type":"vault.init"`); got != 1 {
		t.Fatalf("vault.init events = %d\n%s", got, events)
	}
}

func TestProjectAndStorageCLIJSON(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	projectOut := runCLI(t, "project", "create", "research", "--name", "研究", "--description", "长期研究", "--notes-prefix", "notes/research", "--vault", root, "--json")
	var projectEnvelope map[string]any
	if err := json.Unmarshal([]byte(projectOut), &projectEnvelope); err != nil {
		t.Fatalf("project json invalid: %v\n%s", err, projectOut)
	}
	if projectEnvelope["command"] != "project.create" || projectEnvelope["status"] != "success" {
		t.Fatalf("project envelope = %#v", projectEnvelope)
	}

	listOut := runCLI(t, "project", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=project.list", "fact.projects=1", "fact.current_project=research", "fact.project.1.slug=research", "fact.project.1.name=研究", "fact.project.1.notes_prefix=notes/research"} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("project agent output missing %q:\n%s", want, listOut)
		}
	}

	storageOut := runCLI(t, "storage", "set-s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax/", "--profile", "work", "--vault", root, "--json")
	if strings.Contains(strings.ToLower(storageOut), "secret") || strings.Contains(strings.ToLower(storageOut), "access_key") {
		t.Fatalf("storage output leaked secret-like material:\n%s", storageOut)
	}
	var storageEnvelope map[string]any
	if err := json.Unmarshal([]byte(storageOut), &storageEnvelope); err != nil {
		t.Fatalf("storage json invalid: %v\n%s", err, storageOut)
	}
	if storageEnvelope["command"] != "storage.set_s3" || storageEnvelope["status"] != "success" {
		t.Fatalf("storage envelope = %#v", storageEnvelope)
	}
}

func TestProjectBoardAndNoteDisplayCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "研究", "--notes-prefix", "research", "--vault", root, "--json")
	nextOut := runCLI(t, "note", "add", "看板任务", "--project", "research", "--kind", "task", "--status", "active", "--body", "先做 projection，正文不能进 card。", "--vault", root, "--json")
	runCLI(t, "note", "add", "阻塞任务", "--project", "research", "--kind", "task", "--status", "blocked", "--body", "等待接口。", "--vault", root, "--json")

	boardOut := runCLI(t, "project", "board", "show", "research", "--note-display", "card", "--vault", root, "--json")
	var boardEnvelope map[string]any
	if err := json.Unmarshal([]byte(boardOut), &boardEnvelope); err != nil {
		t.Fatalf("board json invalid: %v\n%s", err, boardOut)
	}
	if boardEnvelope["command"] != "project.board.show" || boardEnvelope["status"] == "failed" {
		t.Fatalf("board envelope = %#v", boardEnvelope)
	}
	boardFacts := boardEnvelope["facts"].(map[string]any)
	if boardFacts["project"] != "research" || boardFacts["next"] != "1" || boardFacts["blocked"] != "1" || boardFacts["note_display"] != "card" {
		t.Fatalf("board facts = %#v", boardFacts)
	}
	if strings.Contains(boardOut, `"body"`) {
		t.Fatalf("board card output leaked body field:\n%s", boardOut)
	}

	var nextEnvelope map[string]any
	if err := json.Unmarshal([]byte(nextOut), &nextEnvelope); err != nil {
		t.Fatalf("note json invalid: %v\n%s", err, nextOut)
	}
	notePath := nextEnvelope["facts"].(map[string]any)["path"].(string)
	cardOut := runCLI(t, "note", "read", notePath, "--display", "card", "--vault", root, "--json")
	if strings.Contains(cardOut, `"body"`) {
		t.Fatalf("note card leaked body:\n%s", cardOut)
	}
	if !strings.Contains(cardOut, `"display":"card"`) || !strings.Contains(cardOut, `"excerpt"`) {
		t.Fatalf("note card missing display fields:\n%s", cardOut)
	}
	bodyOut := runCLI(t, "note", "read", notePath, "--display", "body", "--vault", root, "--json")
	if !strings.Contains(bodyOut, `"display":"body"`) || !strings.Contains(bodyOut, "正文不能进 card") {
		t.Fatalf("note body display missing body:\n%s", bodyOut)
	}

	configureOut := runCLI(t, "project", "board", "configure", "research", "--columns", "inbox,next,doing,blocked,review,done", "--vault", root, "--json")
	if !strings.Contains(configureOut, `"command":"project.board.configure"`) || !strings.Contains(configureOut, `"saved_path":".pinax/project-boards/research.json"`) {
		t.Fatalf("configure output = %s", configureOut)
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, ".pinax", "project-boards", "research.json")), `"schema_version":"pinax.project_board.v1"`) {
		t.Fatalf("board config was not written")
	}

	planOut := runCLI(t, "project", "board", "plan", "research", "--save", "--vault", root, "--json")
	if !strings.Contains(planOut, `"command":"project.board.plan"`) || !strings.Contains(planOut, `"saved_path":".pinax/planning/project-boards/`) {
		t.Fatalf("plan output = %s", planOut)
	}
	exportOut := runCLI(t, "project", "board", "export", "research", "--format", "markdown", "--vault", root, "--json")
	if !strings.Contains(exportOut, `"command":"project.board.export"`) || !strings.Contains(exportOut, "## next") || !strings.Contains(exportOut, "看板任务") {
		t.Fatalf("export output = %s", exportOut)
	}

	itemOut := runCLI(t, "project", "item", "add", "research", "实现 item flow", "--column", "next", "--body", "受控工作项", "--vault", root, "--json")
	if !strings.Contains(itemOut, `"command":"project.item.add"`) || !strings.Contains(itemOut, `"column":"next"`) {
		t.Fatalf("item add output = %s", itemOut)
	}
	var itemEnvelope map[string]any
	if err := json.Unmarshal([]byte(itemOut), &itemEnvelope); err != nil {
		t.Fatalf("item json invalid: %v\n%s", err, itemOut)
	}
	itemID := itemEnvelope["facts"].(map[string]any)["item_id"].(string)
	moveOut := runCLI(t, "project", "item", "move", itemID, "doing", "--vault", root, "--json")
	if !strings.Contains(moveOut, `"command":"project.item.move"`) || !strings.Contains(moveOut, `"column":"doing"`) {
		t.Fatalf("item move output = %s", moveOut)
	}
	moveDoneOut, moveDoneErr := runCLIExpectError("project", "item", "move", itemID, "done", "--vault", root, "--json")
	if moveDoneErr == nil || !strings.Contains(moveDoneOut, `"code":"approval_required"`) {
		t.Fatalf("move done without yes should require approval, err=%v out=%s", moveDoneErr, moveDoneOut)
	}
	moveDoneSnapshotOut, moveDoneSnapshotErr := runCLIExpectError("project", "item", "move", itemID, "done", "--yes", "--vault", root, "--json")
	if moveDoneSnapshotErr == nil || !strings.Contains(moveDoneSnapshotOut, `"code":"snapshot_required"`) || !strings.Contains(moveDoneSnapshotOut, "pinax version snapshot") {
		t.Fatalf("move done without snapshot should require snapshot, err=%v out=%s", moveDoneSnapshotErr, moveDoneSnapshotOut)
	}
	runCLI(t, "version", "snapshot", "--vault", root, "--message", "move done checkpoint", "--json")
	moveDoneOK := runCLI(t, "project", "item", "move", itemID, "done", "--yes", "--vault", root, "--json")
	if !strings.Contains(moveDoneOK, `"command":"project.item.move"`) || !strings.Contains(moveDoneOK, `"column":"done"`) {
		t.Fatalf("item move done output = %s", moveDoneOK)
	}
	archiveOut, archiveErr := runCLIExpectError("project", "item", "archive", itemID, "--vault", root, "--json")
	if archiveErr == nil || !strings.Contains(archiveOut, `"code":"approval_required"`) {
		t.Fatalf("archive without yes should require approval, err=%v out=%s", archiveErr, archiveOut)
	}
	archiveOK := runCLI(t, "project", "item", "archive", itemID, "--yes", "--vault", root, "--json")
	if !strings.Contains(archiveOK, `"command":"project.item.archive"`) || !strings.Contains(archiveOK, `"column":"done"`) {
		t.Fatalf("item archive output = %s", archiveOK)
	}
	apiRoutes := runCLI(t, "api", "routes", "--vault", root, "--json")
	if !strings.Contains(apiRoutes, `"command":"api.routes"`) || !strings.Contains(apiRoutes, "project.board.show") || !strings.Contains(apiRoutes, "/v1/projects/{slug}/board") {
		t.Fatalf("api routes output = %s", apiRoutes)
	}
	apiSchema := runCLI(t, "api", "schema", "export", "--format", "openapi", "--vault", root, "--json")
	if !strings.Contains(apiSchema, `"command":"api.schema.export"`) || !strings.Contains(apiSchema, `"openapi":"3.1.0"`) {
		t.Fatalf("api schema output = %s", apiSchema)
	}
}

func TestProjectSubprojectWorkspaceCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "Research", "--notes-prefix", "notes/research", "--vault", root, "--json")

	created := runCLI(t, "project", "subproject", "create", "research", "stock-learning", "--title", "Stock Learning", "--template", "scenario", "--vault", root, "--json")
	var createEnvelope map[string]any
	if err := json.Unmarshal([]byte(created), &createEnvelope); err != nil {
		t.Fatalf("subproject create json invalid: %v\n%s", err, created)
	}
	createFacts := createEnvelope["facts"].(map[string]any)
	if createEnvelope["command"] != "project.subproject.create" || createFacts["project"] != "research" || createFacts["subproject"] != "stock-learning" || createFacts["workspace_path"] != "notes/projects/research/stock-learning" || createFacts["directories"] != "7" || createFacts["writes"] != "true" {
		t.Fatalf("subproject create facts invalid: %#v\n%s", createFacts, created)
	}
	fullPath := filepath.Join(root, "notes", "projects", "research", "stock-learning")
	if createFacts["vault_root"] != root || createFacts["workspace.full_path"] != fullPath {
		t.Fatalf("subproject create path facts invalid: %#v\n%s", createFacts, created)
	}
	for _, rel := range []string{"charter", "inbox", "sources", "runs", "outputs", "retros", "tool-candidates"} {
		if !fileExists(filepath.Join(root, "notes", "projects", "research", "stock-learning", rel)) {
			t.Fatalf("subproject directory missing: %s", rel)
		}
	}
	registry := readCLIFile(t, filepath.Join(root, ".pinax", "project-workspaces", "research", "stock-learning.json"))
	if !strings.Contains(registry, `"schema_version": "pinax.project_workspace.v1"`) || !strings.Contains(registry, `"subproject": "stock-learning"`) {
		t.Fatalf("subproject registry invalid:\n%s", registry)
	}
	if strings.Contains(registry, root) || strings.Contains(registry, fullPath) {
		t.Fatalf("subproject registry must remain vault-portable, got absolute path:\n%s", registry)
	}
	currentRegistry := readCLIFile(t, filepath.Join(root, ".pinax", "workspaces", "current.json"))
	for _, want := range []string{`"schema_version": "pinax.current_workspace.v1"`, `"project": "research"`, `"subproject": "stock-learning"`, `"workspace_path": "notes/projects/research/stock-learning"`} {
		if !strings.Contains(currentRegistry, want) {
			t.Fatalf("current workspace registry missing %q:\n%s", want, currentRegistry)
		}
	}

	listOut := runCLI(t, "project", "subproject", "list", "research", "--vault", root, "--agent")
	for _, want := range []string{"command=project.subproject.list", "fact.project=research", "fact.subprojects=1", "fact.subproject.1=stock-learning", "fact.workspace.1=notes/projects/research/stock-learning"} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("subproject list agent missing %q:\n%s", want, listOut)
		}
	}

	showOut := runCLI(t, "project", "subproject", "show", "research", "stock-learning", "--vault", root, "--json")
	if !strings.Contains(showOut, `"command":"project.subproject.show"`) || !strings.Contains(showOut, `"directories"`) || !strings.Contains(showOut, `"charter"`) || strings.Contains(showOut, `"00-charter"`) || !strings.Contains(showOut, `"vault_root":"`+root+`"`) || !strings.Contains(showOut, `"workspace.full_path":"`+fullPath+`"`) {
		t.Fatalf("subproject show output invalid:\n%s", showOut)
	}
	showAgent := runCLI(t, "project", "subproject", "show", "research", "stock-learning", "--vault", root, "--agent")
	for _, want := range []string{"fact.workspace.project=research", "fact.workspace.subproject=stock-learning", "fact.workspace.path=notes/projects/research/stock-learning", "fact.vault_root=" + root, "fact.workspace.full_path=" + fullPath} {
		if !strings.Contains(showAgent, want) {
			t.Fatalf("subproject show agent missing %q:\n%s", want, showAgent)
		}
	}
	showHuman := runCLI(t, "project", "subproject", "show", "research", "stock-learning", "--vault", root, "--color", "never")
	for _, want := range []string{"Vault root", root, "Workspace path", "notes/projects/research/stock-learning", "Full path preview", fullPath} {
		if !strings.Contains(showHuman, want) {
			t.Fatalf("subproject show human missing %q:\n%s", want, showHuman)
		}
	}

	subprojectCompletion := runCLI(t, "__complete", "project", "board", "show", "research", "--vault", root, "--subproject", "")
	if !strings.Contains(subprojectCompletion, "stock-learning\tStock Learning") || !strings.Contains(subprojectCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("board show subproject completion invalid:\n%s", subprojectCompletion)
	}

	configureOut := runCLI(t, "project", "board", "configure", "research", "--subproject", "stock-learning", "--columns", "inbox,next,doing,blocked,review,done", "--vault", root, "--json")
	if !strings.Contains(configureOut, `"command":"project.board.configure"`) || !strings.Contains(configureOut, `.pinax/project-boards/research/stock-learning.json`) {
		t.Fatalf("subproject board configure output invalid:\n%s", configureOut)
	}
	itemOut := runCLI(t, "project", "item", "add", "research", "Run first research", "--subproject", "stock-learning", "--column", "next", "--labels", "research,learning", "--milestone", "phase-1", "--priority", "high", "--due-at", "2026-07-01", "--blocked-by", "item_source_review", "--vault", root, "--json")
	for _, want := range []string{`"command":"project.item.add"`, `"subproject":"stock-learning"`, `"labels":["research","learning"]`, `"milestone":"phase-1"`, `"priority":"high"`, `"due_at":"2026-07-01"`, `"blocked_by":["item_source_review"]`} {
		if !strings.Contains(itemOut, want) {
			t.Fatalf("project item add output missing %q:\n%s", want, itemOut)
		}
	}
	if !strings.Contains(itemOut, `notes/projects/research/stock-learning/inbox`) {
		t.Fatalf("subproject item path should live under workspace inbox:\n%s", itemOut)
	}

	boardJSON := runCLI(t, "project", "board", "show", "research", "--subproject", "stock-learning", "--vault", root, "--json")
	for _, want := range []string{`"command":"project.board.show"`, `"subproject":"stock-learning"`, `"workspace_path":"notes/projects/research/stock-learning"`, `"next":"1"`, `"labels":["research","learning"]`} {
		if !strings.Contains(boardJSON, want) {
			t.Fatalf("subproject board json missing %q:\n%s", want, boardJSON)
		}
	}
	boardAgent := runCLI(t, "project", "board", "show", "research", "--subproject", "stock-learning", "--vault", root, "--agent")
	for _, want := range []string{"mode=agent", "command=project.board.show", "fact.project=research", "fact.subproject=stock-learning", "fact.workspace_path=notes/projects/research/stock-learning", `item.1.title="Run first research"`, "item.1.column=next", "item.1.path=notes/projects/research/stock-learning/inbox/run-first-research.md", "item.1.subproject=stock-learning", "item.1.labels=research,learning", "action.board_plan="} {
		if !strings.Contains(boardAgent, want) {
			t.Fatalf("subproject board agent missing %q:\n%s", want, boardAgent)
		}
	}
	if strings.Contains(boardAgent, root) || strings.Contains(boardAgent, "受控工作项") || strings.Contains(boardAgent, "状态:") {
		t.Fatalf("subproject board agent leaked local path, body, or prose:\n%s", boardAgent)
	}
	human := runCLI(t, "project", "board", "show", "research", "--subproject", "stock-learning", "--vault", root, "--color", "never")
	for _, want := range []string{"Project: research / stock-learning", "Path: notes/projects/research/stock-learning", "Structure:", "Board:", "Next", "Run first research", "Recommended next step"} {
		if !strings.Contains(human, want) {
			t.Fatalf("subproject board human output missing %q:\n%s", want, human)
		}
	}
}

func TestTaskAdoptCLIPlansAndWritesLedger(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "Research", "--notes-prefix", "notes/research", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "checklist.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_checklist\ntitle: Checklist Source\nproject: research\nkind: reference\nstatus: active\n---\n\n- [ ] Review source material\n- [x] Already handled\n")

	boardOut := runCLI(t, "project", "board", "show", "research", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(boardOut), &envelope); err != nil {
		t.Fatalf("board json invalid: %v\n%s", err, boardOut)
	}
	data := envelope["data"].(map[string]any)
	board := data["board"].(map[string]any)
	items := board["items"].([]any)
	itemID := ""
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["title"] == "Review source material" {
			itemID = item["item_id"].(string)
			if item["source_status"] != "inferred" || item["writable"] != false {
				t.Fatalf("inferred task item invalid: %#v", item)
			}
		}
	}
	if itemID == "" {
		t.Fatalf("missing inferred task item in board:\n%s", boardOut)
	}

	planOut := runCLI(t, "task", "adopt", itemID, "--plan", "--vault", root, "--agent")
	for _, want := range []string{"command=task.adopt", "fact.writes=false", "fact.adopted=0", "fact.source_status=inferred"} {
		if !strings.Contains(planOut, want) {
			t.Fatalf("task adopt plan missing %q:\n%s", want, planOut)
		}
	}
	if fileExists(filepath.Join(root, ".pinax", "task-adoptions")) {
		t.Fatalf("task adopt --plan wrote ledger")
	}

	applyOut := runCLI(t, "task", "adopt", itemID, "--yes", "--vault", root, "--json")
	if !strings.Contains(applyOut, `"command":"task.adopt"`) || !strings.Contains(applyOut, `"writes":"true"`) || !strings.Contains(applyOut, `"ledger_path"`) {
		t.Fatalf("task adopt apply output invalid:\n%s", applyOut)
	}
	ledger := readCLIFile(t, filepath.Join(root, ".pinax", "task-adoptions", itemID+".json"))
	if !strings.Contains(ledger, `"schema_version": "pinax.task_adoption.v1"`) || !strings.Contains(ledger, `"source_status": "adopted"`) || strings.Contains(ledger, "Already handled") {
		t.Fatalf("task adoption ledger invalid:\n%s", ledger)
	}
}

func TestProjectBoardViewSaveAndShowCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "Research", "--notes-prefix", "notes/research", "--vault", root, "--json")
	runCLI(t, "project", "item", "add", "research", "Active item", "--column", "next", "--vault", root, "--json")
	runCLI(t, "project", "item", "add", "research", "Blocked item", "--column", "blocked", "--vault", root, "--json")

	saved := runCLI(t, "project", "board", "view", "save", "research", "active", "--columns", "next,doing", "--group", "column", "--sort", "priority", "--display", "card", "--vault", root, "--json")
	for _, want := range []string{`"command":"project.board.view.save"`, `"view":"active"`, `"writes":"true"`, `.pinax/project-board-views/research/active.json`} {
		if !strings.Contains(saved, want) {
			t.Fatalf("board view save missing %q:\n%s", want, saved)
		}
	}
	registry := readCLIFile(t, filepath.Join(root, ".pinax", "project-board-views", "research", "active.json"))
	for _, want := range []string{`"schema_version": "pinax.project_board_view.v1"`, `"view": "active"`, `"columns": [`, `"next"`, `"doing"`} {
		if !strings.Contains(registry, want) {
			t.Fatalf("board view registry missing %q:\n%s", want, registry)
		}
	}
	if strings.Contains(registry, "Active item") || strings.Contains(registry, "Blocked item") {
		t.Fatalf("board view registry should not save result rows:\n%s", registry)
	}

	show := runCLI(t, "project", "board", "show", "research", "--view", "active", "--vault", root, "--json")
	for _, want := range []string{`"command":"project.board.show"`, `"view":"active"`, `"column.next":"1"`, `"column.doing":"0"`} {
		if !strings.Contains(show, want) {
			t.Fatalf("board show --view missing %q:\n%s", want, show)
		}
	}
	if strings.Contains(show, `"column.blocked"`) {
		t.Fatalf("board show --view should use saved view columns only:\n%s", show)
	}
}

func TestProjectLearningInitCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	created := runCLI(t, "project", "learning", "init", "investing", "stock-learning", "--title", "学习炒股的全部笔记", "--project-name", "学习炒股", "--notes-prefix", "notes/investing", "--preset", "stock-learning", "--vault", root, "--json")
	for _, want := range []string{`"command":"project.learning.init"`, `"project":"investing"`, `"subproject":"stock-learning"`, `"preset":"stock-learning"`, `"notes.created":"3"`, `"items.created":"4"`, `"columns":"7"`} {
		if !strings.Contains(created, want) {
			t.Fatalf("learning init output missing %q:\n%s", want, created)
		}
	}
	for _, rel := range []string{
		"notes/projects/investing/stock-learning/charter/learning-charter.md",
		"notes/projects/investing/stock-learning/sources/source-index.md",
		"notes/projects/investing/stock-learning/retros/weekly-review.md",
	} {
		if !fileExists(filepath.Join(root, filepath.FromSlash(rel))) {
			t.Fatalf("learning starter note missing: %s", rel)
		}
	}
	createdAgent := runCLI(t, "project", "learning", "init", "history-learning", "history-info", "--title", "历史信息学习资料库", "--project-name", "历史信息学习", "--notes-prefix", "notes/history-learning", "--preset", "learning", "--vault", root, "--agent")
	for _, want := range []string{"mode=agent", "command=project.learning.init", "fact.project=history-learning", "fact.subproject=history-info", "fact.workspace_path=notes/projects/history-learning/history-info", "learning.project=history-learning", "learning.subproject=history-info", "learning.workspace_path=notes/projects/history-learning/history-info", "learning.column.1=inbox", "learning.starter_note.1=notes/projects/history-learning/history-info/charter/learning-charter.md", "learning.starter_item.1=notes/projects/history-learning/history-info/inbox/建立术语表.md", "action.board_show="} {
		if !strings.Contains(createdAgent, want) {
			t.Fatalf("learning init agent missing %q:\n%s", want, createdAgent)
		}
	}
	if strings.Contains(createdAgent, root) || strings.Contains(createdAgent, "状态:") || strings.Contains(createdAgent, "Learning project initialized") {
		t.Fatalf("learning init agent leaked local path or prose:\n%s", createdAgent)
	}

	board := runCLI(t, "project", "board", "show", "investing", "--subproject", "stock-learning", "--vault", root, "--json")
	for _, want := range []string{`"command":"project.board.show"`, `"column.learning":"1"`, `"column.planned":"1"`, `"column.retrospective":"1"`, `"建立交易术语表"`, `"整理 K 线与成交量基础"`} {
		if !strings.Contains(board, want) {
			t.Fatalf("learning board output missing %q:\n%s", want, board)
		}
	}

	second := runCLI(t, "project", "learning", "init", "investing", "stock-learning", "--title", "学习炒股的全部笔记", "--project-name", "学习炒股", "--notes-prefix", "notes/investing", "--preset", "stock-learning", "--vault", root, "--json")
	for _, want := range []string{`"command":"project.learning.init"`, `"notes.created":"0"`, `"items.created":"0"`} {
		if !strings.Contains(second, want) {
			t.Fatalf("second learning init output missing %q:\n%s", want, second)
		}
	}
}

func TestStorageSetS3RequiresBucketAndRegion(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault")
	out, err := runCLIExpectError("storage", "set-s3", "--bucket", "notes", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("storage set-s3 without region succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("storage error json invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "storage.set_s3" {
		t.Fatalf("storage error envelope = %#v", envelope)
	}
}

func TestVaultStatsDoctorAndDashboardCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "active.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_active\ntitle: Active\ntags: [pinax]\n---\n\n# Active\n\nbody\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "raw.md"), "# Raw\n\n")

	statsOut := runCLI(t, "stats", "--vault", root, "--json")
	var statsEnvelope map[string]any
	if err := json.Unmarshal([]byte(statsOut), &statsEnvelope); err != nil {
		t.Fatalf("stats json invalid: %v\n%s", err, statsOut)
	}
	if statsEnvelope["command"] != "vault.stats" || statsEnvelope["status"] != "success" || statsEnvelope["mode"] != "json" {
		t.Fatalf("stats envelope = %#v", statsEnvelope)
	}
	facts, ok := statsEnvelope["facts"].(map[string]any)
	if !ok || facts["notes"] != "1" || facts["index_status"] != "missing" {
		t.Fatalf("stats facts = %#v", statsEnvelope["facts"])
	}

	statsHuman := runCLI(t, "stats", "--vault", root)
	for _, want := range []string{"━━━━━━━━", "────────", "Highlights", "Vault statistics generated.", "Metric", "Value", "Notes", "1"} {
		if !strings.Contains(statsHuman, want) {
			t.Fatalf("stats human output missing %q:\n%s", want, statsHuman)
		}
	}
	for _, old := range []string{"状态:", "重点:", "事实:", "notes=2"} {
		if strings.Contains(statsHuman, old) {
			t.Fatalf("stats human output still uses label prose %q:\n%s", old, statsHuman)
		}
	}
	if strings.HasPrefix(strings.TrimSpace(statsHuman), "{") {
		t.Fatalf("stats human output looks like JSON:\n%s", statsHuman)
	}

	doctorJSON := runCLI(t, "doctor", "--vault", root, "--json")
	var doctorEnvelope map[string]any
	if err := json.Unmarshal([]byte(doctorJSON), &doctorEnvelope); err != nil {
		t.Fatalf("doctor json invalid: %v\n%s", err, doctorJSON)
	}
	if doctorEnvelope["command"] != "vault.doctor" || doctorEnvelope["status"] != "partial" || doctorEnvelope["mode"] != "json" {
		t.Fatalf("doctor envelope = %#v", doctorEnvelope)
	}

	doctorAgent := runCLI(t, "doctor", "--vault", root, "--agent")
	for _, want := range []string{"command=vault.doctor", "status=partial", "fact.issues.total=", "issue.1.code="} {
		if !strings.Contains(doctorAgent, want) {
			t.Fatalf("doctor agent output missing %q:\n%s", want, doctorAgent)
		}
	}
	if strings.Contains(doctorAgent, "状态:") || strings.Contains(doctorAgent, "重点:") {
		t.Fatalf("doctor agent output contains human prose:\n%s", doctorAgent)
	}

	dashboardOut, dashboardErr := runDashboardUntilCanceled(t, root)
	if dashboardOut != "" {
		t.Fatalf("dashboard wrote stdout: %q", dashboardOut)
	}
	if !strings.Contains(dashboardErr, "http://127.0.0.1:") {
		t.Fatalf("dashboard stderr missing URL:\n%s", dashboardErr)
	}

	help := runCLI(t, "--help")
	for _, want := range []string{"vault", "Markdown vault"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
	vaultHelp := runCLI(t, "vault", "--help")
	for _, want := range []string{"stats", "doctor", "dashboard", "ignore"} {
		if !strings.Contains(vaultHelp, want) {
			t.Fatalf("vault help missing %q:\n%s", want, vaultHelp)
		}
	}
}

func TestInitWithoutArgUsesVaultFlagDefault(t *testing.T) {
	root := t.TempDir()
	out := runCLIInDir(t, root, "init", "--title", "Vault")
	for _, want := range []string{"Highlights", "Pinax vault initialized.", "Metric", "Vault", "Next step", "pinax vault validate"} {
		if !strings.Contains(out, want) {
			t.Fatalf("init output missing %q:\n%s", want, out)
		}
	}
	for _, old := range []string{"状态:", "重点:", "推荐下一步:", "vault="} {
		if strings.Contains(out, old) {
			t.Fatalf("init output still uses label prose %q:\n%s", old, out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "config.yaml")); err != nil {
		t.Fatalf("init without arg did not create config in cwd: %v", err)
	}
	for _, rel := range []string{".pinaxignore", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("init without arg did not create %s: %v", rel, err)
		}
	}
	status := runCLIInDir(t, root, "vault", "ignore", "status", "--vault", root, "--json")
	for _, want := range []string{`"command":"vault.ignore.status"`, `"pinaxignore":"present"`, `"git_metadata_only":"present"`} {
		if !strings.Contains(status, want) {
			t.Fatalf("ignore status missing %q:\n%s", want, status)
		}
	}
}
