package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCoreMVPCLIJSON(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "research", "--name", "研究", "--notes-prefix", "notes/research", "--vault", root, "--json")
	runCLI(t, "template", "init", "--vault", root, "--json")

	noteOut := runCLI(t, "note", "new", "研究日志", "--project", "research", "--tags", "pinax,sync", "--template", "mermaid", "--vault", root, "--json")
	var noteEnvelope map[string]any
	if err := json.Unmarshal([]byte(noteOut), &noteEnvelope); err != nil {
		t.Fatalf("note json invalid: %v\n%s", err, noteOut)
	}
	if noteEnvelope["command"] != "note.new" || noteEnvelope["status"] != "success" {
		t.Fatalf("note envelope = %#v", noteEnvelope)
	}

	indexOut := runCLI(t, "index", "rebuild", "--vault", root, "--agent")
	for _, want := range []string{"command=index.rebuild", "fact.notes=1"} {
		if !strings.Contains(indexOut, want) {
			t.Fatalf("index output missing %q:\n%s", want, indexOut)
		}
	}

	searchOut := runCLI(t, "search", "研究日志", "--vault", root, "--agent")
	for _, want := range []string{"command=note.search", "fact.matches=1", "fact.engine="} {
		if !strings.Contains(searchOut, want) {
			t.Fatalf("search output missing %q:\n%s", want, searchOut)
		}
	}

	syncOut := runCLI(t, "sync", "diff", "--target", "cloud", "--vault", root, "--json")
	var syncEnvelope map[string]any
	if err := json.Unmarshal([]byte(syncOut), &syncEnvelope); err != nil {
		t.Fatalf("sync json invalid: %v\n%s", err, syncOut)
	}
	if syncEnvelope["command"] != "sync.diff" || syncEnvelope["status"] != "partial" {
		t.Fatalf("sync envelope = %#v", syncEnvelope)
	}
	failed, err := runCLIExpectError("sync", "push", "--target", "cloud", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("sync push without approval got err=%v out=%s", err, failed)
	}
}
