package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryCaptureListRecallAndContext(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	source := filepath.Join(root, "docs", "operations", "release-packaging.md")
	writeCLIFixture(t, source, "# Release\n\nTag pushes trigger GitHub Actions releases.\n")

	captureOut := runCLI(t, "memory", "capture", "--type", "fact", "--subject", "pinax", "--predicate", "release_workflow", "--object", "tag push triggers GitHub Actions", "--source", "docs/operations/release-packaging.md", "--vault", root, "--json")
	var capture map[string]any
	if err := json.Unmarshal([]byte(captureOut), &capture); err != nil {
		t.Fatalf("memory capture json invalid: %v\n%s", err, captureOut)
	}
	if capture["command"] != "memory.capture" || capture["status"] != "success" {
		t.Fatalf("memory capture envelope = %#v", capture)
	}
	facts := capture["facts"].(map[string]any)
	if facts["type"] != "fact" || facts["status"] != "confirmed" || facts["source"] != "docs/operations/release-packaging.md" {
		t.Fatalf("memory capture facts = %#v", facts)
	}
	recordID := facts["record_id"].(string)
	if recordID == "" {
		t.Fatalf("memory capture missing record id: %#v", facts)
	}

	listOut := runCLI(t, "memory", "list", "--type", "fact", "--entity", "pinax", "--vault", root, "--json")
	assertJSONCommandStatus(t, listOut, "memory.list", "success")
	if !strings.Contains(listOut, recordID) || !strings.Contains(listOut, "release_workflow") {
		t.Fatalf("memory list missing captured record:\n%s", listOut)
	}

	recallOut := runCLI(t, "memory", "recall", "release workflow", "--entity", "pinax", "--vault", root, "--json")
	assertJSONCommandStatus(t, recallOut, "memory.recall", "success")
	if !strings.Contains(recallOut, "recall_reason") || !strings.Contains(recallOut, "entity_match:pinax") {
		t.Fatalf("memory recall missing explainable reason:\n%s", recallOut)
	}

	contextOut := runCLI(t, "memory", "context", "prepare next release", "--entity", "pinax", "--limit", "12", "--vault", root, "--agent")
	for _, want := range []string{"command=memory.context", "status=success", "fact.memory.matches=1", "fact.memory.types=fact", "fact.memory.scope="} {
		if !strings.Contains(contextOut, want) {
			t.Fatalf("memory context agent missing %q:\n%s", want, contextOut)
		}
	}
	if strings.Contains(contextOut, "Tag pushes trigger") || strings.Contains(contextOut, "raw prompt") || strings.Contains(contextOut, "Authorization") {
		t.Fatalf("memory context agent leaked body or sensitive text:\n%s", contextOut)
	}
}

func TestMemoryDryRunDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	dryRun := runCLI(t, "memory", "capture", "--type", "decision", "--subject", "pinax", "--object", "Use structured memory", "--dry-run", "--vault", root, "--json")
	assertJSONCommandStatus(t, dryRun, "memory.capture", "success")
	if fileExists(filepath.Join(root, ".pinax", "memory", "ledger.sqlite")) {
		t.Fatalf("memory dry-run wrote ledger database")
	}

	listOut := runCLI(t, "memory", "list", "--vault", root, "--json")
	assertJSONCommandStatus(t, listOut, "memory.list", "success")
	facts := jsonParseFacts(t, listOut)
	if facts["records"] != "0" {
		t.Fatalf("memory list after dry-run facts = %#v", facts)
	}
}

func TestMemoryRecallExcludesDraftSupersededExpiredAndRejected(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	for _, status := range []string{"draft", "confirmed", "superseded", "expired", "rejected"} {
		runCLI(t, "memory", "capture", "--type", "fact", "--subject", "pinax", "--predicate", "release_workflow", "--object", status+" release memory", "--status", status, "--vault", root, "--json")
	}

	recallOut := runCLI(t, "memory", "recall", "release memory", "--entity", "pinax", "--vault", root, "--json")
	assertJSONCommandStatus(t, recallOut, "memory.recall", "success")
	if !strings.Contains(recallOut, "confirmed release memory") {
		t.Fatalf("memory recall missing confirmed record:\n%s", recallOut)
	}
	for _, forbidden := range []string{"draft release memory", "superseded release memory", "expired release memory", "rejected release memory"} {
		if strings.Contains(recallOut, forbidden) {
			t.Fatalf("memory recall included %q by default:\n%s", forbidden, recallOut)
		}
	}

	allOut := runCLI(t, "memory", "list", "--include-superseded", "--include-draft", "--include-expired", "--include-rejected", "--vault", root, "--json")
	for _, want := range []string{"draft release memory", "superseded release memory", "expired release memory", "rejected release memory"} {
		if !strings.Contains(allOut, want) {
			t.Fatalf("memory list with include flags missing %q:\n%s", want, allOut)
		}
	}
}

func TestMemoryRejectsInvalidRecord(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("memory", "capture", "--type", "unknown", "--subject", "pinax", "--object", "bad", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("memory capture should reject unknown type:\n%s", out)
	}
	assertJSONErrorCode(t, out, "memory_record_invalid")
}
