package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoundedAgentContextProjectionCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "Research", "--notes-prefix", "notes/research", "--vault", root, "--json")
	body := "# Alpha Context\n\nSmall bounded snippet for the context card. [[Beta Context]]\n\n" + strings.Repeat("padding ", 80) + "SECRET_BODY_SENTINEL"
	alpha := runCLI(t, "note", "add", "Alpha Context", "--project", "research", "--kind", "task", "--status", "active", "--body", body, "--vault", root, "--json")
	if !strings.Contains(alpha, "note.create") {
		t.Fatalf("note add failed: %s", alpha)
	}
	runCLI(t, "note", "add", "Beta Context", "--body", "# Beta Context\n", "--vault", root, "--json")
	runCLI(t, "index", "refresh", "--vault", root, "--json")

	noteOut := runCLI(t, "note", "read", "Alpha Context", "--display", "card", "--vault", root, "--json")
	assertAgentContextEnvelope(t, noteOut, "note", "Alpha Context")
	if strings.Contains(noteOut, "SECRET_BODY_SENTINEL") {
		t.Fatalf("note card leaked body sentinel:\n%s", noteOut)
	}

	searchOut := runCLI(t, "search", "Alpha", "--vault", root, "--json")
	assertAgentContextsArray(t, searchOut, "note.search", "search_result")

	boardOut := runCLI(t, "project", "board", "show", "research", "--note-display", "card", "--vault", root, "--json")
	assertAgentContextsArray(t, boardOut, "project.board.show", "project_board_item")
	if strings.Contains(boardOut, "SECRET_BODY_SENTINEL") {
		t.Fatalf("project board card leaked body sentinel:\n%s", boardOut)
	}

	linksOut := runCLI(t, "note", "links", "Alpha Context", "--vault", root, "--json")
	assertAgentContextsArray(t, linksOut, "note.links", "graph_entity")

	_ = filepath.Separator
}

func assertAgentContextEnvelope(t *testing.T, out, sourceKind, title string) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	data, _ := envelope["data"].(map[string]any)
	note, _ := data["note"].(map[string]any)
	ctx, _ := note["agent_context"].(map[string]any)
	assertAgentContextShape(t, ctx, sourceKind, title)
}

func assertAgentContextsArray(t *testing.T, out, command, sourceKind string) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if envelope["command"] != command {
		t.Fatalf("command = %#v, want %s", envelope["command"], command)
	}
	data, _ := envelope["data"].(map[string]any)
	raw, _ := data["agent_contexts"].([]any)
	if len(raw) == 0 {
		t.Fatalf("agent_contexts missing from %s: %s", command, out)
	}
	ctx, _ := raw[0].(map[string]any)
	assertAgentContextShape(t, ctx, sourceKind, "")
}

func assertAgentContextShape(t *testing.T, ctx map[string]any, sourceKind, title string) {
	t.Helper()
	if ctx == nil {
		t.Fatalf("agent context missing")
	}
	for _, key := range []string{"context_id", "source_kind", "display_title", "refs", "snippets", "evidence", "body_exposure", "actions"} {
		if _, ok := ctx[key]; !ok {
			t.Fatalf("agent context missing %s: %#v", key, ctx)
		}
	}
	if ctx["source_kind"] != sourceKind {
		t.Fatalf("source_kind = %#v, want %s in %#v", ctx["source_kind"], sourceKind, ctx)
	}
	if title != "" && ctx["display_title"] != title {
		t.Fatalf("display_title = %#v, want %s", ctx["display_title"], title)
	}
}
