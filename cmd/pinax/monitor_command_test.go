package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMonitorCommandOutputContracts(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\nAlpha body secret-token raw prompt system prompt\n")
	runCLI(t, "search", "Alpha secret-token", "--engine", "native", "--vault", root, "--json")

	jsonOut := runCLI(t, "monitor", "runs", "--vault", root, "--command", "note.search", "--json")
	assertMachineOutputClean(t, jsonOut)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("monitor runs json invalid: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "monitor.runs" || envelope["status"] != "success" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
	if strings.Contains(jsonOut, "Alpha secret-token") || strings.Contains(jsonOut, "raw prompt") || strings.Contains(jsonOut, "system prompt") {
		t.Fatalf("monitor json leaked raw query/body: %s", jsonOut)
	}
	data := envelope["data"].(map[string]any)
	runs := data["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("runs = %#v", runs)
	}
	runID := runs[0].(map[string]any)["run_id"].(string)

	agentOut := runCLI(t, "monitor", "runs", "--vault", root, "--agent")
	assertMachineOutputClean(t, agentOut)
	for _, want := range []string{"command=monitor.runs", "status=success", "fact.runs=1", "fact.schema_version=pinax.monitor_run.v1"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("monitor agent missing %q:\n%s", want, agentOut)
		}
	}

	eventsOut := runCLI(t, "monitor", "tail", "--vault", root, "--events")
	assertMachineOutputClean(t, eventsOut)
	assertNDJSONEvents(t, eventsOut, "monitor.tail")

	showOut := runCLI(t, "monitor", "show", runID, "--vault", root, "--json")
	assertMachineOutputClean(t, showOut)
	if !strings.Contains(showOut, "monitor.show") || !strings.Contains(showOut, runID) {
		t.Fatalf("monitor show output = %s", showOut)
	}

	explainOut := runCLI(t, "monitor", "manage", "--vault", root, "--explain")
	if !strings.Contains(explainOut, "Conclusion:") || strings.Contains(explainOut, "secret-token") {
		t.Fatalf("monitor explain output = %s", explainOut)
	}
}

func TestMonitorCommandPartialWarningsOutput(t *testing.T) {
	root := t.TempDir()
	writeCLIFixture(t, filepath.Join(root, ".pinax", "monitor", "runs", "bad.json"), "{\n")

	jsonOut := runCLI(t, "monitor", "runs", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("monitor partial json invalid: %v\n%s", err, jsonOut)
	}
	if envelope["status"] != "partial" || envelope["facts"].(map[string]any)["warnings"] != "1" {
		t.Fatalf("monitor partial envelope = %#v", envelope)
	}
	humanOut := runCLI(t, "monitor", "runs", "--vault", root)
	for _, want := range []string{"Monitor warnings", "Source", "Path", "Message", "monitor_runs", ".pinax/monitor/runs/bad.json"} {
		if !strings.Contains(humanOut, want) {
			t.Fatalf("monitor partial human output missing %q:\n%s", want, humanOut)
		}
	}
	agentOut := runCLI(t, "monitor", "runs", "--vault", root, "--agent")
	for _, want := range []string{"command=monitor.runs", "status=partial", "fact.warnings=1", "warning.1.source=monitor_runs", "warning.1.path=.pinax/monitor/runs/bad.json"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("monitor partial agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func TestMonitorAndActivityShowCompletionCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\nAlpha body secret-token raw prompt system prompt\n")
	runCLI(t, "search", "Alpha secret-token", "--engine", "native", "--vault", root, "--json")

	monitorOut := runCLI(t, "monitor", "runs", "--vault", root, "--command", "note.search", "--json")
	var monitorEnvelope map[string]any
	if err := json.Unmarshal([]byte(monitorOut), &monitorEnvelope); err != nil {
		t.Fatalf("monitor runs json invalid: %v\n%s", err, monitorOut)
	}
	monitorRuns := monitorEnvelope["data"].(map[string]any)["runs"].([]any)
	if len(monitorRuns) != 1 {
		t.Fatalf("monitor runs = %#v", monitorRuns)
	}
	runID := monitorRuns[0].(map[string]any)["run_id"].(string)

	monitorCompletion := runCLI(t, "__complete", "monitor", "show", "--vault", root, "")
	assertCompletionContains(t, monitorCompletion, runID+"\tnote.search success", "ShellCompDirectiveNoFileComp")
	assertCompletionDoesNotContain(t, monitorCompletion, "secret-token", "raw prompt", "system prompt")

	activityOut := runCLI(t, "activity", "list", "--vault", root, "--source", "monitor_runs", "--json")
	var activityEnvelope map[string]any
	if err := json.Unmarshal([]byte(activityOut), &activityEnvelope); err != nil {
		t.Fatalf("activity list json invalid: %v\n%s", err, activityOut)
	}
	activityEntries := activityEnvelope["data"].(map[string]any)["entries"].([]any)
	if len(activityEntries) != 1 {
		t.Fatalf("activity entries = %#v", activityEntries)
	}
	eventID := activityEntries[0].(map[string]any)["event_id"].(string)

	activityCompletion := runCLI(t, "__complete", "activity", "show", "--vault", root, "")
	assertCompletionContains(t, activityCompletion, eventID+"\tmonitor_runs note.search success", "ShellCompDirectiveNoFileComp")
	assertCompletionDoesNotContain(t, activityCompletion, "secret-token", "raw prompt", "system prompt")
}
