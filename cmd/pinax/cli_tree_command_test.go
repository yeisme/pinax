package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceBackedCommandTreeGapsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "Research", "--notes-prefix", "notes/research", "--vault", root, "--json")

	projectShow := runCLI(t, "project", "show", "research", "--vault", root, "--json")
	assertJSONCommandStatus(t, projectShow, "project.show", "success")
	if !strings.Contains(projectShow, "notes/research") {
		t.Fatalf("project show missing project details:\n%s", projectShow)
	}

	itemOut := runCLI(t, "project", "item", "add", "research", "Review CLI tree", "--column", "next", "--vault", root, "--json")
	var itemEnvelope map[string]any
	if err := json.Unmarshal([]byte(itemOut), &itemEnvelope); err != nil {
		t.Fatalf("project item add json invalid: %v\n%s", err, itemOut)
	}
	itemID := itemEnvelope["facts"].(map[string]any)["item_id"].(string)

	itemShow := runCLI(t, "project", "item", "show", itemID, "--vault", root, "--json")
	assertJSONCommandStatus(t, itemShow, "project.item.show", "success")
	if !strings.Contains(itemShow, "Review CLI tree") {
		t.Fatalf("project item show missing title:\n%s", itemShow)
	}

	itemPlan := runCLI(t, "project", "item", "plan", itemID, "--action", "move", "--column", "doing", "--vault", root, "--json")
	assertJSONCommandStatus(t, itemPlan, "project.item.plan", "success")
	if !strings.Contains(itemPlan, "\"action\":\"move\"") || !strings.Contains(itemPlan, "\"writes\":\"false\"") {
		t.Fatalf("project item plan output invalid:\n%s", itemPlan)
	}

	runCLI(t, "repair", "plan", "--save", "--vault", root, "--json")
	repairList := runCLI(t, "repair", "list", "--vault", root, "--json")
	assertJSONCommandStatus(t, repairList, "repair.list", "success")
	if !strings.Contains(repairList, "\"plans\":\"1\"") {
		t.Fatalf("repair list missing saved plan:\n%s", repairList)
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n[[Missing]]\n")
	graphSummary := runCLI(t, "graph", "summary", "--vault", root, "--json")
	assertJSONCommandStatus(t, graphSummary, "graph.summary", "partial")
	if !strings.Contains(graphSummary, "\"broken\":\"1\"") {
		t.Fatalf("graph summary missing broken link count:\n%s", graphSummary)
	}
}
