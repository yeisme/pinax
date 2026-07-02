package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectDeleteTrashRestoreCLIContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "history", "--name", "History", "--notes-prefix", "notes/history", "--vault", root, "--json")

	refused, err := runCLIExpectError("project", "delete", "history", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("project delete without --yes succeeded:\n%s", refused)
	}
	assertJSONErrorCode(t, refused, "approval_required")
	if !strings.Contains(readCLIFile(t, filepath.Join(root, ".pinax", "projects.json")), "history") {
		t.Fatalf("delete without --yes modified project registry")
	}

	deleteOut := runCLI(t, "project", "delete", "history", "--vault", root, "--yes", "--json")
	var deleteEnvelope map[string]any
	if err := json.Unmarshal([]byte(deleteOut), &deleteEnvelope); err != nil {
		t.Fatalf("delete json invalid: %v\n%s", err, deleteOut)
	}
	if deleteEnvelope["command"] != "project.delete" || deleteEnvelope["status"] != "success" {
		t.Fatalf("delete envelope = %#v", deleteEnvelope)
	}
	deleteFacts := deleteEnvelope["facts"].(map[string]any)
	if deleteFacts["local_write"] != "true" || deleteFacts["remote_write"] != "false" || deleteFacts["trash_path"] == "" || deleteFacts["current_project"] != "" {
		t.Fatalf("delete facts = %#v", deleteFacts)
	}
	if strings.Contains(runCLI(t, "project", "list", "--vault", root, "--json"), "history") {
		t.Fatalf("deleted project still appears in project list")
	}

	trashJSON := runCLI(t, "trash", "list", "--vault", root, "--json")
	assertJSONCommandStatus(t, trashJSON, "trash.list", "success")
	if !strings.Contains(trashJSON, "project/history") || strings.Contains(trashJSON, "secret-token") {
		t.Fatalf("trash list missing tombstone or leaked data:\n%s", trashJSON)
	}
	trashAgent := runCLI(t, "trash", "list", "--vault", root, "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=trash.list", "status=success", "fact.entries=1"} {
		if !strings.Contains(trashAgent, want) {
			t.Fatalf("trash agent output missing %q:\n%s", want, trashAgent)
		}
	}

	showDeleted, err := runCLIExpectError("project", "show", "history", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("project show deleted succeeded:\n%s", showDeleted)
	}
	assertJSONErrorCode(t, showDeleted, "project_not_found")
	if !strings.Contains(showDeleted, "pinax trash restore project/history") {
		t.Fatalf("project show missing restore action:\n%s", showDeleted)
	}

	restoreOut := runCLI(t, "trash", "restore", "project/history", "--vault", root, "--json")
	assertJSONCommandStatus(t, restoreOut, "trash.restore", "success")
	if !strings.Contains(runCLI(t, "project", "list", "--vault", root, "--json"), "history") {
		t.Fatalf("restored project did not return to active list")
	}

	deleteOut = runCLI(t, "project", "delete", "history", "--vault", root, "--yes", "--json")
	deleteFacts = mustJSONFacts(t, deleteOut)
	dryRunOut := runCLI(t, "trash", "purge", "project/history", "--dry-run", "--vault", root, "--json")
	assertJSONCommandStatus(t, dryRunOut, "trash.purge", "success")
	trashPath := filepath.Join(root, filepath.FromSlash(deleteFacts["trash_path"].(string)))
	if _, err := os.Stat(trashPath); err != nil {
		t.Fatalf("purge dry-run removed trash path: %v", err)
	}
	purgeOut := runCLI(t, "trash", "purge", "project/history", "--hard", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, purgeOut, "trash.purge", "success")
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Fatalf("purge hard left trash path err=%v", err)
	}
}

