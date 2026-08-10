package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIPersonalBackupFacade(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "backup.md"), "# Backup\n\nlocal knowledge\n")

	help := runCLI(t, "backup", "--help")
	for _, want := range []string{"local version/Git snapshots", "status", "create", "history", "restore"} {
		if !strings.Contains(help, want) {
			t.Fatalf("backup help missing %q:\n%s", want, help)
		}
	}
	for _, absent := range []string{"S3", "rclone", "Capsa"} {
		if strings.Contains(help, absent) {
			t.Fatalf("backup help should not advertise remote transport %q:\n%s", absent, help)
		}
	}
	for _, absent := range []string{"--api-url", "--api-token", "--color", "--theme", "--width", "--markdown-style"} {
		if strings.Contains(help, absent) {
			t.Fatalf("backup help should hide advanced global flag %q:\n%s", absent, help)
		}
	}

	statusJSON := runCLI(t, "backup", "--vault", root, "--json")
	assertJSONCommandStatus(t, statusJSON, "backup.status", "success")
	if !strings.Contains(statusJSON, `"version_backend":"local"`) {
		t.Fatalf("backup status missing local backend facts:\n%s", statusJSON)
	}

	statusAgent := runCLI(t, "backup", "status", "--vault", root, "--agent")
	for _, want := range []string{"command=backup.status", "fact.version_backend=local", "action.snapshot="} {
		if !strings.Contains(statusAgent, want) {
			t.Fatalf("backup status agent missing %q:\n%s", want, statusAgent)
		}
	}
	if strings.Contains(statusAgent, "pinax version snapshot") || !strings.Contains(statusAgent, "pinax backup create") {
		t.Fatalf("backup status should recommend backup create:\n%s", statusAgent)
	}

	createJSON := runCLI(t, "backup", "create", "--message", "personal checkpoint", "--vault", root, "--json")
	var createEnvelope map[string]any
	if err := json.Unmarshal([]byte(createJSON), &createEnvelope); err != nil {
		t.Fatalf("backup create JSON invalid: %v\n%s", err, createJSON)
	}
	if createEnvelope["command"] != "backup.create" || createEnvelope["status"] != "success" {
		t.Fatalf("backup create envelope = %#v", createEnvelope)
	}
	snapshotID := createEnvelope["facts"].(map[string]any)["snapshot_id"]
	if snapshotID == nil || snapshotID == "" {
		t.Fatalf("backup create snapshot id missing: %#v", createEnvelope)
	}

	historyJSON := runCLI(t, "backup", "history", "--vault", root, "--json")
	assertJSONCommandStatus(t, historyJSON, "backup.history", "success")
	if !strings.Contains(historyJSON, snapshotID.(string)) {
		t.Fatalf("backup history missing snapshot %q:\n%s", snapshotID, historyJSON)
	}
	assertNDJSONEvents(t, runCLI(t, "backup", "history", "--vault", root, "--events"), "backup.history")

	missingMessage, err := runCLIExpectError("backup", "create", "--vault", root, "--json")
	if err == nil || !strings.Contains(missingMessage, `"command":"backup.create"`) || !strings.Contains(missingMessage, "backup create requires --message") {
		t.Fatalf("backup create missing message contract err=%v out=%s", err, missingMessage)
	}

	missingPlan, err := runCLIExpectError("backup", "restore", "notes/backup.md", "--vault", root, "--json")
	if err == nil || !strings.Contains(missingPlan, `"command":"backup.restore"`) || !strings.Contains(missingPlan, "--plan") {
		t.Fatalf("backup restore approval contract err=%v out=%s", err, missingPlan)
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "backup.md"), "# Backup\n\ncorrupted\n")
	planJSON := runCLI(t, "backup", "restore", "notes/backup.md", "--revision", snapshotID.(string), "--plan", "--vault", root, "--json")
	assertJSONCommandStatus(t, planJSON, "backup.restore", "success")
	planID := jsonParseFacts(t, planJSON)["plan_id"].(string)
	applyJSON := runCLI(t, "backup", "restore", "apply", "--plan", planID, "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, applyJSON, "backup.restore.apply", "success")
	applyFacts := jsonParseFacts(t, applyJSON)
	if applyFacts["local_write"] != "true" || applyFacts["remote_write"] != "false" {
		t.Fatalf("backup restore apply facts = %#v", applyFacts)
	}
	if restored := readCLIFile(t, filepath.Join(root, "notes", "backup.md")); !strings.Contains(restored, "local knowledge") || strings.Contains(restored, "corrupted") {
		t.Fatalf("backup restore did not recover local content:\n%s", restored)
	}

	versionJSON := runCLI(t, "version", "status", "--vault", root, "--json")
	assertJSONCommandStatus(t, versionJSON, "version.status", "success")
}
