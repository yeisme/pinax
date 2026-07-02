package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestActivityCommandOutputContracts(t *testing.T) {
	root := t.TempDir()
	writeCLIFixture(t, filepath.Join(root, ".pinax", "events.jsonl"), `{"schema_version":"pinax.event.v1","type":"note.new","status":"success","ts":"2026-06-27T10:00:00Z","facts":{"path":"notes/alpha.md","Authorization":"Bearer secret-token"}}`+"\n")

	jsonOut := runCLI(t, "activity", "list", "--vault", root, "--query", "alpha", "--json")
	assertMachineOutputClean(t, jsonOut)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("activity list json invalid: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "activity.list" || envelope["status"] != "success" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	entries := data["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries = %#v", entries)
	}
	entry := entries[0].(map[string]any)
	if entry["source"] != "vault_events" || entry["kind"] != "note.new" {
		t.Fatalf("entry = %#v", entry)
	}
	if strings.Contains(jsonOut, "secret-token") || strings.Contains(jsonOut, "Authorization") {
		t.Fatalf("activity json leaked secret: %s", jsonOut)
	}

	agentOut := runCLI(t, "activity", "list", "--vault", root, "--agent")
	assertMachineOutputClean(t, agentOut)
	for _, want := range []string{"command=activity.list", "status=success", "fact.entries=1", "fact.schema_version=pinax.activity_event.v1"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("activity agent missing %q:\n%s", want, agentOut)
		}
	}

	eventsOut := runCLI(t, "activity", "tail", "--vault", root, "--events")
	assertMachineOutputClean(t, eventsOut)
	assertNDJSONEvents(t, eventsOut, "activity.tail")

	showOut := runCLI(t, "activity", "show", entry["event_id"].(string), "--vault", root, "--json")
	assertMachineOutputClean(t, showOut)
	if !strings.Contains(showOut, "activity.show") || !strings.Contains(showOut, entry["event_id"].(string)) {
		t.Fatalf("activity show output = %s", showOut)
	}

	explainOut := runCLI(t, "activity", "manage", "--vault", root, "--explain")
	if !strings.Contains(explainOut, "Conclusion:") || strings.Contains(explainOut, "secret-token") {
		t.Fatalf("activity explain output = %s", explainOut)
	}
}

func TestActivityCommandPartialOnCorruptOptionalSource(t *testing.T) {
	root := t.TempDir()
	writeCLIFixture(t, filepath.Join(root, ".pinax", "events.jsonl"), `{"schema_version":"pinax.event.v1","type":"index.refresh","status":"success","ts":"2026-06-27T10:00:00Z","facts":{"path":"notes/index.md"}}`+"\n{"+"\n")

	out := runCLI(t, "activity", "list", "--vault", root, "--json")
	assertMachineOutputClean(t, out)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("activity partial json invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "partial" {
		t.Fatalf("status = %#v, want partial\n%s", envelope["status"], out)
	}
	if facts := envelope["facts"].(map[string]any); facts["warnings"] != "1" {
		t.Fatalf("facts = %#v", facts)
	}
}