func TestProjectSubprojectDeleteRequiresSnapshotAndRestoresWorkspace(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "history-learning", "--name", "History Learning", "--notes-prefix", "notes/history-learning", "--vault", root, "--json")
	runCLI(t, "project", "subproject", "create", "history-learning", "history-info", "--title", "History Info", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "projects", "history-learning", "history-info", "brief.md"), pinaxNoteFixture("note_history_info", "History Info", "[]", "workspace body"))

	refused, err := runCLIExpectError("project", "subproject", "delete", "history-learning", "history-info", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("subproject delete without --yes succeeded:\n%s", refused)
	}
	assertJSONErrorCode(t, refused, "approval_required")

	requireSnapshot, err := runCLIExpectError("project", "subproject", "delete", "history-learning", "history-info", "--vault", root, "--yes", "--json")
	if err == nil {
		t.Fatalf("non-empty subproject delete without snapshot succeeded:\n%s", requireSnapshot)
	}
	assertJSONErrorCode(t, requireSnapshot, "snapshot_required")
	if !strings.Contains(requireSnapshot, "pinax version snapshot") {
		t.Fatalf("snapshot_required missing next action:\n%s", requireSnapshot)
	}

	runCLI(t, "version", "snapshot", "--vault", root, "--message", "before subproject delete", "--json")
	deleteOut := runCLI(t, "project", "subproject", "delete", "history-learning", "history-info", "--vault", root, "--yes", "--json")
	assertJSONCommandStatus(t, deleteOut, "project.subproject.delete", "success")
	if strings.Contains(runCLI(t, "project", "subproject", "list", "history-learning", "--vault", root, "--json"), "history-info") {
		t.Fatalf("deleted subproject still appears in list")
	}
	boardDeleted, err := runCLIExpectError("project", "board", "show", "history-learning", "--subproject", "history-info", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("board show for deleted subproject succeeded:\n%s", boardDeleted)
	}
	assertJSONErrorCode(t, boardDeleted, "subproject_not_found")
	indexOut := runCLI(t, "index", "rebuild", "--vault", root, "--json")
	indexFacts := mustJSONFacts(t, indexOut)
	if indexFacts["notes"] != "0" {
		t.Fatalf("index rebuild counted trashed workspace notes: facts=%#v output=%s", indexFacts, indexOut)
	}
	searchOut := runCLI(t, "search", "workspace body", "--vault", root, "--json")
	searchFacts := mustJSONFacts(t, searchOut)
	if searchFacts["matches"] != "0" {
		t.Fatalf("search returned trashed workspace note: facts=%#v output=%s", searchFacts, searchOut)
	}
	if !strings.Contains(runCLI(t, "trash", "list", "--vault", root, "--json"), "subproject/history-learning/history-info") {
		t.Fatalf("trash list missing subproject tombstone")
	}
	restoreOut := runCLI(t, "trash", "restore", "subproject/history-learning/history-info", "--vault", root, "--json")
	assertJSONCommandStatus(t, restoreOut, "trash.restore", "success")
	if !strings.Contains(runCLI(t, "project", "subproject", "list", "history-learning", "--vault", root, "--json"), "history-info") {
		t.Fatalf("restored subproject did not return to active list")
	}
}

func TestProjectDeleteEdgeCasesAndOutputModes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "archive", "--name", "Archive", "--notes-prefix", "notes/archive", "--vault", root, "--json")
	runCLI(t, "project", "create", "history", "--name", "History", "--notes-prefix", "notes/history", "--vault", root, "--json")
	runCLI(t, "project", "switch", "history", "--vault", root, "--json")

	missing, err := runCLIExpectError("project", "delete", "missing", "--vault", root, "--yes", "--json")
	if err == nil {
		t.Fatalf("missing project delete succeeded:\n%s", missing)
	}
	assertJSONErrorCode(t, missing, "project_not_found")

	agentOut := runCLI(t, "project", "delete", "history", "--vault", root, "--yes", "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=project.delete", "status=success", "fact.project=history", "fact.current_project=archive", "fact.local_write=true", "fact.remote_write=false", "action.restore="} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("project delete agent output missing %q:\n%s", want, agentOut)
		}
	}

	listOut := runCLI(t, "project", "list", "--vault", root, "--json")
	if strings.Contains(listOut, "history") || !strings.Contains(listOut, "archive") {
		t.Fatalf("current project delete did not hide history or preserve archive:\n%s", listOut)
	}

	trashDefault := runCLI(t, "trash", "list", "--vault", root)
	if strings.HasPrefix(strings.TrimSpace(trashDefault), "{") || !strings.Contains(trashDefault, "Trash entries listed.") || !strings.Contains(trashDefault, "project/history") {
		t.Fatalf("trash default output invalid:\n%s", trashDefault)
	}
	restoreAgent := runCLI(t, "trash", "restore", "project/history", "--vault", root, "--agent")
	for _, want := range []string{"command=trash.restore", "status=success", "fact.object_id=project/history", "fact.local_write=true"} {
		if !strings.Contains(restoreAgent, want) {
			t.Fatalf("trash restore agent output missing %q:\n%s", want, restoreAgent)
		}
	}

	deleteDefault := runCLI(t, "project", "delete", "history", "--vault", root, "--yes")
	if strings.HasPrefix(strings.TrimSpace(deleteDefault), "{") || !strings.Contains(deleteDefault, "Project moved to trash.") || !strings.Contains(deleteDefault, "pinax trash restore project/history") {
		t.Fatalf("project delete default output invalid:\n%s", deleteDefault)
	}

	purgeAgent := runCLI(t, "trash", "purge", "project/history", "--dry-run", "--vault", root, "--agent")
	for _, want := range []string{"command=trash.purge", "status=success", "fact.dry_run=true", "fact.local_write=false"} {
		if !strings.Contains(purgeAgent, want) {
			t.Fatalf("trash purge agent output missing %q:\n%s", want, purgeAgent)
		}
	}
}

func mustJSONFacts(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, out)
	}
	facts, ok := envelope["facts"].(map[string]any)
	if !ok {
		t.Fatalf("facts missing: %#v", envelope)
	}
	return facts
}
