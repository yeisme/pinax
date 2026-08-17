package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordIdentityAuditMachineOutputsAndReadOnlyBehavior(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCLIFixture(t, filepath.Join(root, "notes", "legacy.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_legacy\ntitle: Legacy\nkind: reference\n---\n\n# Legacy\n")

	jsonOut := runCLI(t, "record", "identity", "audit", "--vault", root, "--json")
	assertJSONCommandStatus(t, jsonOut, "record.identity.audit", "success")
	facts := jsonParseFacts(t, jsonOut)
	if facts["writes"] != "false" || facts["legacy"] != "1" {
		t.Fatalf("facts = %#v", facts)
	}
	agentOut := runCLI(t, "record", "identity", "audit", "--vault", root, "--agent")
	for _, want := range []string{"command=record.identity.audit", "status=success", "fact.writes=false", "fact.legacy=1"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax")); !os.IsNotExist(err) {
		t.Fatalf("audit wrote .pinax: %v", err)
	}
}

func TestIdentityMigrationPlanSaveReturnsSavedAsset(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCLIFixture(t, filepath.Join(root, "notes", "missing.md"), "---\nschema_version: pinax.note.v1\ntitle: Missing\nkind: reference\n---\n\n# Missing\n")

	out := runCLI(t, "record", "identity", "plan", "--save", "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "record.identity.plan", "success")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	savedPath := data["saved_path"].(string)
	if savedPath == "" {
		t.Fatalf("data = %#v", data)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(savedPath))); err != nil {
		t.Fatalf("saved plan missing: %v", err)
	}
}

func TestRecordIdentityHelpAndHumanSummary(t *testing.T) {
	t.Parallel()
	help := runCLI(t, "record", "identity", "--help")
	for _, want := range []string{"audit", "plan"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
	root := t.TempDir()
	writeCLIFixture(t, filepath.Join(root, "notes", "canonical.md"), "---\nschema_version: pinax.note.v1\nnote_id: 018f22e2-7b6d-7a3a-8db8-1f7ddf0c0101\ntitle: Canonical\nkind: reference\n---\n\n# Canonical\n")
	human := runCLI(t, "record", "identity", "audit", "--vault", root)
	if !strings.Contains(human, "Identity audit completed") || strings.Contains(human, "{\"") {
		t.Fatalf("human output = %s", human)
	}
}

func TestIdentityMigrationApplyCLIRequiresYesAndSupportsResume(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCLIFixture(t, filepath.Join(root, "notes", "legacy.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_legacy\ntitle: Legacy\nkind: reference\n---\n\n# Legacy\n")
	planned := runCLI(t, "record", "identity", "plan", "--save", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(planned), &envelope); err != nil {
		t.Fatal(err)
	}
	planID := envelope["data"].(map[string]any)["plan_id"].(string)
	failed, err := runCLIExpectError("record", "identity", "apply", "--plan", planID, "--vault", root, "--json")
	if err == nil {
		t.Fatal("apply without --yes succeeded")
	}
	assertJSONErrorCode(t, failed, "approval_required")
	applied := runCLI(t, "record", "identity", "apply", "--plan", planID, "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, applied, "record.identity.apply", "success")
	resumed := runCLI(t, "record", "identity", "apply", "--plan", planID, "--yes", "--resume", "--vault", root, "--agent")
	for _, want := range []string{"command=record.identity.apply", "status=success", "fact.status=completed"} {
		if !strings.Contains(resumed, want) {
			t.Fatalf("resume output missing %q:\n%s", want, resumed)
		}
	}
}
