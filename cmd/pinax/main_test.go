package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVersionCommand(t *testing.T) {
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "pinax dev") {
		t.Fatalf("version output = %q", got)
	}
}

func TestGitSnapshotHiddenCompatibilityAliasCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	help := runCLI(t, "--help")
	if strings.Contains(help, "git") || !strings.Contains(help, "version") {
		t.Fatalf("root help should show version and hide git:\n%s", help)
	}

	out := runCLI(t, "git", "snapshot", "--vault", root, "--message", "compat", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("git snapshot alias json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "version.snapshot" || facts["version_backend"] != "local" || facts["snapshot_id"] == "" {
		t.Fatalf("git snapshot alias envelope = %#v", envelope)
	}
}

func TestAPIServeMachineModesAreQuietAndWriteModeConflictIsStable(t *testing.T) {
	root := t.TempDir()
	for _, mode := range []string{"--json", "--agent"} {
		stdout, stderr, err := runAPIServeUntilCanceled(t, root, "api", "serve", "--port", "0", "--vault", root, mode)
		if err == nil || stderr != "" {
			t.Fatalf("api serve %s should fail without diagnostics on stderr: err=%v stderr=%q stdout=%s", mode, err, stderr, stdout)
		}
		if !strings.Contains(stdout, "unsupported_output_mode") || strings.Contains(stdout, "Pinax local API") || strings.Contains(stdout, "http://127.0.0.1:") {
			t.Fatalf("api serve %s stdout contract violated: %s", mode, stdout)
		}
	}
	stdout, stderr, err := runCLISeparate("api", "serve", "--readonly", "--allow-write", "--vault", root, "--json")
	if err == nil || stderr != "" || !strings.Contains(stdout, "write_mode_conflict") {
		t.Fatalf("api serve write mode conflict err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
}

func TestAPIServeLifecycleOutput(t *testing.T) {
	root := t.TempDir()
	stdout, stderr, err := runAPIServeUntilCanceled(t, root, "api", "serve", "--port", "0", "--vault", root)
	if err != nil || stdout != "" {
		t.Fatalf("api serve default err=%v stdout=%q stderr=%s", err, stdout, stderr)
	}
	if !strings.Contains(stderr, "pinax api ready") || !strings.Contains(stderr, "http://127.0.0.1:") || !strings.Contains(stderr, "auth_mode") {
		t.Fatalf("api serve default stderr missing zap startup log: %s", stderr)
	}

	stdout, stderr, err = runAPIServeUntilCanceled(t, root, "api", "serve", "--readonly", "--port", "0", "--vault", root, "--events")
	if err != nil || stderr != "" {
		t.Fatalf("api serve events err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	events := parseNDJSONEvents(t, stdout)
	for _, want := range []string{"start", "ready", "shutdown"} {
		if !hasEventType(events, want) {
			t.Fatalf("api serve events missing %s: %#v\n%s", want, events, stdout)
		}
	}
	for _, event := range events {
		if event["type"] == "ready" && !strings.Contains(fmt.Sprint(event["url"]), "http://127.0.0.1:") {
			t.Fatalf("ready event missing localhost URL: %#v", event)
		}
		if strings.Contains(fmt.Sprint(event["message"]), "Temp token:") {
			t.Fatalf("events leaked temp token log: %#v", event)
		}
	}
}

func TestNoteTagUnsafeValuesRejectedCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	unsafeNew, err := runCLIExpectError("note", "new", "Unsafe", "--tags", "bad]", "--vault", root, "--json")
	if err == nil || !strings.Contains(unsafeNew, `"code":"invalid_tag"`) {
		t.Fatalf("note new unsafe tag should fail with invalid_tag: err=%v out=%s", err, unsafeNew)
	}
	if fileExists(filepath.Join(root, "Unsafe.md")) || fileExists(filepath.Join(root, ".pinax", "index.sqlite")) || fileExists(filepath.Join(root, ".pinax", "records", "ledger.jsonl")) {
		t.Fatalf("unsafe note new wrote vault assets")
	}

	safeOut := runCLI(t, "note", "new", "Safe", "--tags", "safe", "--vault", root, "--json")
	var safeEnvelope map[string]any
	if err := json.Unmarshal([]byte(safeOut), &safeEnvelope); err != nil {
		t.Fatalf("safe note json invalid: %v\n%s", err, safeOut)
	}
	path := filepath.Join(root, safeEnvelope["facts"].(map[string]any)["path"].(string))
	before := readCLIFile(t, path)
	unsafeTag, err := runCLIExpectError("note", "tag", "add", "Safe", "bad]", "--vault", root, "--json")
	if err == nil || !strings.Contains(unsafeTag, `"code":"invalid_tag"`) {
		t.Fatalf("note tag unsafe value should fail with invalid_tag: err=%v out=%s", err, unsafeTag)
	}
	if got := readCLIFile(t, path); got != before {
		t.Fatalf("unsafe note tag changed frontmatter:\n%s", got)
	}
}

func TestNoteTagRecordFactsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	created := runCLI(t, "note", "new", "Taggable", "--tags", "safe", "--vault", root, "--json")
	var createdEnvelope map[string]any
	if err := json.Unmarshal([]byte(created), &createdEnvelope); err != nil {
		t.Fatalf("created note json invalid: %v\n%s", err, created)
	}
	path := createdEnvelope["facts"].(map[string]any)["path"].(string)

	stdout, stderr, err := runCLISeparate("note", "tag", "add", path, "research", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("note tag json err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("note tag json invalid: %v\n%s", err, stdout)
	}
	facts := envelope["facts"].(map[string]any)
	for key, want := range map[string]string{"record_event": "note.metadata_updated", "ledger_seq": "2", "record_version": "2", "index_updated": "true"} {
		if facts[key] != want {
			t.Fatalf("fact %s = %#v, want %q; envelope=%#v", key, facts[key], want, envelope)
		}
	}

	agentOut := runCLI(t, "note", "tag", "add", path, "cli", "--vault", root, "--agent")
	for _, want := range []string{"command=note.tag", "fact.record_event=note.metadata_updated", "fact.ledger_seq=3", "fact.index_updated=true"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func parseNDJSONEvents(t *testing.T, out string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	events := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event is not JSON: %v\n%s", err, line)
		}
		events = append(events, event)
	}
	return events
}

func hasEventType(events []map[string]any, want string) bool {
	for _, event := range events {
		if event["type"] == want {
			return true
		}
	}
	return false
}

func TestVersionWorkflowContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "version.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_version\ntitle: Version\n---\n\n# Version\n")

	human := runCLI(t, "version", "status", "--vault", root)
	for _, want := range []string{"Version backend", "local", "pinax version snapshot"} {
		if !strings.Contains(human, want) {
			t.Fatalf("version status human missing %q:\n%s", want, human)
		}
	}

	statusOut := runCLI(t, "version", "status", "--vault", root, "--json")
	var statusEnvelope map[string]any
	if err := json.Unmarshal([]byte(statusOut), &statusEnvelope); err != nil {
		t.Fatalf("version status json invalid: %v\n%s", err, statusOut)
	}
	statusFacts := statusEnvelope["facts"].(map[string]any)
	for key, want := range map[string]string{"version_backend": "local", "snapshot_supported": "true", "changed_paths_supported": "false", "read_at_revision_supported": "false"} {
		if statusFacts[key] != want {
			t.Fatalf("version status fact %s=%#v want %q envelope=%#v", key, statusFacts[key], want, statusEnvelope)
		}
	}
	if statusEnvelope["command"] != "version.status" || statusEnvelope["status"] != "success" {
		t.Fatalf("version status envelope = %#v", statusEnvelope)
	}

	backendsAgent := runCLI(t, "version", "backends", "--vault", root, "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=version.backends", "status=success", "fact.backends=2", "fact.active_backend=local", "fact.backend.1.name=local", "fact.backend.2.name=none"} {
		if !strings.Contains(backendsAgent, want) {
			t.Fatalf("version backends agent missing %q:\n%s", want, backendsAgent)
		}
	}

	snapshotOut := runCLI(t, "version", "snapshot", "--vault", root, "--message", "checkpoint", "--json")
	var snapshotEnvelope map[string]any
	if err := json.Unmarshal([]byte(snapshotOut), &snapshotEnvelope); err != nil {
		t.Fatalf("version snapshot json invalid: %v\n%s", err, snapshotOut)
	}
	snapshotFacts := snapshotEnvelope["facts"].(map[string]any)
	if snapshotEnvelope["command"] != "version.snapshot" || snapshotFacts["snapshot_id"] == "" || snapshotFacts["version_backend"] != "local" || snapshotFacts["files"] == "" {
		t.Fatalf("version snapshot envelope = %#v", snapshotEnvelope)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "version", "snapshots", snapshotFacts["snapshot_id"].(string)+".json")); err != nil {
		t.Fatalf("snapshot evidence missing: %v", err)
	}

	statusAgent := runCLI(t, "version", "status", "--vault", root, "--agent")
	for _, want := range []string{"command=version.status", "fact.version_backend=local", "fact.last_snapshot_id="} {
		if !strings.Contains(statusAgent, want) {
			t.Fatalf("version status agent missing %q:\n%s", want, statusAgent)
		}
	}
}
func TestVersionExtendedCommandsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "version.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_version\ntitle: Version\n---\n\n# Version\n")

	snapshotOut := runCLI(t, "version", "snapshot", "--vault", root, "--message", "history checkpoint", "--json")
	var snapshotEnvelope map[string]any
	if err := json.Unmarshal([]byte(snapshotOut), &snapshotEnvelope); err != nil {
		t.Fatalf("version snapshot json invalid: %v\n%s", err, snapshotOut)
	}
	snapshotID := snapshotEnvelope["facts"].(map[string]any)["snapshot_id"].(string)

	help := runCLI(t, "version", "--help")
	for _, want := range []string{"history", "diff", "show", "restore", "changed"} {
		if !strings.Contains(help, want) {
			t.Fatalf("version help missing %q:\n%s", want, help)
		}
	}

	historyOut := runCLI(t, "version", "history", "--vault", root, "--json")
	var historyEnvelope map[string]any
	if err := json.Unmarshal([]byte(historyOut), &historyEnvelope); err != nil {
		t.Fatalf("version history json invalid: %v\n%s", err, historyOut)
	}
	historyFacts := historyEnvelope["facts"].(map[string]any)
	if historyEnvelope["command"] != "version.history" || historyEnvelope["status"] != "success" || historyFacts["snapshots"] != "1" || !strings.Contains(historyOut, snapshotID) {
		t.Fatalf("version history envelope = %#v\n%s", historyEnvelope, historyOut)
	}

	assertVersionError := func(args []string, wantCommand, wantCode string) {
		t.Helper()
		out, err := runCLIExpectError(args...)
		if err == nil {
			t.Fatalf("pinax %v succeeded unexpectedly:\n%s", args, out)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("pinax %v json invalid: %v\n%s", args, err, out)
		}
		if envelope["command"] != wantCommand || envelope["status"] != "failed" {
			t.Fatalf("pinax %v envelope = %#v", args, envelope)
		}
		errorObject := envelope["error"].(map[string]any)
		if errorObject["code"] != wantCode {
			t.Fatalf("pinax %v error code=%#v want %q envelope=%#v", args, errorObject["code"], wantCode, envelope)
		}
	}

	assertVersionError([]string{"version", "changed", "--since", "rev_0", "--vault", root, "--json"}, "version.changed", "version_changed_paths_unavailable")
	assertVersionError([]string{"version", "show", "notes/version.md", "--revision", "rev_0", "--vault", root, "--json"}, "version.show", "version_read_unavailable")
	assertVersionError([]string{"version", "diff", "--base", "rev_0", "--target", "rev_1", "--vault", root, "--json"}, "version.diff", "version_read_unavailable")
	assertVersionError([]string{"version", "restore", "notes/version.md", "--revision", "rev_0", "--plan", "--vault", root, "--json"}, "version.restore", "version_read_unavailable")
}

func TestAssetLinkCommandCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	notePath := filepath.Join(root, "notes", "alpha.md")
	writeCLIFixture(t, notePath, "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\nbody before link\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	before, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read note before link: %v", err)
	}

	help := runCLI(t, "asset", "--help")
	if !strings.Contains(help, "link") {
		t.Fatalf("asset help missing link command:\n%s", help)
	}

	out := runCLI(t, "asset", "link", "diagram", "--note", "Alpha", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("asset link json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "asset.link" || envelope["status"] != "partial" || facts["writes"] != "false" || facts["asset_path"] != "assets/diagram.png" || facts["note_path"] != "notes/alpha.md" {
		t.Fatalf("asset link envelope = %#v\n%s", envelope, out)
	}
	if !strings.Contains(out, "asset_link") || !strings.Contains(out, "requires_snapshot") {
		t.Fatalf("asset link output missing plan details:\n%s", out)
	}
	after, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read note after link: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("asset link command modified note body:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestAssetRelationshipCommandsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeCLIFixture(t, filepath.Join(root, "assets", "orphan.pdf"), "pdf")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n![Missing](../assets/missing.png)\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	backlinksOut := runCLI(t, "asset", "backlinks", "diagram", "--vault", root, "--json")
	var backlinks map[string]any
	if err := json.Unmarshal([]byte(backlinksOut), &backlinks); err != nil {
		t.Fatalf("asset backlinks json invalid: %v\n%s", err, backlinksOut)
	}
	backlinkFacts := backlinks["facts"].(map[string]any)
	if backlinks["command"] != "asset.backlinks" || backlinkFacts["linked_notes"] != "1" || !strings.Contains(backlinksOut, "notes/alpha.md") || !strings.Contains(backlinksOut, "![Diagram](../assets/diagram.png)") {
		t.Fatalf("asset backlinks output = %#v\n%s", backlinks, backlinksOut)
	}

	orphansOut := runCLI(t, "asset", "orphans", "--vault", root, "--json")
	var orphans map[string]any
	if err := json.Unmarshal([]byte(orphansOut), &orphans); err != nil {
		t.Fatalf("asset orphans json invalid: %v\n%s", err, orphansOut)
	}
	orphanFacts := orphans["facts"].(map[string]any)
	if orphans["command"] != "asset.orphans" || orphanFacts["orphan"] != "1" || !strings.Contains(orphansOut, "assets/orphan.pdf") {
		t.Fatalf("asset orphans output = %#v\n%s", orphans, orphansOut)
	}

	missingOut := runCLI(t, "asset", "missing", "--vault", root, "--json")
	var missing map[string]any
	if err := json.Unmarshal([]byte(missingOut), &missing); err != nil {
		t.Fatalf("asset missing json invalid: %v\n%s", err, missingOut)
	}
	missingFacts := missing["facts"].(map[string]any)
	if missing["command"] != "asset.missing" || missingFacts["missing"] != "1" || !strings.Contains(missingOut, "assets/missing.png") {
		t.Fatalf("asset missing output = %#v\n%s", missing, missingOut)
	}

	repairOut := runCLI(t, "asset", "repair", "--plan", "--vault", root, "--json")
	var repair map[string]any
	if err := json.Unmarshal([]byte(repairOut), &repair); err != nil {
		t.Fatalf("asset repair json invalid: %v\n%s", err, repairOut)
	}
	repairFacts := repair["facts"].(map[string]any)
	if repair["command"] != "asset.repair" || repairFacts["writes"] != "false" || repairFacts["missing"] != "1" || repairFacts["orphan"] != "1" || !strings.Contains(repairOut, "asset_missing") || !strings.Contains(repairOut, "asset_orphan") {
		t.Fatalf("asset repair output = %#v\n%s", repair, repairOut)
	}
}
func TestAssetMoveRemovePlanCommandsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\n---\n\n# Beta\n\n![[../assets/diagram.png]]\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	moveOut := runCLI(t, "asset", "move", "diagram", "attachments/archive/diagram.png", "--plan", "--vault", root, "--json")
	var move map[string]any
	if err := json.Unmarshal([]byte(moveOut), &move); err != nil {
		t.Fatalf("asset move json invalid: %v\n%s", err, moveOut)
	}
	moveFacts := move["facts"].(map[string]any)
	if move["command"] != "asset.move" || moveFacts["writes"] != "false" || moveFacts["linked_notes"] != "2" || moveFacts["requires_snapshot"] != "true" || !strings.Contains(moveOut, "asset_move") || !strings.Contains(moveOut, "asset_reference_rewrite") || !strings.Contains(moveOut, "![Diagram](../assets/diagram.png)") || !strings.Contains(moveOut, "![[../assets/diagram.png]]") || !strings.Contains(moveOut, "pinax asset move diagram attachments/archive/diagram.png --vault") {
		t.Fatalf("asset move plan output = %#v\n%s", move, moveOut)
	}
	if _, err := os.Stat(filepath.Join(root, "assets", "diagram.png")); err != nil {
		t.Fatalf("asset move --plan should not move source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "attachments", "archive", "diagram.png")); !os.IsNotExist(err) {
		t.Fatalf("asset move --plan unexpectedly created target: %v", err)
	}

	removeOut := runCLI(t, "asset", "remove", "diagram", "--plan", "--vault", root, "--json")
	var remove map[string]any
	if err := json.Unmarshal([]byte(removeOut), &remove); err != nil {
		t.Fatalf("asset remove json invalid: %v\n%s", err, removeOut)
	}
	removeFacts := remove["facts"].(map[string]any)
	if remove["command"] != "asset.remove" || removeFacts["writes"] != "false" || removeFacts["shared"] != "true" || removeFacts["delete_allowed"] != "false" || removeFacts["requires_snapshot"] != "true" || strings.Contains(removeOut, "asset_delete") || !strings.Contains(removeOut, "asset_reference_review") || !strings.Contains(removeOut, "pinax asset remove diagram --vault") {
		t.Fatalf("asset remove plan output = %#v\n%s", remove, removeOut)
	}
	if _, err := os.Stat(filepath.Join(root, "assets", "diagram.png")); err != nil {
		t.Fatalf("asset remove --plan should not delete source: %v", err)
	}
}

func TestAssetApplyLikeCommandsRequirePlanCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	assetPath := filepath.Join(root, "assets", "diagram.png")
	writeCLIFixture(t, assetPath, "png")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	before := readCLIFile(t, assetPath)

	checks := [][]string{
		{"asset", "move", "diagram", "attachments/archive/diagram.png", "--vault", root, "--json"},
		{"asset", "remove", "diagram", "--vault", root, "--json"},
		{"asset", "repair", "--vault", root, "--json"},
	}
	for _, args := range checks {
		out, err := runCLIExpectError(args...)
		if err == nil || !strings.Contains(out, "approval_required") || !strings.Contains(out, "--plan") {
			t.Fatalf("%v error = %v output = %s", args, err, out)
		}
	}
	if got := readCLIFile(t, assetPath); got != before {
		t.Fatalf("asset command without plan modified asset: %q", got)
	}
	if fileExists(filepath.Join(root, "attachments", "archive", "diagram.png")) {
		t.Fatalf("asset move without plan created target")
	}
}

func TestAssetPreviewOutputContractCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	specSource := filepath.Join(root, "spec-source.md")
	writeCLIFixture(t, specSource, "# Spec\n\nAsset preview body")
	textSource := filepath.Join(root, "transcript-source.txt")
	writeCLIFixture(t, textSource, "abcdef")
	imageSource := filepath.Join(root, "diagram-source.png")
	writeCLIFixture(t, imageSource, "png-bytes")
	runCLI(t, "asset", "add", specSource, "--vault", root, "--json")
	runCLI(t, "asset", "add", textSource, "--vault", root, "--json")
	runCLI(t, "asset", "add", imageSource, "--vault", root, "--json")

	markdownOut := runCLI(t, "asset", "preview", "spec-source", "--as", "markdown", "--vault", root, "--json")
	if !strings.Contains(markdownOut, "\"command\":\"asset.preview\"") || !strings.Contains(markdownOut, "Asset preview body") || !strings.Contains(markdownOut, "embedded_asset") {
		t.Fatalf("asset markdown preview:\n%s", markdownOut)
	}
	textOut := runCLI(t, "asset", "preview", "transcript-source", "--as", "text", "--max-preview-bytes", "3", "--vault", root, "--json")
	if !strings.Contains(textOut, "abc") || strings.Contains(textOut, "def") || !strings.Contains(textOut, "\"truncated\":true") {
		t.Fatalf("asset text preview:\n%s", textOut)
	}
	imageOut := runCLI(t, "asset", "preview", "diagram-source", "--as", "markdown", "--vault", root, "--json")
	if !strings.Contains(imageOut, "image/png") || !strings.Contains(imageOut, "placeholder") || strings.Contains(imageOut, "png-bytes") {
		t.Fatalf("asset image preview:\n%s", imageOut)
	}
}

func TestRenderedNoteAttachmentPreviewCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "spec.md"), "# Spec\n\nInline spec body")
	writeCLIFixture(t, filepath.Join(root, "notes", "transcript.txt"), "abcdef")
	writeCLIFixture(t, filepath.Join(root, "notes", "diagram.png"), "png-bytes")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![[spec.md]]\n![[transcript.txt]]\n![[diagram.png]]\n")

	renderedOut := runCLI(t, "note", "show", "Alpha", "--view", "rendered", "--embed-attachments", "markdown", "--vault", root, "--json")
	if !strings.Contains(renderedOut, "Inline spec body") || !strings.Contains(renderedOut, "pinax asset show diagram.png") || !strings.Contains(renderedOut, "embedded_assets") {
		t.Fatalf("rendered attachment preview:\n%s", renderedOut)
	}
	sourceOut := runCLI(t, "note", "show", "Alpha", "--view", "source", "--embed-attachments", "markdown", "--vault", root, "--json")
	if strings.Contains(sourceOut, "Inline spec body") || strings.Contains(sourceOut, "embedded_assets") {
		t.Fatalf("source view should not inline attachments:\n%s", sourceOut)
	}
	textOut := runCLI(t, "note", "show", "Alpha", "--view", "rendered", "--embed-attachments", "text", "--max-embed-bytes", "3", "--vault", root, "--json")
	if !strings.Contains(textOut, "truncated") {
		t.Fatalf("text attachment bounded preview:\n%s", textOut)
	}
	previewOut := runCLI(t, "note", "preview", "Alpha", "--embed-attachments", "markdown", "--vault", root, "--json")
	if !strings.Contains(previewOut, "\"command\":\"note.preview\"") || !strings.Contains(previewOut, "Inline spec body") {
		t.Fatalf("note preview output:\n%s", previewOut)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "render-runs")); !os.IsNotExist(err) {
		t.Fatalf("note preview should not write render run receipts: %v", err)
	}
}

func TestAttachmentPathStyleCommandsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n")
	writeCLIFixture(t, filepath.Join(root, "attachments", "other", "diagram.png"), "other")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	attachmentsOut := runCLI(t, "note", "attachments", "Alpha", "--path-style", "note-relative", "--vault", root, "--json")
	if !strings.Contains(attachmentsOut, "\"path\":\"assets/diagram.png\"") || !strings.Contains(attachmentsOut, "\"display_path\":\"../assets/diagram.png\"") {
		t.Fatalf("note attachments path style output:\n%s", attachmentsOut)
	}

	missingContext, _, err := runCLISeparate("asset", "show", "diagram", "--path-style", "note-relative", "--vault", root, "--json")
	if err == nil || !strings.Contains(missingContext, "path_context_required") || !strings.Contains(missingContext, "--context-note") {
		t.Fatalf("missing context stdout=%s err=%v", missingContext, err)
	}
	markdownOut := runCLI(t, "asset", "show", "diagram", "--path-style", "markdown", "--context-note", "Alpha", "--vault", root, "--json")
	if !strings.Contains(markdownOut, "\"display_path\":\"![diagram.png](../assets/diagram.png)\"") {
		t.Fatalf("markdown path style output:\n%s", markdownOut)
	}
	wikiOut := runCLI(t, "asset", "show", "diagram", "--path-style", "wiki", "--vault", root, "--json")
	if !strings.Contains(wikiOut, "\"display_path\":\"![[assets/diagram.png]]\"") || strings.Contains(wikiOut, "![[diagram.png]]") {
		t.Fatalf("wiki path style output:\n%s", wikiOut)
	}
	absoluteOut := runCLI(t, "asset", "show", "diagram", "--path-style", "absolute", "--vault", root, "--json")
	if !strings.Contains(absoluteOut, filepath.ToSlash(filepath.Join(root, "assets", "diagram.png"))) || strings.Contains(absoluteOut, filepath.ToSlash(filepath.Join(root, "notes", "alpha.md"))) {
		t.Fatalf("absolute path style output:\n%s", absoluteOut)
	}
}

func TestAssetCompletionUsesIndexedAttachmentCandidatesCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "assets", "diagram.png"), "png")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\n---\n\n# Alpha\n\n![Diagram](../assets/diagram.png)\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	showCompletion := runCLI(t, "__complete", "asset", "show", "--vault", root, "")
	for _, want := range []string{"diagram.png\timage/png linked_notes=1", "assets/diagram.png\timage/png linked_notes=1", "diagram\timage/png linked_notes=1", "ShellCompDirectiveNoFileComp"} {
		if !strings.Contains(showCompletion, want) {
			t.Fatalf("asset show completion missing %q:\n%s", want, showCompletion)
		}
	}

	backlinksCompletion := runCLI(t, "__complete", "asset", "backlinks", "--vault", root, "dia")
	if !strings.Contains(backlinksCompletion, "diagram.png\timage/png linked_notes=1") || !strings.Contains(backlinksCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("asset backlinks completion = %s", backlinksCompletion)
	}
	moveCompletion := runCLI(t, "__complete", "asset", "move", "--vault", root, "dia")
	if !strings.Contains(moveCompletion, "diagram.png\timage/png linked_notes=1") || !strings.Contains(moveCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("asset move completion = %s", moveCompletion)
	}
	removeCompletion := runCLI(t, "__complete", "asset", "remove", "--vault", root, "dia")
	if !strings.Contains(removeCompletion, "diagram.png\timage/png linked_notes=1") || !strings.Contains(removeCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("asset remove completion = %s", removeCompletion)
	}

	attachSourceCompletion := runCLI(t, "__complete", "note", "attach", "Alpha", "--vault", root, "")
	if strings.Contains(attachSourceCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("note attach source file completion should remain enabled:\n%s", attachSourceCompletion)
	}
}

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

func TestAssetCommandContractsCLI(t *testing.T) {

	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	source := filepath.Join(root, "diagram-source.png")
	payload := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 'p', 'i', 'n', 'a', 'x', '-', 'b', 'i', 'n', 'a', 'r', 'y'}
	if err := os.WriteFile(source, payload, 0o644); err != nil {
		t.Fatalf("write asset source: %v", err)
	}

	addOut, addErr, err := runCLISeparate("asset", "add", source, "--vault", root, "--json")
	if err != nil || addErr != "" {
		t.Fatalf("asset add err=%v stderr=%q stdout=%s", err, addErr, addOut)
	}
	if strings.Contains(addOut, string(payload)) || strings.Contains(addOut, "pinax-binary") {
		t.Fatalf("asset add leaked payload:\n%s", addOut)
	}
	var addEnvelope map[string]any
	if err := json.Unmarshal([]byte(addOut), &addEnvelope); err != nil {
		t.Fatalf("asset add json invalid: %v\n%s", err, addOut)
	}
	addFacts := addEnvelope["facts"].(map[string]any)
	assetPath, _ := addFacts["asset_path"].(string)
	if addEnvelope["command"] != "asset.add" || addFacts["sha256"] == "" || addFacts["media_type"] != "image/png" || assetPath == "" {
		t.Fatalf("asset add envelope = %#v", addEnvelope)
	}

	listAgent, listErr, err := runCLISeparate("asset", "list", "--vault", root, "--agent")
	if err != nil || listErr != "" {
		t.Fatalf("asset list err=%v stderr=%q stdout=%s", err, listErr, listAgent)
	}
	for _, want := range []string{"command=asset.list", "fact.assets=1", "fact.asset.1.path="} {
		if !strings.Contains(listAgent, want) {
			t.Fatalf("asset list agent missing %q:\n%s", want, listAgent)
		}
	}

	showOut := runCLI(t, "asset", "show", filepath.Base(assetPath), "--vault", root, "--json")
	var showEnvelope map[string]any
	if err := json.Unmarshal([]byte(showOut), &showEnvelope); err != nil {
		t.Fatalf("asset show json invalid: %v\n%s", err, showOut)
	}
	showFacts := showEnvelope["facts"].(map[string]any)
	if showEnvelope["command"] != "asset.show" || showFacts["asset_path"] != assetPath || strings.Contains(showOut, "pinax-binary") {
		t.Fatalf("asset show envelope = %#v\n%s", showEnvelope, showOut)
	}

	verifyOut := runCLI(t, "asset", "verify", "--vault", root, "--json")
	var verifyEnvelope map[string]any
	if err := json.Unmarshal([]byte(verifyOut), &verifyEnvelope); err != nil {
		t.Fatalf("asset verify json invalid: %v\n%s", err, verifyOut)
	}
	verifyFacts := verifyEnvelope["facts"].(map[string]any)
	if verifyEnvelope["command"] != "asset.verify" || verifyFacts["verified"] != "1" || verifyFacts["missing"] != "0" || verifyFacts["changed"] != "0" {
		t.Fatalf("asset verify envelope = %#v", verifyEnvelope)
	}
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
	for _, want := range []string{"command=project.list", "fact.projects=1", "fact.current_project=research"} {
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

func TestSearchDefaultOutputShowsResultPathAndSnippet(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Demo Note", "--body", "这个 demo 片段应该直接出现在搜索结果里。", "--slug", "demo-note", "--vault", root, "--json")

	out := runCLI(t, "search", "demo", "--vault", root)
	for _, want := range []string{"demo-note.md", "Demo Note", "demo 片段"} {
		if !strings.Contains(out, want) {
			t.Fatalf("search default output missing %q:\n%s", want, out)
		}
	}
}

func TestNoteAddCommandRegistersOnlyPinaxNotes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "raw.md"), "# Raw Markdown\n\nraw-only marker\n")

	noteOut := runCLI(t, "note", "add", "Managed", "--body", "managed marker", "--vault", root, "--json")
	var noteEnvelope map[string]any
	if err := json.Unmarshal([]byte(noteOut), &noteEnvelope); err != nil {
		t.Fatalf("note add json invalid: %v\n%s", err, noteOut)
	}
	if noteEnvelope["command"] != "note.new" || noteEnvelope["status"] != "success" {
		t.Fatalf("note add envelope = %#v", noteEnvelope)
	}

	listOut := runCLI(t, "note", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=note.list", "fact.total=1", "fact.returned=1"} {
		if !strings.Contains(listOut, want) {
			t.Fatalf("note list output missing %q:\n%s", want, listOut)
		}
	}
	if strings.Contains(listOut, "raw.md") || strings.Contains(listOut, "Raw Markdown") {
		t.Fatalf("raw markdown leaked into note list:\n%s", listOut)
	}

	searchOut := runCLI(t, "search", "raw-only", "--vault", root, "--agent")
	if !strings.Contains(searchOut, "fact.matches=0") {
		t.Fatalf("raw markdown leaked into search:\n%s", searchOut)
	}
	assertMachineOutputClean(t, listOut)
	assertMachineOutputClean(t, searchOut)
}

func TestTemplateAuthoringCLIJSON(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	designOut := runCLI(t, "template", "create", "视频学习", "--vault", root, "--json")
	var designEnvelope map[string]any
	if err := json.Unmarshal([]byte(designOut), &designEnvelope); err != nil {
		t.Fatalf("design create json invalid: %v\n%s", err, designOut)
	}
	if designEnvelope["command"] != "template.create" || designEnvelope["status"] != "success" {
		t.Fatalf("design create envelope = %#v", designEnvelope)
	}
	designContent := readCLIFile(t, filepath.Join(root, ".pinax", "templates", "视频学习.md"))
	for _, want := range []string{"schema_version: pinax.template_design.v1", "kind: template_design", "title: 视频学习"} {
		if !strings.Contains(designContent, want) {
			t.Fatalf("template design missing %q:\n%s", want, designContent)
		}
	}

	source := filepath.Join(root, "meeting-template.md")
	writeCLIFixture(t, source, "# {{title}}\n客户: {{client}}\n")

	createOut := runCLI(t, "template", "create", "meeting", "--from", source, "--vault", root, "--json")
	var createEnvelope map[string]any
	if err := json.Unmarshal([]byte(createOut), &createEnvelope); err != nil {
		t.Fatalf("create json invalid: %v\n%s", err, createOut)
	}
	if createEnvelope["command"] != "template.create" || createEnvelope["status"] != "success" {
		t.Fatalf("create envelope = %#v", createEnvelope)
	}

	renderOut := runCLI(t, "template", "render", "meeting", "--title", "客户会议", "--var", "client=Acme", "--vault", root, "--json")
	if !strings.Contains(renderOut, "客户: Acme") {
		t.Fatalf("render output missing custom var:\n%s", renderOut)
	}

	validateOut := runCLI(t, "template", "validate", "meeting", "--vault", root, "--agent")
	for _, want := range []string{"command=template.validate", "status=success", "fact.issues=0"} {
		if !strings.Contains(validateOut, want) {
			t.Fatalf("validate output missing %q:\n%s", want, validateOut)
		}
	}

	noteOut := runCLI(t, "note", "new", "客户会议", "--template", "meeting", "--var", "client=Acme", "--tags", "meeting,client", "--vault", root, "--json")
	if !strings.Contains(noteOut, "note.new") {
		t.Fatalf("note output = %s", noteOut)
	}
	content := readCLIFile(t, filepath.Join(root, "客户会议.md"))
	if !strings.Contains(content, "客户: Acme") {
		t.Fatalf("note content = %s", content)
	}

	failed, err := runCLIExpectError("template", "delete", "meeting", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("delete without approval err=%v out=%s", err, failed)
	}
	deleteOut := runCLI(t, "template", "delete", "meeting", "--vault", root, "--yes", "--json")
	if !strings.Contains(deleteOut, "template.delete") {
		t.Fatalf("delete output = %s", deleteOut)
	}
}

func TestTemplateInspectPreviewOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "template", "create", "go-meeting", "--engine", "go-template", "--body", "# {{ .Title | upper }}\n客户: {{ .Vars.client }}\n", "--vault", root, "--json")

	inspectJSON, inspectStderr, inspectErr := runCLISeparate("template", "inspect", "go-meeting", "--vault", root, "--json")
	if inspectErr != nil || inspectStderr != "" {
		t.Fatalf("inspect json failed: err=%v stderr=%q stdout=%s", inspectErr, inspectStderr, inspectJSON)
	}
	var inspectEnvelope map[string]any
	if err := json.Unmarshal([]byte(inspectJSON), &inspectEnvelope); err != nil {
		t.Fatalf("inspect json invalid: %v\n%s", err, inspectJSON)
	}
	if inspectEnvelope["command"] != "template.inspect" || inspectEnvelope["mode"] != "json" || inspectEnvelope["status"] != "success" {
		t.Fatalf("inspect envelope = %#v", inspectEnvelope)
	}
	inspectFacts := inspectEnvelope["facts"].(map[string]any)
	if inspectFacts["template"] != "go-meeting" || inspectFacts["engine"] != "go-template" || inspectFacts["schema_version"] != "pinax.template.v2" {
		t.Fatalf("inspect facts = %#v", inspectFacts)
	}

	previewOut := runCLI(t, "template", "preview", "go-meeting", "--title", "weekly", "--var", "client=Acme", "--vault", root, "--agent")
	for _, want := range []string{"command=template.preview", "mode=agent", "status=success", "fact.template=go-meeting", "fact.engine=go-template"} {
		if !strings.Contains(previewOut, want) {
			t.Fatalf("preview agent output missing %q:\n%s", want, previewOut)
		}
	}

	stdout, stderr, err := runCLISeparate("template", "preview", "go-meeting", "--title", "weekly", "--var", "client=Acme", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("preview json stdout/stderr err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	var previewEnvelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &previewEnvelope); err != nil {
		t.Fatalf("preview json invalid: %v\n%s", err, stdout)
	}
	data := previewEnvelope["data"].(map[string]any)
	if body, _ := data["body"].(string); !strings.Contains(body, "# WEEKLY") || !strings.Contains(body, "客户: Acme") {
		t.Fatalf("preview body = %#v", data["body"])
	}
}

func TestPreviewSummaryShowsBodyAndTags(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "template", "create", "brief", "--body", "# {{title}}\nPreview body", "--vault", root, "--json")

	templateSummary := runCLI(t, "template", "preview", "brief", "--title", "Weekly", "--tags", "research,client", "--vault", root)
	for _, want := range []string{"Template preview generated", "Tags", "research,client", "# Weekly", "Preview body"} {
		if !strings.Contains(templateSummary, want) {
			t.Fatalf("template preview summary missing %q:\n%s", want, templateSummary)
		}
	}

	runCLI(t, "note", "new", "Tagged Preview", "--tags", "research,client", "--body", "preview body", "--vault", root, "--json")
	noteSummary := runCLI(t, "note", "preview", "Tagged Preview", "--vault", root)
	for _, want := range []string{"Local note read", "Tags", "research,client", "Tagged Preview", "preview body"} {
		if !strings.Contains(noteSummary, want) {
			t.Fatalf("note preview summary missing %q:\n%s", want, noteSummary)
		}
	}
}

func TestTemplateQueryBackedCLIOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "A", "--body", "priority:: 1\n", "--status", "active", "--vault", root, "--json")
	runCLI(t, "note", "new", "B", "--body", "priority:: 2\n", "--status", "done", "--vault", root, "--json")

	body := strings.Join([]string{
		"---",
		"schema_version: pinax.template.v2",
		"engine: go-template",
		"kind: note",
		"queries:",
		"  active:",
		"    language: sql",
		"    sql: SELECT title, status FROM notes WHERE status = \"active\" LIMIT 5",
		"    kind: table",
		"    max_rows: 5",
		"    required: true",
		"---",
		"# Active",
		"{{ table .Queries.active }}",
	}, "\n")
	runCLI(t, "template", "create", "active-report", "--body", body, "--vault", root, "--json")

	inspectJSON, inspectStderr, inspectErr := runCLISeparate("template", "inspect", "active-report", "--vault", root, "--json")
	if inspectErr != nil || inspectStderr != "" {
		t.Fatalf("inspect query json err=%v stderr=%q stdout=%s", inspectErr, inspectStderr, inspectJSON)
	}
	var inspectEnvelope map[string]any
	if err := json.Unmarshal([]byte(inspectJSON), &inspectEnvelope); err != nil {
		t.Fatalf("inspect query json invalid: %v\n%s", err, inspectJSON)
	}
	facts := inspectEnvelope["facts"].(map[string]any)
	if facts["queries"] != "1" || facts["engine"] != "go-template" || !strings.Contains(inspectJSON, "query_explain") {
		t.Fatalf("inspect query envelope = %#v", inspectEnvelope)
	}

	previewAgent := runCLI(t, "template", "preview", "active-report", "--title", "Active Report", "--vault", root, "--agent")
	for _, want := range []string{"command=template.preview", "mode=agent", "status=success", "fact.template=active-report", "fact.engine=go-template"} {
		if !strings.Contains(previewAgent, want) {
			t.Fatalf("preview query agent missing %q:\n%s", want, previewAgent)
		}
	}
	assertMachineOutputClean(t, previewAgent)

	previewJSON := runCLI(t, "template", "preview", "active-report", "--title", "Active Report", "--vault", root, "--json")
	var previewEnvelope map[string]any
	if err := json.Unmarshal([]byte(previewJSON), &previewEnvelope); err != nil {
		t.Fatalf("preview query json invalid: %v\n%s", err, previewJSON)
	}
	data := previewEnvelope["data"].(map[string]any)
	bodyOut, _ := data["body"].(string)
	if !strings.Contains(bodyOut, "| title | status |") || !strings.Contains(bodyOut, "| A | active |") || strings.Contains(bodyOut, "| B | done |") {
		t.Fatalf("preview query body = %q", bodyOut)
	}
}

func TestNoteShowRenderedAndRefreshCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "A", "--body", "priority:: 1\n", "--status", "active", "--vault", root, "--json")
	runCLI(t, "note", "new", "B", "--body", "priority:: 2\n", "--status", "done", "--vault", root, "--json")
	dashboardBody := strings.Join([]string{
		"Dashboard intro",
		"```pinax-sql active",
		"SELECT title, status FROM notes WHERE status = \"active\" LIMIT 5",
		"```",
		"<!-- pinax:render active start -->",
		"stale",
		"<!-- pinax:render active end -->",
	}, "\n")
	dashboardOut := runCLI(t, "note", "new", "Dashboard", "--body", dashboardBody, "--status", "active", "--vault", root, "--json")
	var dashboardEnvelope map[string]any
	if err := json.Unmarshal([]byte(dashboardOut), &dashboardEnvelope); err != nil {
		t.Fatalf("dashboard create json invalid: %v\n%s", err, dashboardOut)
	}
	path := dashboardEnvelope["facts"].(map[string]any)["path"].(string)

	sourceJSON := runCLI(t, "note", "show", "Dashboard", "--view", "source", "--vault", root, "--json")
	if !strings.Contains(sourceJSON, "```pinax-sql active") || !strings.Contains(sourceJSON, `"query_count":"0"`) {
		t.Fatalf("source json = %s", sourceJSON)
	}
	renderedJSON := runCLI(t, "note", "show", "Dashboard", "--view", "rendered", "--vault", root, "--json")
	if !strings.Contains(renderedJSON, "| A | active |") || strings.Contains(renderedJSON, "| B | done |") || !strings.Contains(renderedJSON, `"view":"rendered"`) {
		t.Fatalf("rendered json = %s", renderedJSON)
	}
	refreshFail, err := runCLIExpectError("note", "refresh", "Dashboard", "--rendered", "--vault", root, "--json")
	if err == nil || !strings.Contains(refreshFail, "approval_required") {
		t.Fatalf("refresh without yes err=%v out=%s", err, refreshFail)
	}
	refreshOut := runCLI(t, "note", "refresh", "Dashboard", "--rendered", "--yes", "--vault", root, "--json")
	if !strings.Contains(refreshOut, `"changed_blocks":"1"`) || !strings.Contains(refreshOut, `"query_count":"1"`) {
		t.Fatalf("refresh out = %s", refreshOut)
	}
	content := readCLIFile(t, filepath.Join(root, path))
	if !strings.Contains(content, "```pinax-sql active") || !strings.Contains(content, "| A | active |") || strings.Contains(content, "stale") {
		t.Fatalf("refreshed content = %s", content)
	}
}

func TestRenderRunSnapshotAndPruneCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "template", "create", "study", "--engine", "go-template", "--body", "# {{ .Title }}\nURL: {{ .Vars.url }}\n", "--vault", root, "--json")

	saveOut := runCLI(t, "template", "render", "study", "--title", "Go", "--var", "url=https://go.dev", "--save-run", "go-run", "--vault", root, "--json")
	if !strings.Contains(saveOut, `"run_saved":"true"`) || !strings.Contains(saveOut, `"run_name":"go-run"`) {
		t.Fatalf("save run out = %s", saveOut)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "renders", "templates", "study", "index.json")); err != nil {
		t.Fatalf("template render index missing: %v", err)
	}

	reuseOut := runCLI(t, "template", "render", "study", "--run", "go-run", "--vault", root, "--json")
	if !strings.Contains(reuseOut, "URL: https://go.dev") || !strings.Contains(reuseOut, `"run":"go-run"`) {
		t.Fatalf("reuse run out = %s", reuseOut)
	}
	inspectRuns := runCLI(t, "template", "inspect", "study", "--runs", "--vault", root, "--json")
	if !strings.Contains(inspectRuns, "go-run") || !strings.Contains(inspectRuns, "render_runs") {
		t.Fatalf("inspect runs = %s", inspectRuns)
	}
	templateRunCompletion := runCLI(t, "__complete", "template", "render", "study", "--vault", root, "--run", "")
	if !strings.Contains(templateRunCompletion, "go-run	render-run") || !strings.Contains(templateRunCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("template run completion = %s", templateRunCompletion)
	}

	runCLI(t, "note", "new", "A", "--body", "priority:: 1\n", "--status", "active", "--vault", root, "--json")
	dashboardBody := strings.Join([]string{
		"```pinax-sql active",
		"SELECT title, status FROM notes WHERE status = \"active\" LIMIT 5",
		"```",
		"<!-- pinax:render active start -->",
		"empty",
		"<!-- pinax:render active end -->",
	}, "\n")
	runCLI(t, "note", "new", "Dashboard", "--body", dashboardBody, "--status", "active", "--vault", root, "--json")
	refreshOut := runCLI(t, "note", "refresh", "Dashboard", "--rendered", "--save-run", "dash", "--yes", "--vault", root, "--json")
	if !strings.Contains(refreshOut, `"run_saved":"true"`) || !strings.Contains(refreshOut, `"run_name":"dash"`) {
		t.Fatalf("refresh save run = %s", refreshOut)
	}
	snapshotOut := runCLI(t, "note", "show", "Dashboard", "--view", "rendered", "--snapshot", "latest", "--vault", root, "--json")
	if !strings.Contains(snapshotOut, "| A | active |") || !strings.Contains(snapshotOut, `"snapshot":"latest"`) {
		t.Fatalf("snapshot out = %s", snapshotOut)
	}
	snapshotCompletion := runCLI(t, "__complete", "note", "show", "Dashboard", "--view", "rendered", "--vault", root, "--snapshot", "")
	if !strings.Contains(snapshotCompletion, "dash	render-run") || !strings.Contains(snapshotCompletion, "latest	render-run") || !strings.Contains(snapshotCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("snapshot completion = %s", snapshotCompletion)
	}

	pruneDry := runCLI(t, "template", "runs", "prune", "study", "--keep", "0", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(pruneDry, `"dry_run":"true"`) || !strings.Contains(pruneDry, `"delete_candidates":"`) {
		t.Fatalf("prune dry = %s", pruneDry)
	}
	repairOut := runCLI(t, "template", "runs", "repair", "--vault", root, "--json")
	if !strings.Contains(repairOut, "template.runs.repair") {
		t.Fatalf("repair out = %s", repairOut)
	}
}

func TestTemplateAuthoringCLIRejectsBadInput(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("template", "create", "../bad", "--body", "x", "--vault", root, "--json")
	if err == nil || !strings.Contains(out, "invalid_template_name") {
		t.Fatalf("unsafe template create err=%v out=%s", err, out)
	}
	out, err = runCLIExpectError("template", "render", "note", "--var", "bad", "--vault", root, "--json")
	if err == nil || !strings.Contains(out, "template_variable_invalid") {
		t.Fatalf("bad var err=%v out=%s", err, out)
	}
}

func TestCLITreeHelpSmoke(t *testing.T) {
	for _, tc := range []struct {
		args   []string
		want   []string
		absent []string
	}{
		{args: []string{"--help"}, want: []string{"Local vault", "Note workflows", "Organization and search", "Automation and integrations", "Configuration and maintenance", "vault", "journal", "storage", "organize", "note", "folder", "template"}, absent: []string{"\n  daily ", "\n  weekly ", "\n  monthly ", "\n  stats ", "\n  validate ", "\n  doctor ", "\n  dashboard ", "\n  tag ", "\n  kind ", "\n  group ", "\n  schema "}},
		{args: []string{"vault", "--help"}, want: []string{"stats", "validate", "doctor", "dashboard"}},
		{args: []string{"journal", "--help"}, want: []string{"daily", "weekly", "monthly"}},
		{args: []string{"storage", "--help"}, want: []string{"set", "status", "doctor"}, absent: []string{"\n  set-local ", "\n  set-s3 "}},
		{args: []string{"storage", "set", "--help"}, want: []string{"local", "s3"}},
		{args: []string{"note", "--help"}, want: []string{"new", "show", "tags", "folders", "kinds", "groups"}},
		{args: []string{"organize", "--help"}, want: []string{"plan", "list", "apply"}, absent: []string{"\n  suggest "}},
	} {
		out := runCLI(t, tc.args...)
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Fatalf("help %v missing %q:\n%s", tc.args, want, out)
			}
		}
		for _, absent := range tc.absent {
			if strings.Contains(out, absent) {
				t.Fatalf("help %v should hide compatibility command %q:\n%s", tc.args, strings.TrimSpace(absent), out)
			}
		}
	}
}

func TestNoteDimensionPrimaryPaths(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Dim", "--tags", "research", "--kind", "reference", "--folder", "work", "--vault", root, "--json")
	humanTags := runCLI(t, "note", "tags", "--vault", root)
	for _, want := range []string{"Tag", "Count", "Share", "Heat", "research", "##########"} {
		if !strings.Contains(humanTags, want) {
			t.Fatalf("note tags summary missing %q:\n%s", want, humanTags)
		}
	}
	for _, tc := range []struct {
		legacy  []string
		primary []string
		command string
	}{
		{legacy: []string{"tag", "list"}, primary: []string{"note", "tags"}, command: "tag.list"},
		{legacy: []string{"kind", "list"}, primary: []string{"note", "kinds"}, command: "kind.list"},
		{legacy: []string{"group", "list"}, primary: []string{"note", "groups"}, command: "group.list"},
	} {
		legacyArgs := append(append([]string{}, tc.legacy...), "--vault", root, "--json")
		primaryArgs := append(append([]string{}, tc.primary...), "--vault", root, "--json")
		legacyOut := runCLI(t, legacyArgs...)
		primaryOut := runCLI(t, primaryArgs...)
		assertSameCommandAndFacts(t, legacyOut, primaryOut, tc.command)
	}
}

func TestCLITreePrimaryPathAliases(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	rootStats := runCLI(t, "stats", "--vault", root, "--json")
	vaultStats := runCLI(t, "vault", "stats", "--vault", root, "--json")
	assertSameCommandAndFacts(t, rootStats, vaultStats, "vault.stats")

	rootValidate := runCLI(t, "validate", "--vault", root, "--json")
	vaultValidate := runCLI(t, "vault", "validate", "--vault", root, "--json")
	assertSameCommandAndFacts(t, rootValidate, vaultValidate, "vault.validate")

	runCLI(t, "daily", "append", "--body", "alias", "--vault", root, "--json")
	dailyRoot := runCLI(t, "daily", "show", "--vault", root, "--json")
	dailyPrimary := runCLI(t, "journal", "daily", "show", "--vault", root, "--json")
	assertSameCommandAndFacts(t, dailyRoot, dailyPrimary, "daily.show")

	legacyStorage := runCLI(t, "storage", "set-local", "--root", root, "--vault", root, "--json")
	primaryStorage := runCLI(t, "storage", "set", "local", "--root", root, "--vault", root, "--json")
	assertSameCommandAndFacts(t, legacyStorage, primaryStorage, "storage.set_local")

	rootSchema := runCLI(t, "schema", "export", "--format", "openapi", "--vault", root, "--json")
	apiSchema := runCLI(t, "api", "schema", "export", "--format", "openapi", "--vault", root, "--json")
	assertSameCommandAndFacts(t, rootSchema, apiSchema, "api.schema.export")
	schemaHelp := runCLI(t, "schema", "--help")
	for _, want := range []string{"pinax schema export", "Export the local API schema"} {
		if !strings.Contains(schemaHelp, want) {
			t.Fatalf("schema help missing %q:\n%s", want, schemaHelp)
		}
	}
}

func TestAPIRoutesHumanOutputListsEndpointsCLI(t *testing.T) {
	root := t.TempDir()
	out := runCLI(t, "api", "routes", "--vault", root)
	for _, want := range []string{"GET /v1/projects/{slug}/board", "CALL Pinax.Note.Read", "project.board.show"} {
		if !strings.Contains(out, want) {
			t.Fatalf("api routes human output missing %q:\n%s", want, out)
		}
	}
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("api routes human output should not be JSON:\n%s", out)
	}
}

func TestCLIRemoteModeForwardsSupportedCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rpc" || r.Method != http.MethodPost {
			t.Fatalf("unexpected remote request %s %s", r.Method, r.URL.Path)
		}
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode remote request: %v", err)
		}
		if req.Method != "Pinax.Folder.List" || req.Params["include_empty"] != true || req.Params["purpose"] != "notes" {
			t.Fatalf("remote request = %#v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": "1.0", "mode": "json", "command": "folder.list", "status": "success", "facts": map[string]string{"remote": "true"}})
	}))
	defer server.Close()

	out := runCLI(t, "--api-url", server.URL, "folder", "list", "--purpose", "notes", "--include-empty", "--json")
	assertJSONCommandStatus(t, out, "folder.list", "success")
	if !strings.Contains(out, `"remote":"true"`) {
		t.Fatalf("remote output missing returned projection facts: %s", out)
	}
}

func TestCLIRemoteModeEnvironmentAndAgentOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode remote request: %v", err)
		}
		if req.Method != "Pinax.Inbox.List" {
			t.Fatalf("remote method = %s", req.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": "1.0", "mode": "json", "command": "inbox.list", "status": "success", "facts": map[string]string{"count": "0"}})
	}))
	defer server.Close()
	t.Setenv("PINAX_API_URL", server.URL)

	out := runCLI(t, "inbox", "list", "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=inbox.list", "status=success", "fact.count=0"} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent output missing %q:\n%s", want, out)
		}
	}
}

func TestCLIRemoteModeRejectsVaultConflictAndUnsupportedCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("remote server should not be called for invalid remote mode command")
	}))
	defer server.Close()

	out, err := runCLIExpectError("--api-url", server.URL, "--vault", t.TempDir(), "folder", "list", "--json")
	if err == nil || !strings.Contains(out, "remote_vault_conflict") {
		t.Fatalf("expected remote_vault_conflict, err=%v out=%s", err, out)
	}
	out, err = runCLIExpectError("--api-url", server.URL, "version", "--json")
	if err == nil || !strings.Contains(out, "remote_command_unsupported") {
		t.Fatalf("expected remote_command_unsupported, err=%v out=%s", err, out)
	}
	tokenFile := filepath.Join(t.TempDir(), "token.txt")
	writeCLIFixture(t, tokenFile, "file-token")
	out, err = runCLIExpectError("--api-url", server.URL, "--api-token", "inline-token", "--api-token-file", tokenFile, "folder", "list", "--json")
	if err == nil || !strings.Contains(out, "remote_token_conflict") || strings.Contains(out, "inline-token") || strings.Contains(out, "file-token") {
		t.Fatalf("expected redacted remote_token_conflict, err=%v out=%s", err, out)
	}
}

func TestCLIRemoteModeTokenSourcesStayRedacted(t *testing.T) {
	const secret = "pinax-remote-secret"
	tokenFile := filepath.Join(t.TempDir(), "token.txt")
	writeCLIFixture(t, tokenFile, secret+"\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
			t.Fatalf("authorization header = %q", got)
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"spec_version": "1.0", "mode": "json", "command": "folder.create", "status": "failed", "error": map[string]string{"code": "write_disabled", "message": "remote writes disabled"}})
	}))
	defer server.Close()

	out, err := runCLIExpectError("--api-url", server.URL, "--api-token-file", tokenFile, "folder", "create", "secret-folder", "--yes", "--json")
	if err == nil || !strings.Contains(out, "write_disabled") {
		t.Fatalf("expected remote projection error, err=%v out=%s", err, out)
	}
	if strings.Contains(out, secret) || strings.Contains(out, "Authorization") {
		t.Fatalf("remote output leaked token/header: %s", out)
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
	for _, want := range []string{"stats", "doctor", "dashboard"} {
		if !strings.Contains(vaultHelp, want) {
			t.Fatalf("vault help missing %q:\n%s", want, vaultHelp)
		}
	}
}

func TestRepairPlanJSONIsReadonlyAndSaveWritesPlanAsset(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	notePath := filepath.Join(root, "No Tags.md")
	writeCLIFixture(t, notePath, "# No Tags\n\nbody\n")
	beforeNote := readCLIFile(t, notePath)

	out := runCLI(t, "repair", "plan", "--vault", root, "--json")
	if got := readCLIFile(t, notePath); got != beforeNote {
		t.Fatalf("repair plan modified markdown:\n%s", got)
	}
	if fileExists(filepath.Join(root, ".pinax", "repair-plans")) {
		t.Fatalf("repair plan without --save wrote repair-plans asset")
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("repair plan json invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "repair.plan" || envelope["status"] != "partial" || envelope["mode"] != "json" {
		t.Fatalf("repair plan envelope = %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	if data["schema_version"] != "pinax.repair_plan.v1" || data["plan_id"] == "" {
		t.Fatalf("repair plan identity missing: %#v", data)
	}
	if len(data["operations"].([]any)) == 0 {
		t.Fatalf("repair plan operations missing: %#v", data)
	}
	if len(envelope["actions"].([]any)) == 0 {
		t.Fatalf("repair plan next actions missing: %#v", envelope)
	}

	savedOut := runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved repair plan json invalid: %v\n%s", err, savedOut)
	}
	savedData := savedEnvelope["data"].(map[string]any)
	savedPath := savedData["saved_path"].(string)
	if savedPath == "" || !strings.HasPrefix(savedPath, ".pinax/repair-plans/") {
		t.Fatalf("saved path not relative repair plan asset: %#v", savedData)
	}
	planContent := readCLIFile(t, filepath.Join(root, filepath.FromSlash(savedPath)))
	var savedPlan map[string]any
	if err := json.Unmarshal([]byte(planContent), &savedPlan); err != nil {
		t.Fatalf("saved repair plan invalid json: %v\n%s", err, planContent)
	}
	if savedPlan["schema_version"] != "pinax.repair_plan.v1" || savedPlan["status"] != "planned" {
		t.Fatalf("saved repair plan fields = %#v", savedPlan)
	}
}

func TestDoctorRepairAndOrganizeUseLinkEvidence(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	sourcePath := filepath.Join(root, "Source.md")
	writeCLIFixture(t, sourcePath, pinaxNoteFixture("note_source", "Source", "[]", "[[Missing Target]]\n\n[[Shared]]\n"))
	writeCLIFixture(t, filepath.Join(root, "First.md"), pinaxNoteFixture("note_first", "Shared", "[]", "first\n"))
	writeCLIFixture(t, filepath.Join(root, "Second.md"), pinaxNoteFixture("note_second", "Shared", "[]", "second\n"))
	writeCLIFixture(t, filepath.Join(root, "Orphan.md"), pinaxNoteFixture("note_orphan", "Orphan", "[]", "solo\n"))
	beforeSource := readCLIFile(t, sourcePath)

	doctorOut := runCLI(t, "doctor", "--vault", root, "--json")
	var doctorEnvelope map[string]any
	if err := json.Unmarshal([]byte(doctorOut), &doctorEnvelope); err != nil {
		t.Fatalf("doctor json invalid: %v\n%s", err, doctorOut)
	}
	doctorData := doctorEnvelope["data"].(map[string]any)
	issues := doctorData["issues"].([]any)
	for _, code := range []string{"broken_link", "ambiguous_link", "orphan_note"} {
		if !hasIssue(issues, code) {
			t.Fatalf("doctor issue %s missing: %#v", code, issues)
		}
	}

	repairOut := runCLI(t, "repair", "plan", "--vault", root, "--json")
	if got := readCLIFile(t, sourcePath); got != beforeSource {
		t.Fatalf("repair plan rewrote note body:\n%s", got)
	}
	if fileExists(filepath.Join(root, ".pinax", "repair-plans")) {
		t.Fatalf("repair plan without --save wrote repair plan asset")
	}
	var repairEnvelope map[string]any
	if err := json.Unmarshal([]byte(repairOut), &repairEnvelope); err != nil {
		t.Fatalf("repair plan json invalid: %v\n%s", err, repairOut)
	}
	repairOps := repairEnvelope["data"].(map[string]any)["operations"].([]any)
	for _, want := range []struct {
		kind   string
		target string
	}{
		{kind: "link_resolution", target: "Missing Target"},
		{kind: "link_rewrite", target: "Shared"},
		{kind: "orphan_review", target: "Orphan"},
	} {
		if !hasManualReviewOperation(repairOps, want.kind, want.target) {
			t.Fatalf("repair operation %s target %q missing: %#v", want.kind, want.target, repairOps)
		}
	}

	organizeOut := runCLI(t, "organize", "suggest", "--vault", root, "--json")
	if got := readCLIFile(t, sourcePath); got != beforeSource {
		t.Fatalf("organize suggest rewrote note body:\n%s", got)
	}
	if fileExists(filepath.Join(root, ".pinax", "organize-plans")) {
		t.Fatalf("organize suggest without --save wrote organize plan asset")
	}
	var organizeEnvelope map[string]any
	if err := json.Unmarshal([]byte(organizeOut), &organizeEnvelope); err != nil {
		t.Fatalf("organize suggest json invalid: %v\n%s", err, organizeOut)
	}
	organizeOps := organizeEnvelope["data"].(map[string]any)["operations"].([]any)
	for _, want := range []struct {
		kind   string
		target string
	}{
		{kind: "link_resolution", target: "Missing Target"},
		{kind: "link_rewrite", target: "Shared"},
		{kind: "orphan_review", target: "Orphan"},
	} {
		if !hasManualReviewOperation(organizeOps, want.kind, want.target) {
			t.Fatalf("organize operation %s target %q missing: %#v", want.kind, want.target, organizeOps)
		}
	}
}

func hasIssue(issues []any, code string) bool {
	for _, item := range issues {
		issue := item.(map[string]any)
		if issue["issue_code"] == code && len(issue["evidence"].([]any)) > 0 {
			return true
		}
	}
	return false
}

func hasManualReviewOperation(operations []any, kind, target string) bool {
	for _, item := range operations {
		op := item.(map[string]any)
		if op["kind"] != kind || op["mode"] != "manual_review" || op["risk"] != "review" {
			continue
		}
		if !strings.Contains(fmt.Sprint(op["target"]), target) {
			continue
		}
		if len(op["evidence"].([]any)) == 0 {
			continue
		}
		return true
	}
	return false
}

func TestRepairApplyRequiresApprovalAndSnapshot(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "No Tags.md"), pinaxNoteFixture("note_no_tags", "No Tags", "[]", "body\n"))
	savedOut := runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved repair plan json invalid: %v\n%s", err, savedOut)
	}
	planID := savedEnvelope["facts"].(map[string]any)["plan_id"].(string)
	eventsPath := filepath.Join(root, ".pinax", "events.jsonl")
	eventsBefore := ""
	if fileExists(eventsPath) {
		eventsBefore = readCLIFile(t, eventsPath)
	}

	failed, err := runCLIExpectError("repair", "apply", "--vault", root, "--plan", planID, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("repair apply without approval err=%v out=%s", err, failed)
	}
	if got := readCLIFile(t, eventsPath); got != eventsBefore {
		t.Fatalf("repair apply without approval changed events:\n%s", got)
	}

	failed, err = runCLIExpectError("repair", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	if err == nil || !strings.Contains(failed, "snapshot_required") {
		t.Fatalf("repair apply without snapshot err=%v out=%s", err, failed)
	}
	if !strings.Contains(failed, "pinax version snapshot") || strings.Contains(failed, "pinax git snapshot") {
		t.Fatalf("snapshot error missing version action or leaked git action:\n%s", failed)
	}
}
func TestRepairApplyProjectionOnlyPlanDoesNotRequireSnapshot(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	savedOut := runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved projection repair plan json invalid: %v\n%s", err, savedOut)
	}
	planID := savedEnvelope["facts"].(map[string]any)["plan_id"].(string)
	data := savedEnvelope["data"].(map[string]any)
	ops := fmt.Sprint(data["operations"])
	if !strings.Contains(ops, "index_rebuild") || strings.Contains(ops, "metadata_patch") || strings.Contains(ops, "tags_patch") {
		t.Fatalf("expected projection-only operations, got %s", ops)
	}

	applyOut := runCLI(t, "repair", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	if strings.Contains(applyOut, "snapshot_required") || !strings.Contains(applyOut, "repair.apply") {
		t.Fatalf("projection-only repair apply output = %s", applyOut)
	}
}

func TestRepairApplyLowRiskOperationsAndRejectsStalePlan(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	notePath := filepath.Join(root, "No Tags.md")
	writeCLIFixture(t, notePath, pinaxNoteFixture("note_no_tags", "No Tags", "[]", "body\n"))
	savedOut := runCLI(t, "repair", "plan", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved repair plan json invalid: %v\n%s", err, savedOut)
	}
	planID := savedEnvelope["facts"].(map[string]any)["plan_id"].(string)

	applyOut := runCLI(t, "repair", "apply", "--vault", root, "--plan", planID, "--yes", "--snapshot-message", "repair 前快照", "--json")
	if !strings.Contains(applyOut, "repair.apply") || !strings.Contains(applyOut, "\"status\":\"success\"") {
		t.Fatalf("repair apply output = %s", applyOut)
	}
	content := readCLIFile(t, notePath)
	for _, want := range []string{"schema_version: pinax.note.v1", "note_id:", "title: No Tags", "tags: []", "# No Tags"} {
		if !strings.Contains(content, want) {
			t.Fatalf("repair apply note missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, ".pinax", "events.jsonl")), "repair.apply") {
		t.Fatalf("repair apply did not append event evidence")
	}

	staleRoot := t.TempDir()
	runCLI(t, "init", staleRoot, "--title", "Vault", "--json")
	staleNotePath := filepath.Join(staleRoot, "No Tags.md")
	writeCLIFixture(t, staleNotePath, pinaxNoteFixture("note_no_tags", "No Tags", "[]", "body\n"))
	staleSavedOut := runCLI(t, "repair", "plan", "--vault", staleRoot, "--save", "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleSavedOut), &staleEnvelope); err != nil {
		t.Fatalf("stale saved repair plan json invalid: %v\n%s", err, staleSavedOut)
	}
	stalePlanID := staleEnvelope["facts"].(map[string]any)["plan_id"].(string)
	writeCLIFixture(t, staleNotePath, pinaxNoteFixture("note_no_tags", "No Tags", "[]", "changed\n"))
	failed, err := runCLIExpectError("repair", "apply", "--vault", staleRoot, "--plan", stalePlanID, "--yes", "--snapshot-message", "repair 前快照", "--json")
	if err == nil || !strings.Contains(failed, "plan_stale") || !strings.Contains(failed, "pinax repair plan") {
		t.Fatalf("stale repair apply err=%v out=%s", err, failed)
	}
}

func TestOrganizeSuggestCreatesReviewableAgentPlan(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "Research Idea.md"), pinaxNoteFixture("note_research_idea", "Research Idea", "[]", "body #research [[Missing Target]]\n\n![Missing](missing.png)\n"))
	writeCLIFixture(t, filepath.Join(root, ".agents", "skills", "Internal.md"), "# Internal\n\nagent asset\n")
	writeCLIFixture(t, filepath.Join(root, "docs", "Product.md"), "# Product\n\nproject doc\n")
	writeCLIFixture(t, filepath.Join(root, "AGENTS.md"), "# Agent Rules\n\nproject rules\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-06.md"), "# Daily 2026-06-06\n\nlog\n")

	out := runCLI(t, "organize", "suggest", "--vault", root, "--json")
	if fileExists(filepath.Join(root, ".pinax", "organize-plans")) {
		t.Fatalf("organize suggest without --save wrote organize-plans asset")
	}
	humanOut := runCLI(t, "organize", "suggest", "--vault", root)
	for _, want := range []string{"Operation preview", "Mode", "Risk", "Action", "Source", "Target", "Reason", "Research Idea.md", "pinax organize plan --vault", "--save"} {
		if !strings.Contains(humanOut, want) {
			t.Fatalf("organize suggest human output missing %q:\n%s", want, humanOut)
		}
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("organize suggest json invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "organize.suggest" || envelope["status"] != "partial" {
		t.Fatalf("organize suggest envelope = %#v", envelope)
	}
	data := envelope["data"].(map[string]any)
	if data["schema_version"] != "pinax.organize_plan.v1" || data["plan_id"] == "" || data["status"] != "planned" {
		t.Fatalf("organize plan identity missing: %#v", data)
	}
	operations := data["operations"].([]any)
	if len(operations) == 0 {
		t.Fatalf("organize plan operations missing: %#v", data)
	}
	operationKinds := map[string]bool{}
	for _, item := range operations {
		operation := item.(map[string]any)
		operationKinds[operation["kind"].(string)] = true
		path := fmt.Sprint(operation["path"])
		if strings.Contains(path, ".agents/") || strings.HasPrefix(path, "docs/") || path == "AGENTS.md" {
			t.Fatalf("organize plan should not include project assets: %#v", operation)
		}
		if operation["kind"] == "move" && strings.HasPrefix(path, "notes/") {
			t.Fatalf("organize plan should not move notes already under notes/: %#v", operation)
		}
	}
	for _, kind := range []string{"move", "tag_patch", "kind_patch", "status_patch", "link_resolution", "attachment_repair"} {
		if !operationKinds[kind] {
			t.Fatalf("organize operation kind %s missing: %#v", kind, operations)
		}
	}
	op := operations[0].(map[string]any)
	for _, key := range []string{"operation_id", "kind", "mode", "risk", "path", "reason", "evidence"} {
		if op[key] == nil || op[key] == "" {
			t.Fatalf("organize operation missing %s: %#v", key, op)
		}
	}

	savedOut := runCLI(t, "organize", "suggest", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved organize suggest json invalid: %v\n%s", err, savedOut)
	}
	savedData := savedEnvelope["data"].(map[string]any)
	savedPath := savedData["saved_path"].(string)
	if savedPath == "" || !strings.HasPrefix(savedPath, ".pinax/organize-plans/") {
		t.Fatalf("saved organize path invalid: %#v", savedData)
	}
	savedHumanOut := runCLI(t, "organize", "suggest", "--vault", root, "--save")
	for _, want := range []string{"Saved path", "pinax organize apply --vault", "--snapshot-message", "snapshot before organize"} {
		if !strings.Contains(savedHumanOut, want) {
			t.Fatalf("saved organize human output missing %q:\n%s", want, savedHumanOut)
		}
	}
	planContent := readCLIFile(t, filepath.Join(root, filepath.FromSlash(savedPath)))
	if !strings.Contains(planContent, "pinax.organize_plan.v1") || !strings.Contains(planContent, "Research Idea.md") {
		t.Fatalf("saved organize plan content invalid:\n%s", planContent)
	}
	listOut := runCLI(t, "organize", "list", "--vault", root, "--json")
	if !strings.Contains(listOut, "organize.list") || !strings.Contains(listOut, savedData["plan_id"].(string)) {
		t.Fatalf("organize list output invalid:\n%s", listOut)
	}
	listHumanOut := runCLI(t, "organize", "list", "--vault", root)
	for _, want := range []string{"Saved plans", "Planned", "Operation", savedData["plan_id"].(string), "pinax organize apply --vault", "--plan"} {
		if !strings.Contains(listHumanOut, want) {
			t.Fatalf("organize list human output missing %q:\n%s", want, listHumanOut)
		}
	}

	agentOut := runCLI(t, "organize", "suggest", "--vault", root, "--save", "--agent")
	for _, want := range []string{"command=organize.suggest", "status=partial", "fact.plan_id=", "fact.operations=", "fact.automatic=", "fact.manual_review=", "fact.risk.low=", "fact.saved_path=.pinax/organize-plans/"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("organize suggest agent missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, "状态:") || strings.Contains(agentOut, "重点:") {
		t.Fatalf("organize suggest agent contains human prose:\n%s", agentOut)
	}
}

func TestOrganizeListEmptyGuidesSaveWorkflow(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	out := runCLI(t, "organize", "list", "--vault", root)
	for _, want := range []string{"Plans", "0", "Next step", "pinax organize plan --vault", "--save"} {
		if !strings.Contains(out, want) {
			t.Fatalf("empty organize list output missing %q:\n%s", want, out)
		}
	}
}

func TestOrganizeApplySavedPlanRejectsStaleAndMoves(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	source := filepath.Join(root, "Research Idea.md")
	writeCLIFixture(t, source, pinaxNoteFixture("note_research_idea", "Research Idea", "[]", "body #research\n"))
	savedOut := runCLI(t, "organize", "suggest", "--vault", root, "--save", "--json")
	var savedEnvelope map[string]any
	if err := json.Unmarshal([]byte(savedOut), &savedEnvelope); err != nil {
		t.Fatalf("saved organize suggest json invalid: %v\n%s", err, savedOut)
	}
	planID := savedEnvelope["facts"].(map[string]any)["plan_id"].(string)

	failed, err := runCLIExpectError("organize", "apply", "--vault", root, "--plan", planID, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("organize apply saved plan without approval err=%v out=%s", err, failed)
	}
	failedHuman, err := runCLIExpectError("organize", "apply", "--vault", root, "--plan", planID)
	if err == nil {
		t.Fatalf("organize apply human without approval unexpectedly succeeded: %s", failedHuman)
	}
	for _, want := range []string{"approval_required", "organize apply requires --yes", "pinax organize plan --vault", "--save", "pinax organize apply --vault", "--plan", "--snapshot-message"} {
		if !strings.Contains(failedHuman, want) {
			t.Fatalf("organize apply human error missing %q:\n%s", want, failedHuman)
		}
	}
	failed, err = runCLIExpectError("organize", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	if err == nil || !strings.Contains(failed, "snapshot_required") {
		t.Fatalf("organize apply saved plan without snapshot err=%v out=%s", err, failed)
	}
	if !strings.Contains(failed, "pinax version snapshot") || strings.Contains(failed, "pinax git snapshot") {
		t.Fatalf("organize snapshot error missing version action or leaked git action:\n%s", failed)
	}

	applyOut := runCLI(t, "organize", "apply", "--vault", root, "--plan", planID, "--yes", "--snapshot-message", "整理前快照", "--json")
	var applyEnvelope map[string]any
	if err := json.Unmarshal([]byte(applyOut), &applyEnvelope); err != nil {
		t.Fatalf("organize apply json invalid: %v\n%s", err, applyOut)
	}
	applyFacts := applyEnvelope["facts"].(map[string]any)
	if applyEnvelope["command"] != "organize.apply" || applyFacts["plan_id"] != planID || applyFacts["applied_moves"] != "1" || applyFacts["applied_metadata"] != "2" {
		t.Fatalf("organize apply envelope = %#v", applyEnvelope)
	}
	target := filepath.Join(root, "notes", "research-idea.md")
	if fileExists(source) || !fileExists(target) {
		t.Fatalf("organize apply did not move source into notes/")
	}
	targetContent := readCLIFile(t, target)
	for _, want := range []string{"tags: [research]", "status: active", "# Research Idea"} {
		if !strings.Contains(targetContent, want) {
			t.Fatalf("organize apply did not apply metadata %q:\n%s", want, targetContent)
		}
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, ".pinax", "events.jsonl")), "organize.apply") {
		t.Fatalf("organize apply did not append event evidence")
	}

	staleRoot := t.TempDir()
	runCLI(t, "init", staleRoot, "--title", "Vault", "--json")
	staleSource := filepath.Join(staleRoot, "Stale Note.md")
	writeCLIFixture(t, staleSource, pinaxNoteFixture("note_stale", "Stale Note", "[]", "body\n"))
	staleOut := runCLI(t, "organize", "suggest", "--vault", staleRoot, "--save", "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("stale organize suggest json invalid: %v\n%s", err, staleOut)
	}
	stalePlanID := staleEnvelope["facts"].(map[string]any)["plan_id"].(string)
	writeCLIFixture(t, staleSource, pinaxNoteFixture("note_stale", "Stale Note", "[]", "changed\n"))
	failed, err = runCLIExpectError("organize", "apply", "--vault", staleRoot, "--plan", stalePlanID, "--yes", "--snapshot-message", "整理前快照", "--json")
	if err == nil || !strings.Contains(failed, "plan_stale") || !strings.Contains(failed, "pinax organize plan") {
		t.Fatalf("stale organize apply err=%v out=%s", err, failed)
	}
}

func TestDailyInboxWorkflowCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	editorLog := filepath.Join(root, "daily-editor.log")
	editor := writeFakeEditor(t, root, editorLog)
	date := time.Now().UTC().Format("2006-01-02")
	dailyRel := filepath.ToSlash(filepath.Join("daily", date+".md"))

	openOut := runCLI(t, "daily", "open", "--editor", editor, "--vault", root, "--json")
	var openEnvelope map[string]any
	if err := json.Unmarshal([]byte(openOut), &openEnvelope); err != nil {
		t.Fatalf("daily open json invalid: %v\n%s", err, openOut)
	}
	openFacts := openEnvelope["facts"].(map[string]any)
	if openEnvelope["command"] != "daily.open" || openFacts["path"] != dailyRel || openFacts["date"] != date || openFacts["editor_executable"] != editor {
		t.Fatalf("daily open envelope = %#v", openEnvelope)
	}
	if !strings.Contains(readCLIFile(t, editorLog), dailyRel) {
		t.Fatalf("daily open did not invoke editor with daily path")
	}

	showOut := runCLI(t, "daily", "show", "--vault", root, "--json")
	if !strings.Contains(showOut, "daily.show") || !strings.Contains(showOut, dailyRel) {
		t.Fatalf("daily show output invalid:\n%s", showOut)
	}
	appendOut := runCLI(t, "daily", "append", "--body", "复盘", "--vault", root, "--json")
	if !strings.Contains(appendOut, "daily.append") || !strings.Contains(appendOut, "index_updated") {
		t.Fatalf("daily append output invalid:\n%s", appendOut)
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, filepath.FromSlash(dailyRel))), "复盘") {
		t.Fatalf("daily append did not write body")
	}

	journalDate := "2026-06-06"
	dailyDatedRel := filepath.ToSlash(filepath.Join("daily", journalDate+".md"))
	dailyDatedOut := runCLI(t, "daily", "show", "--date", journalDate, "--vault", root, "--json")
	if !strings.Contains(dailyDatedOut, dailyDatedRel) || !strings.Contains(dailyDatedOut, `"period":"daily"`) {
		t.Fatalf("daily show --date output invalid:\n%s", dailyDatedOut)
	}
	dailyNextOut := runCLI(t, "daily", "show", "--date", journalDate, "--next", "--vault", root, "--json")
	if !strings.Contains(dailyNextOut, "daily/2026-06-07.md") || !strings.Contains(dailyNextOut, `"date":"2026-06-07"`) {
		t.Fatalf("daily show --next output invalid:\n%s", dailyNextOut)
	}

	weeklyOut := runCLI(t, "weekly", "show", "--date", journalDate, "--vault", root, "--json")
	if !strings.Contains(weeklyOut, "weekly.show") || !strings.Contains(weeklyOut, "weekly/2026-W23.md") || !strings.Contains(weeklyOut, `"period":"weekly"`) {
		t.Fatalf("weekly show output invalid:\n%s", weeklyOut)
	}
	weeklyKeyOut := runCLI(t, "weekly", "show", "--date", "2026-W23", "--vault", root, "--json")
	if !strings.Contains(weeklyKeyOut, "weekly/2026-W23.md") || !strings.Contains(weeklyKeyOut, `"date":"2026-W23"`) {
		t.Fatalf("weekly show should accept completion key:\n%s", weeklyKeyOut)
	}
	monthlyOut := runCLI(t, "monthly", "show", "--date", journalDate, "--vault", root)
	for _, want := range []string{"Highlights", "monthly/2026-06.md", "Period", "monthly", "Monthly 2026-06"} {
		if !strings.Contains(monthlyOut, want) {
			t.Fatalf("monthly show human output missing %q:\n%s", want, monthlyOut)
		}
	}
	monthlyKeyOut := runCLI(t, "monthly", "show", "--date", "2026-06", "--vault", root, "--json")
	if !strings.Contains(monthlyKeyOut, "monthly/2026-06.md") || !strings.Contains(monthlyKeyOut, `"date":"2026-06"`) {
		t.Fatalf("monthly show should accept completion key:\n%s", monthlyKeyOut)
	}

	captureOut := runCLI(t, "inbox", "capture", "Inbox Idea", "--body", "body", "--tags", "idea", "--vault", root, "--json")
	var captureEnvelope map[string]any
	if err := json.Unmarshal([]byte(captureOut), &captureEnvelope); err != nil {
		t.Fatalf("inbox capture json invalid: %v\n%s", err, captureOut)
	}
	captureFacts := captureEnvelope["facts"].(map[string]any)
	inboxPath := captureFacts["path"].(string)
	if captureEnvelope["command"] != "inbox.capture" || !strings.HasPrefix(inboxPath, "inbox/") || captureFacts["kind"] != "inbox" || captureFacts["status"] != "inbox" {
		t.Fatalf("inbox capture envelope = %#v", captureEnvelope)
	}
	inboxContent := readCLIFile(t, filepath.Join(root, filepath.FromSlash(inboxPath)))
	for _, want := range []string{"kind: inbox", "status: inbox", "body"} {
		if !strings.Contains(inboxContent, want) {
			t.Fatalf("inbox content missing %q:\n%s", want, inboxContent)
		}
	}

	listOut := runCLI(t, "inbox", "list", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("inbox list json invalid: %v\n%s", err, listOut)
	}
	if listEnvelope["command"] != "inbox.list" || listEnvelope["facts"].(map[string]any)["returned"] != "1" {
		t.Fatalf("inbox list envelope = %#v", listEnvelope)
	}

	triageOut := runCLI(t, "inbox", "triage", inboxPath, "--group", "work", "--folder", "ideas", "--kind", "reference", "--status", "active", "--vault", root, "--json")
	var triageEnvelope map[string]any
	if err := json.Unmarshal([]byte(triageOut), &triageEnvelope); err != nil {
		t.Fatalf("inbox triage json invalid: %v\n%s", err, triageOut)
	}
	triageFacts := triageEnvelope["facts"].(map[string]any)
	triagedPath := "notes/work/ideas/inbox-idea.md"
	if triageEnvelope["command"] != "inbox.triage" || triageFacts["path"] != triagedPath || triageFacts["status"] != "active" || triageFacts["kind"] != "reference" {
		t.Fatalf("inbox triage envelope = %#v", triageEnvelope)
	}
	triagedContent := readCLIFile(t, filepath.Join(root, filepath.FromSlash(triagedPath)))
	for _, want := range []string{"project: work", "folder: ideas", "kind: reference", "status: active"} {
		if !strings.Contains(triagedContent, want) {
			t.Fatalf("triaged note missing %q:\n%s", want, triagedContent)
		}
	}
}

func TestDatabaseSchemaAndViewRegistryV2CLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "A", "--body", "priority:: 2", "--status", "active", "--vault", root, "--json")
	inferOut := runCLI(t, "database", "schema", "infer", "--vault", root, "--json")
	var inferEnvelope map[string]any
	if err := json.Unmarshal([]byte(inferOut), &inferEnvelope); err != nil {
		t.Fatalf("schema infer json invalid: %v\n%s", err, inferOut)
	}
	if inferEnvelope["command"] != "database.schema.infer" || inferEnvelope["facts"].(map[string]any)["properties"] == "0" || !strings.Contains(inferOut, "priority") {
		t.Fatalf("schema infer envelope = %#v", inferEnvelope)
	}
	if fileExists(filepath.Join(root, ".pinax", "schema-overrides.json")) {
		t.Fatalf("schema infer wrote overrides asset")
	}
	setOut := runCLI(t, "database", "schema", "set", "status", "--type", "select", "--values", "active,done", "--vault", root, "--json")
	if !strings.Contains(setOut, "database.schema.set") || !strings.Contains(readCLIFile(t, filepath.Join(root, ".pinax", "schema-overrides.json")), "status") {
		t.Fatalf("schema set failed:\n%s", setOut)
	}
	viewOut := runCLI(t, "database", "view", "save", "sql-active", "--query", "SELECT title FROM notes LIMIT 20", "--kind", "table", "--column", "title", "--vault", root, "--json")
	if !strings.Contains(viewOut, "database.view.save") {
		t.Fatalf("database query view save output = %s", viewOut)
	}
	views := readCLIFile(t, filepath.Join(root, ".pinax", "views.json"))
	for _, want := range []string{"pinax.views.v2", "sql-active", "SELECT title FROM notes LIMIT 20", "columns"} {
		if !strings.Contains(views, want) {
			t.Fatalf("views registry missing %q:\n%s", want, views)
		}
	}
}

func TestNoteListPropertyOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "A", "--body", "priority:: 2", "--status", "active", "--vault", root, "--json")
	out := runCLI(t, "note", "list", "--property", "priority", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("note list property json invalid: %v\n%s", err, out)
	}
	if envelope["facts"].(map[string]any)["properties"] != "priority" || !strings.Contains(out, "priority") {
		t.Fatalf("note list property envelope = %#v", envelope)
	}
	failed, err := runCLIExpectError("note", "list", "--property", "missing", "--strict-properties", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "property_not_found") {
		t.Fatalf("strict property err=%v out=%s", err, failed)
	}
}

func TestQueryRunOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Active", "--body", "priority:: 2", "--status", "active", "--tags", "pinax", "--vault", root, "--json")
	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("remove index: %v", err)
	}
	failed, err := runCLIExpectError("query", "run", `SELECT title FROM notes LIMIT 5`, "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "index_required") {
		t.Fatalf("query run without index err=%v out=%s", err, failed)
	}
	out := runCLI(t, "query", "run", `SELECT title, status FROM notes WHERE status = "active" LIMIT 5`, "--lazy-index", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("query run json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "query.run" || facts["rows"] != "1" || facts["columns"] != "title,status" || facts["index_loaded"] != "lazy_rebuild" {
		t.Fatalf("query run envelope = %#v", envelope)
	}
}

func TestQueryExplainOutputContract(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out := runCLI(t, "query", "explain", `SELECT title, status FROM notes WHERE status = "active" LIMIT 10`, "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("query explain json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "query.explain" || envelope["mode"] != "json" || facts["source"] != "notes" || facts["columns"] != "title,status" || facts["limit"] != "10" {
		t.Fatalf("query explain envelope = %#v", envelope)
	}
	agent := runCLI(t, "query", "explain", "SELECT title FROM notes LIMIT 5", "--vault", root, "--agent")
	for _, want := range []string{"command=query.explain", "fact.source=notes", "fact.columns=title", "fact.limit=5"} {
		if !strings.Contains(agent, want) {
			t.Fatalf("query explain agent missing %q:\n%s", want, agent)
		}
	}
}

func TestDatabaseViewQueryCompletionAndHelp(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Active Note", "--body", "body", "--tags", "pinax", "--kind", "reference", "--status", "active", "--vault", root, "--json")
	runCLI(t, "view", "save", "active-notes", "--status", "active", "--vault", root, "--json")

	viewCompletion := runCLI(t, "__complete", "view", "show", "--vault", root, "")
	for _, want := range []string{"active-notes\tview", "ShellCompDirectiveNoFileComp"} {
		if !strings.Contains(viewCompletion, want) {
			t.Fatalf("view show completion missing %q:\n%s", want, viewCompletion)
		}
	}

	databaseViewCompletion := runCLI(t, "__complete", "database", "view", "show", "--vault", root, "")
	if !strings.Contains(databaseViewCompletion, "active-notes\tview") || !strings.Contains(databaseViewCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("database view completion = %s", databaseViewCompletion)
	}

	noteCompletion := runCLI(t, "__complete", "note", "show", "--vault", root, "")
	if !strings.Contains(noteCompletion, "Active Note\tnote") || !strings.Contains(noteCompletion, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("note show completion = %s", noteCompletion)
	}

	searchSortCompletion := runCLI(t, "__complete", "search", "query", "--vault", root, "--sort", "")
	if !strings.Contains(searchSortCompletion, "relevance\tsort") || !strings.Contains(searchSortCompletion, "updated\tsort") {
		t.Fatalf("search sort completion = %s", searchSortCompletion)
	}

	noteStatusCompletion := runCLI(t, "__complete", "note", "list", "--vault", root, "--status", "")
	if !strings.Contains(noteStatusCompletion, "active\tstatus") || !strings.Contains(noteStatusCompletion, "done\tstatus") {
		t.Fatalf("note status completion = %s", noteStatusCompletion)
	}

	queryRunSortCompletion := runCLI(t, "__complete", "query", "run", "SELECT title FROM notes", "--vault", root, "--sort", "")
	if !strings.Contains(queryRunSortCompletion, "title\tproperty") || !strings.Contains(queryRunSortCompletion, "updated_at\tproperty") {
		t.Fatalf("query run sort completion = %s", queryRunSortCompletion)
	}

	schemaTypeCompletion := runCLI(t, "__complete", "database", "schema", "set", "status", "--vault", root, "--type", "")
	if !strings.Contains(schemaTypeCompletion, "select\ttype") || !strings.Contains(schemaTypeCompletion, "date\ttype") {
		t.Fatalf("database schema type completion = %s", schemaTypeCompletion)
	}

	help := runCLI(t, "query", "--help")
	for _, want := range []string{"index status", "query explain", "query run", "database view save"} {
		if !strings.Contains(help, want) {
			t.Fatalf("query help missing %q:\n%s", want, help)
		}
	}
}

func TestJournalDateCompletionCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-04.md"), "# Daily 2026-06-04\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-06.md"), "# Daily 2026-06-06\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-05.md"), "# Daily 2026-06-05\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "weekly", "2026-W22.md"), "# Weekly 2026-W22\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "weekly", "2026-W23.md"), "# Weekly 2026-W23\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "monthly", "2026-06.md"), "# Monthly 2026-06\n")

	daily := runCLI(t, "__complete", "daily", "show", "--vault", root, "--date", "")
	for _, want := range []string{"\t", "ShellCompDirectiveNoFileComp"} {
		if !strings.Contains(daily, want) {
			t.Fatalf("daily date completion missing %q:\n%s", want, daily)
		}
	}
	dailyLines := completionValueLines(daily)
	if strings.Join(dailyLines, ",") != "2026-06-06,2026-06-05,2026-06-04" {
		t.Fatalf("daily completion should be newest first:\n%s", daily)
	}
	if strings.Contains(daily, "2026-06-03") {
		t.Fatalf("daily completion should not include missing journals:\n%s", daily)
	}

	weekly := runCLI(t, "__complete", "weekly", "show", "--vault", root, "--date", "")
	for _, want := range []string{"week", "(", "--", ")"} {
		if !strings.Contains(weekly, want) {
			t.Fatalf("weekly date completion missing %q:\n%s", want, weekly)
		}
	}
	weeklyLines := completionValueLines(weekly)
	if strings.Join(weeklyLines, ",") != "2026-W23,2026-W22" {
		t.Fatalf("weekly completion should be newest first:\n%s", weekly)
	}
	if strings.Contains(weekly, "2026-W21") {
		t.Fatalf("weekly completion should not include missing journals:\n%s", weekly)
	}

	monthly := runCLI(t, "__complete", "monthly", "show", "--vault", root, "--date", "")
	if !strings.Contains(monthly, "--") || !strings.Contains(monthly, "\t") {
		t.Fatalf("monthly date completion should include date range descriptions:\n%s", monthly)
	}
	monthlyLines := completionValueLines(monthly)
	if strings.Join(monthlyLines, ",") != "2026-06" || strings.Contains(monthly, "2026-05") {
		t.Fatalf("monthly completion should only include existing journals:\n%s", monthly)
	}
}

func TestNotebookOrganizationViewsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "note", "new", "架构笔记", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--tags", "auth,design", "--body", "正文", "--slug", "arch-note", "--vault", root, "--json")
	runCLI(t, "note", "new", "草稿笔记", "--folder", "drafts", "--kind", "fleeting", "--status", "draft", "--tags", "draft", "--body", "草稿", "--slug", "draft-note", "--vault", root, "--json")

	listOut := runCLI(t, "note", "list", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--created-after", "2000-01-01", "--updated-before", "2999-01-01", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("org note list json invalid: %v\n%s", err, listOut)
	}
	listFacts := listEnvelope["facts"].(map[string]any)
	for _, key := range []string{"filter.group", "filter.folder", "filter.kind", "filter.status", "filter.created_after", "filter.updated_before"} {
		if listFacts[key] == "" {
			t.Fatalf("note list facts missing %s: %#v", key, listFacts)
		}
	}
	if listFacts["returned"] != "1" || !strings.Contains(listOut, "架构笔记") || strings.Contains(listOut, "草稿笔记") {
		t.Fatalf("org note list output invalid facts=%#v out=%s", listFacts, listOut)
	}

	for _, tc := range []struct {
		args []string
		cmd  string
		want string
	}{
		{args: []string{"tag", "list", "--vault", root, "--json"}, cmd: "tag.list", want: "auth"},
		{args: []string{"kind", "list", "--vault", root, "--json"}, cmd: "kind.list", want: "reference"},
		{args: []string{"group", "list", "--vault", root, "--json"}, cmd: "group.list", want: "work"},
	} {
		out := runCLI(t, tc.args...)
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("%s json invalid: %v\n%s", tc.cmd, err, out)
		}
		facts := envelope["facts"].(map[string]any)
		if envelope["command"] != tc.cmd || facts["dimensions"] == "" || facts["notes"] == "" || !strings.Contains(out, tc.want) {
			t.Fatalf("%s output invalid facts=%#v out=%s", tc.cmd, facts, out)
		}
	}
	folderOut := runCLI(t, "folder", "list", "--vault", root, "--json")
	var folderEnvelope map[string]any
	if err := json.Unmarshal([]byte(folderOut), &folderEnvelope); err != nil {
		t.Fatalf("folder list json invalid: %v\n%s", err, folderOut)
	}
	folderFacts := folderEnvelope["facts"].(map[string]any)
	if folderEnvelope["command"] != "folder.list" || folderFacts["folders"] == "" || !strings.Contains(folderOut, "architecture") {
		t.Fatalf("folder list output invalid facts=%#v out=%s", folderFacts, folderOut)
	}
}

func TestDailyNoteCompletionUsesShellFriendlyTitle(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "daily", "2026-06-09.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_daily_legacy\ntitle: Daily 2026-06-09\ntags: [daily]\nfolder: daily\nkind: daily\nstatus: journal\n---\n\n# 2026-06-09\n")

	completion := runCLI(t, "__complete", "note", "show", "--vault", root, "Daily")
	if !strings.Contains(completion, "Daily-2026-06-09\tnote") || strings.Contains(completion, "Daily 2026-06-09") {
		t.Fatalf("daily note completion should use shell-friendly title:\n%s", completion)
	}
	shown := runCLI(t, "note", "show", "Daily-2026-06-09", "--vault", root, "--json")
	if !strings.Contains(shown, `"path":"daily/2026-06-09.md"`) {
		t.Fatalf("daily shell-friendly alias should resolve to the journal note:\n%s", shown)
	}
}

func TestSavedViewsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "note", "new", "工作参考", "--group", "work", "--folder", "refs", "--kind", "reference", "--status", "active", "--tags", "work", "--body", "正文", "--slug", "work-ref", "--vault", root, "--json")
	runCLI(t, "note", "new", "个人草稿", "--folder", "drafts", "--kind", "fleeting", "--status", "draft", "--tags", "personal", "--body", "正文", "--slug", "personal-draft", "--vault", root, "--json")

	saveOut := runCLI(t, "view", "save", "active-work", "--group", "work", "--status", "active", "--kind", "reference", "--sort", "title", "--vault", root, "--json")
	var saveEnvelope map[string]any
	if err := json.Unmarshal([]byte(saveOut), &saveEnvelope); err != nil {
		t.Fatalf("view save json invalid: %v\n%s", err, saveOut)
	}
	if saveEnvelope["command"] != "view.save" || saveEnvelope["facts"].(map[string]any)["view"] != "active-work" {
		t.Fatalf("view save envelope = %#v", saveEnvelope)
	}
	viewsPath := filepath.Join(root, ".pinax", "views.json")
	viewsContent := readCLIFile(t, viewsPath)
	if !strings.Contains(viewsContent, "active-work") || !strings.Contains(viewsContent, "pinax.views.v1") {
		t.Fatalf("views asset invalid:\n%s", viewsContent)
	}

	listOut := runCLI(t, "view", "list", "--vault", root, "--json")
	if !strings.Contains(listOut, "view.list") || !strings.Contains(listOut, "active-work") {
		t.Fatalf("view list output invalid:\n%s", listOut)
	}
	showOut := runCLI(t, "view", "show", "active-work", "--vault", root, "--json")
	var showEnvelope map[string]any
	if err := json.Unmarshal([]byte(showOut), &showEnvelope); err != nil {
		t.Fatalf("view show json invalid: %v\n%s", err, showOut)
	}
	showFacts := showEnvelope["facts"].(map[string]any)
	if showEnvelope["command"] != "view.show" || showFacts["view"] != "active-work" || showFacts["returned"] != "1" || !strings.Contains(showOut, "工作参考") || strings.Contains(showOut, "个人草稿") {
		t.Fatalf("view show envelope = %#v out=%s", showEnvelope, showOut)
	}

	dbSaveOut := runCLI(t, "database", "view", "save", "db-active", "--group", "work", "--status", "active", "--kind", "reference", "--sort", "title", "--vault", root, "--json")
	var dbSaveEnvelope map[string]any
	if err := json.Unmarshal([]byte(dbSaveOut), &dbSaveEnvelope); err != nil {
		t.Fatalf("database view save json invalid: %v\n%s", err, dbSaveOut)
	}
	if dbSaveEnvelope["command"] != "database.view.save" || dbSaveEnvelope["facts"].(map[string]any)["view"] != "db-active" {
		t.Fatalf("database view save envelope = %#v", dbSaveEnvelope)
	}

	dbListOut := runCLI(t, "database", "view", "list", "--vault", root, "--agent")
	for _, want := range []string{"command=database.view.list", "fact.views=2"} {
		if !strings.Contains(dbListOut, want) {
			t.Fatalf("database view list missing %q:\n%s", want, dbListOut)
		}
	}

	dbShowOut := runCLI(t, "database", "view", "show", "db-active", "--vault", root, "--json")
	var dbShowEnvelope map[string]any
	if err := json.Unmarshal([]byte(dbShowOut), &dbShowEnvelope); err != nil {
		t.Fatalf("database view show json invalid: %v\n%s", err, dbShowOut)
	}
	if dbShowEnvelope["command"] != "database.view.show" || dbShowEnvelope["facts"].(map[string]any)["returned"] != "1" || !strings.Contains(dbShowOut, "工作参考") {
		t.Fatalf("database view show envelope = %#v out=%s", dbShowEnvelope, dbShowOut)
	}

	dbDeleteFailed, err := runCLIExpectError("database", "view", "delete", "db-active", "--vault", root, "--json")
	if err == nil || !strings.Contains(dbDeleteFailed, "approval_required") {
		t.Fatalf("database view delete without approval err=%v out=%s", err, dbDeleteFailed)
	}
	dbDeleteOut := runCLI(t, "database", "view", "delete", "db-active", "--yes", "--vault", root, "--json")
	if !strings.Contains(dbDeleteOut, "database.view.delete") || strings.Contains(readCLIFile(t, viewsPath), "db-active") {
		t.Fatalf("database view delete failed:\n%s\nviews=%s", dbDeleteOut, readCLIFile(t, viewsPath))
	}

	failed, err := runCLIExpectError("view", "delete", "active-work", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("view delete without approval err=%v out=%s", err, failed)
	}
	deleteOut := runCLI(t, "view", "delete", "active-work", "--yes", "--vault", root, "--json")
	if !strings.Contains(deleteOut, "view.delete") || strings.Contains(readCLIFile(t, viewsPath), "active-work") {
		t.Fatalf("view delete failed:\n%s\nviews=%s", deleteOut, readCLIFile(t, viewsPath))
	}
	if !fileExists(filepath.Join(root, "notes", "work", "refs", "work-ref.md")) {
		t.Fatalf("view delete removed note")
	}
}

func TestNoteLinkGraphCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nSee [[Beta]] and [[Missing Target]].\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nkind: reference\n---\n\n# Beta\n\nLinked by Alpha.\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "gamma.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_gamma\ntitle: Gamma\nkind: reference\n---\n\n# Gamma\n\nNo graph edges.\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "daily", "2026-06-06.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_daily_index\ntitle: Daily Index\nkind: index\n---\n\n# Daily Index\n\nSystem index note.\n")

	linksOut := runCLI(t, "note", "links", "Alpha", "--vault", root, "--json")
	var linksEnvelope map[string]any
	if err := json.Unmarshal([]byte(linksOut), &linksEnvelope); err != nil {
		t.Fatalf("note links json invalid: %v\n%s", err, linksOut)
	}
	linksFacts := linksEnvelope["facts"].(map[string]any)
	if linksEnvelope["command"] != "note.links" || linksFacts["links"] != "2" || linksFacts["resolved"] != "1" || linksFacts["broken"] != "1" || !strings.Contains(linksOut, "notes/beta.md") || !strings.Contains(linksOut, "Missing Target") {
		t.Fatalf("note links envelope = %#v out=%s", linksEnvelope, linksOut)
	}

	backlinksOut := runCLI(t, "note", "backlinks", "Beta", "--vault", root, "--json")
	var backlinksEnvelope map[string]any
	if err := json.Unmarshal([]byte(backlinksOut), &backlinksEnvelope); err != nil {
		t.Fatalf("note backlinks json invalid: %v\n%s", err, backlinksOut)
	}
	backlinksFacts := backlinksEnvelope["facts"].(map[string]any)
	if backlinksEnvelope["command"] != "note.backlinks" || backlinksFacts["backlinks"] != "1" || backlinksFacts["unresolved"] != "0" || !strings.Contains(backlinksOut, "notes/alpha.md") {
		t.Fatalf("note backlinks envelope = %#v out=%s", backlinksEnvelope, backlinksOut)
	}

	orphansOut := runCLI(t, "note", "orphans", "--vault", root, "--json")
	var orphansEnvelope map[string]any
	if err := json.Unmarshal([]byte(orphansOut), &orphansEnvelope); err != nil {
		t.Fatalf("note orphans json invalid: %v\n%s", err, orphansOut)
	}
	orphansFacts := orphansEnvelope["facts"].(map[string]any)
	if orphansEnvelope["command"] != "note.orphans" || orphansFacts["orphans"] != "1" || !strings.Contains(orphansOut, "notes/gamma.md") || strings.Contains(orphansOut, "Daily Index") {
		t.Fatalf("note orphans envelope = %#v out=%s", orphansEnvelope, orphansOut)
	}
	orphansHuman := runCLI(t, "note", "orphans", "--vault", root)
	for _, want := range []string{"Highlights", "Orphan notes listed.", "Path", "Title", "Kind", "notes/gamma.md", "Gamma", "Reference"} {
		if !strings.Contains(orphansHuman, want) {
			t.Fatalf("note orphans human output missing %q:\n%s", want, orphansHuman)
		}
	}
	for _, old := range []string{"状态:", "重点:", "事实:"} {
		if strings.Contains(orphansHuman, old) {
			t.Fatalf("note orphans human output still uses old prose %q:\n%s", old, orphansHuman)
		}
	}
}

func TestSearchLinkTargetCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nSource mentions [[Beta]] and [[Missing Target]].\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nkind: reference\n---\n\n# Beta\n\nTarget note.\n")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")

	for _, target := range []string{"Beta", "notes/beta.md", "note_beta", "Missing Target"} {
		out := runCLI(t, "search", "Source", "--link-target", target, "--vault", root, "--json")
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("search link target json invalid for %q: %v\n%s", target, err, out)
		}
		facts := envelope["facts"].(map[string]any)
		resultPath := firstSearchResultPath(envelope)
		if envelope["command"] != "note.search" || facts["filter.link_target"] != target || facts["returned"] != "1" || resultPath != "notes/alpha.md" {
			t.Fatalf("search link target %q envelope = %#v out=%s", target, envelope, out)
		}
	}

	failed, err := runCLIExpectError("search", "Source", "--link-target", "   ", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "invalid_link_filter") {
		t.Fatalf("invalid link filter err=%v out=%s", err, failed)
	}
}

func firstSearchResultPath(envelope map[string]any) string {
	data := envelope["data"].(map[string]any)
	if results, ok := data["results"].([]any); ok && len(results) == 1 {
		return results[0].(map[string]any)["note"].(map[string]any)["path"].(string)
	}
	if notes, ok := data["notes"].([]any); ok && len(notes) == 1 {
		return notes[0].(map[string]any)["path"].(string)
	}
	return ""
}

func TestSearchLinkTargetAmbiguousCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "Source.md"), pinaxNoteFixture("note_source", "Source", "[]", "Source links to [[Shared]].\n"))
	writeCLIFixture(t, filepath.Join(root, "First.md"), pinaxNoteFixture("note_first", "Shared", "[]", "first\n"))
	writeCLIFixture(t, filepath.Join(root, "Second.md"), pinaxNoteFixture("note_second", "Shared", "[]", "second\n"))

	failed, err := runCLIExpectError("search", "Source", "--link-target", "Shared", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "link_target_ambiguous") || !strings.Contains(failed, "First.md") || !strings.Contains(failed, "Second.md") {
		t.Fatalf("ambiguous link target err=%v out=%s", err, failed)
	}
}

func TestLinkOutputContractModes(t *testing.T) {
	root := linkOutputFixture(t)
	out := runCLI(t, "note", "links", "Alpha", "--vault", root, "--json")
	assertMachineOutputClean(t, out)
	if strings.Contains(out, "secret-token") || strings.Contains(out, "raw prompt") {
		t.Fatalf("link json leaked note body or secret:\n%s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("link json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "note.links" || envelope["mode"] != "json" || facts["links"] != "2" || facts["broken"] != "1" || facts["engine"] == "" {
		t.Fatalf("link json envelope = %#v", envelope)
	}
}

func TestBacklinkOutputContractAgent(t *testing.T) {
	root := linkOutputFixture(t)
	out := runCLI(t, "note", "backlinks", "Beta", "--vault", root, "--agent")
	assertMachineOutputClean(t, out)
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=note.backlinks", "status=success", "fact.backlinks=1", "fact.engine="} {
		if !strings.Contains(out, want) {
			t.Fatalf("backlink agent missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret-token") || strings.Contains(out, "raw prompt") {
		t.Fatalf("backlink agent leaked note body or secret:\n%s", out)
	}
}

func TestOrphanOutputContractEvents(t *testing.T) {
	root := linkOutputFixture(t)
	out := runCLI(t, "note", "orphans", "--vault", root, "--events")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("orphan events lines = %q", out)
	}
	for i, line := range lines {
		assertMachineOutputClean(t, line)
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("orphan event %d invalid: %v\n%s", i, err, out)
		}
		if event["command"] != "note.orphans" || event["mode"] != "events" {
			t.Fatalf("orphan event %d payload = %#v", i, event)
		}
	}
	if !strings.Contains(lines[0], `"type":"start"`) || !strings.Contains(lines[1], `"type":"end"`) || !strings.Contains(lines[1], `"orphans":"1"`) {
		t.Fatalf("orphan events missing start/end facts:\n%s", out)
	}
}

func TestGraphExplainOutputContract(t *testing.T) {
	root := linkOutputFixture(t)
	out := runCLI(t, "note", "links", "Alpha", "--vault", root, "--explain")
	for _, want := range []string{"Conclusion:", "Evidence:", "Confidence:", "Recommended next step:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("graph explain missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret-token") || strings.Contains(out, "raw prompt") || strings.Contains(out, "system prompt") {
		t.Fatalf("graph explain leaked sensitive body text:\n%s", out)
	}
}

func TestStdoutStderrLinkOutputContract(t *testing.T) {
	root := linkOutputFixture(t)
	stdout, stderr, err := runCLISeparate("note", "links", "Alpha", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("link json stdout/stderr err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("link stdout json invalid: %v\n%s", err, stdout)
	}
	stdout, stderr, err = runCLISeparate("note", "links", "Missing", "--vault", root, "--json")
	if err == nil || stderr != "" {
		t.Fatalf("link error stdout/stderr err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("link error stdout json invalid: %v\n%s", err, stdout)
	}
	errorObject := envelope["error"].(map[string]any)
	if envelope["status"] != "failed" || errorObject["code"] != "note_not_found" {
		t.Fatalf("link error envelope = %#v", envelope)
	}
}

func linkOutputFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nSee [[Beta]] and [[Missing Target]].\n\nsecret-token raw prompt system prompt\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "beta.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_beta\ntitle: Beta\nkind: reference\n---\n\n# Beta\n\nLinked by Alpha.\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "gamma.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_gamma\ntitle: Gamma\nkind: reference\n---\n\n# Gamma\n\nNo graph edges.\n")
	return root
}

func assertMachineOutputClean(t *testing.T, out string) {
	t.Helper()
	for _, forbidden := range []string{"\x1b[", "状态", "重点", "事实:"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("machine output contains %q:\n%s", forbidden, out)
		}
	}
}

func TestNoteAttachPlacementLinkStyleAndModesCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nBody.\n")
	sourceDir := t.TempDir()
	diagram := filepath.Join(sourceDir, "diagram.png")
	writeCLIFixture(t, diagram, "png-bytes")

	attachOut := runCLI(t, "note", "attach", "Alpha", diagram, "--placement", "note-folder", "--embed", "--rename", "renamed.png", "--vault", root, "--json")
	var attachEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachOut), &attachEnvelope); err != nil {
		t.Fatalf("note attach placement json invalid: %v\n%s", err, attachOut)
	}
	facts := attachEnvelope["facts"].(map[string]any)
	if attachEnvelope["command"] != "note.attach" || facts["attachment_path"] != "notes/assets/renamed.png" || facts["placement"] != "note-folder" || facts["link_style"] != "markdown" || facts["mode"] != "copy" || facts["reference"] != "![renamed.png](assets/renamed.png)" {
		t.Fatalf("placement attach envelope = %#v", attachEnvelope)
	}
	if !fileExists(filepath.Join(root, "notes", "assets", "renamed.png")) || !strings.Contains(readCLIFile(t, filepath.Join(root, "notes", "alpha.md")), "![renamed.png](assets/renamed.png)") {
		t.Fatalf("note-folder attach did not write expected file/reference")
	}

	before := readCLIFile(t, filepath.Join(root, "notes", "alpha.md"))
	moveSource := filepath.Join(sourceDir, "move.pdf")
	writeCLIFixture(t, moveSource, "move-pdf")
	failed, err := runCLIExpectError("note", "attach", "Alpha", moveSource, "--mode", "move", "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "approval_required") {
		t.Fatalf("move without approval err=%v out=%s", err, failed)
	}
	if !fileExists(moveSource) || readCLIFile(t, filepath.Join(root, "notes", "alpha.md")) != before {
		t.Fatalf("move without approval modified source or note")
	}

	registeredPath := filepath.Join(root, "notes", "assets", "spec.pdf")
	writeCLIFixture(t, registeredPath, "pdf-bytes")
	registerOut := runCLI(t, "note", "attach", "Alpha", registeredPath, "--mode", "register", "--link-style", "wiki", "--vault", root, "--json")
	var registerEnvelope map[string]any
	if err := json.Unmarshal([]byte(registerOut), &registerEnvelope); err != nil {
		t.Fatalf("note attach register json invalid: %v\n%s", err, registerOut)
	}
	registerFacts := registerEnvelope["facts"].(map[string]any)
	if registerFacts["attachment_path"] != "notes/assets/spec.pdf" || registerFacts["mode"] != "register" || registerFacts["link_style"] != "wiki" || registerFacts["reference"] != "[[notes/assets/spec.pdf]]" {
		t.Fatalf("register attach envelope = %#v", registerEnvelope)
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, "notes", "alpha.md")), "[[notes/assets/spec.pdf]]") {
		t.Fatalf("register attach missing wiki reference")
	}
}

func TestNoteAttachmentCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_alpha\ntitle: Alpha\nkind: reference\n---\n\n# Alpha\n\nBody.\n")
	sourceDir := t.TempDir()
	source := filepath.Join(sourceDir, "diagram.png")
	writeCLIFixture(t, source, "png-bytes")

	attachOut := runCLI(t, "note", "attach", "Alpha", source, "--vault", root, "--json")
	var attachEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachOut), &attachEnvelope); err != nil {
		t.Fatalf("note attach json invalid: %v\n%s", err, attachOut)
	}
	attachFacts := attachEnvelope["facts"].(map[string]any)
	attachmentPath, _ := attachFacts["attachment_path"].(string)
	if attachEnvelope["command"] != "note.attach" || attachFacts["path"] != "notes/alpha.md" || attachmentPath == "" || !strings.HasPrefix(attachmentPath, "attachments/note_alpha/") {
		t.Fatalf("note attach envelope = %#v", attachEnvelope)
	}
	if !fileExists(filepath.Join(root, attachmentPath)) {
		t.Fatalf("attachment file missing at %s", attachmentPath)
	}
	alphaContent := readCLIFile(t, filepath.Join(root, "notes", "alpha.md"))
	if !strings.Contains(alphaContent, "![diagram.png](/"+attachmentPath+")") && !strings.Contains(alphaContent, "![diagram.png](../../"+attachmentPath+")") && !strings.Contains(alphaContent, "![diagram.png](../"+attachmentPath+")") {
		t.Fatalf("note body missing attachment reference:\n%s", alphaContent)
	}

	attachmentsOut := runCLI(t, "note", "attachments", "Alpha", "--vault", root, "--json")
	var attachmentsEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachmentsOut), &attachmentsEnvelope); err != nil {
		t.Fatalf("note attachments json invalid: %v\n%s", err, attachmentsOut)
	}
	attachmentsFacts := attachmentsEnvelope["facts"].(map[string]any)
	if attachmentsEnvelope["command"] != "note.attachments" || attachmentsFacts["attachments"] != "1" || attachmentsFacts["missing"] != "0" || !strings.Contains(attachmentsOut, attachmentPath) {
		t.Fatalf("note attachments envelope = %#v out=%s", attachmentsEnvelope, attachmentsOut)
	}

	before := readCLIFile(t, filepath.Join(root, "notes", "alpha.md"))
	failed, err := runCLIExpectError("note", "attach", "Alpha", filepath.Join(sourceDir, "missing.png"), "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "attachment_source_missing") {
		t.Fatalf("missing attachment err=%v out=%s", err, failed)
	}
	if after := readCLIFile(t, filepath.Join(root, "notes", "alpha.md")); after != before {
		t.Fatalf("missing source modified note:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestImportExportMarkdownCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "research", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: existing_alpha\ntitle: Existing Alpha\ntags: [imported]\nproject: research\n---\n\n# Existing Alpha\n")
	sourceDir := t.TempDir()
	writeCLIFixture(t, filepath.Join(sourceDir, "alpha.md"), "# Alpha\n\nImported alpha.\n")
	writeCLIFixture(t, filepath.Join(sourceDir, "beta.md"), "# Beta\n\nImported beta.\n")

	dryRunOut := runCLI(t, "import", "markdown", sourceDir, "--group", "research", "--tags", "imported", "--dry-run", "--vault", root, "--json")
	var dryRunEnvelope map[string]any
	if err := json.Unmarshal([]byte(dryRunOut), &dryRunEnvelope); err != nil {
		t.Fatalf("import dry-run json invalid: %v\n%s", err, dryRunOut)
	}
	dryFacts := dryRunEnvelope["facts"].(map[string]any)
	if dryRunEnvelope["command"] != "import.markdown" || dryFacts["dry_run"] != "true" || dryFacts["planned"] != "2" || dryFacts["written"] != "0" {
		t.Fatalf("import dry-run envelope = %#v", dryRunEnvelope)
	}
	if fileExists(filepath.Join(root, "notes", "research", "beta.md")) || fileExists(filepath.Join(root, ".pinax", "receipts")) {
		t.Fatalf("dry-run wrote vault assets")
	}

	importOut := runCLI(t, "import", "markdown", sourceDir, "--group", "research", "--tags", "imported", "--kind", "reference", "--status", "active", "--conflict", "rename", "--yes", "--vault", root, "--json")
	var importEnvelope map[string]any
	if err := json.Unmarshal([]byte(importOut), &importEnvelope); err != nil {
		t.Fatalf("import json invalid: %v\n%s", err, importOut)
	}
	importFacts := importEnvelope["facts"].(map[string]any)
	if importEnvelope["command"] != "import.markdown" || importFacts["written"] != "2" || importFacts["renamed"] != "1" || importFacts["receipt_path"] == "" {
		t.Fatalf("import envelope = %#v", importEnvelope)
	}
	if !fileExists(filepath.Join(root, "notes", "research", "alpha-2.md")) || !fileExists(filepath.Join(root, "notes", "research", "beta.md")) {
		t.Fatalf("imported files missing")
	}
	imported := readCLIFile(t, filepath.Join(root, "notes", "research", "beta.md"))
	for _, want := range []string{"schema_version: pinax.note.v1", "project: research", "kind: reference", "status: active", "tags: [imported]"} {
		if !strings.Contains(imported, want) {
			t.Fatalf("imported note missing %q:\n%s", want, imported)
		}
	}
	writeCLIFixture(t, filepath.Join(sourceDir, "beta.md"), "# Beta\n\nOverwritten beta.\n")
	overwriteOut := runCLI(t, "import", "markdown", filepath.Join(sourceDir, "beta.md"), "--group", "research", "--tags", "imported", "--conflict", "overwrite", "--yes", "--vault", root, "--json")
	var overwriteEnvelope map[string]any
	if err := json.Unmarshal([]byte(overwriteOut), &overwriteEnvelope); err != nil {
		t.Fatalf("overwrite import json invalid: %v\n%s", err, overwriteOut)
	}
	overwriteFacts := overwriteEnvelope["facts"].(map[string]any)
	if overwriteEnvelope["command"] != "import.markdown" || overwriteFacts["written"] != "1" || overwriteFacts["overwritten"] != "1" || !strings.Contains(readCLIFile(t, filepath.Join(root, "notes", "research", "beta.md")), "Overwritten beta") {
		t.Fatalf("overwrite import failed envelope=%#v", overwriteEnvelope)
	}

	asset := filepath.Join(sourceDir, "diagram.png")
	writeCLIFixture(t, asset, "png")
	attachOut := runCLI(t, "note", "attach", "Beta", asset, "--vault", root, "--json")
	var attachEnvelope map[string]any
	if err := json.Unmarshal([]byte(attachOut), &attachEnvelope); err != nil {
		t.Fatalf("attach json invalid: %v\n%s", err, attachOut)
	}
	attachmentPath := attachEnvelope["facts"].(map[string]any)["attachment_path"].(string)
	outDir := filepath.Join(t.TempDir(), "bundle")
	exportOut := runCLI(t, "export", "markdown", outDir, "--tag", "imported", "--vault", root, "--json")
	var exportEnvelope map[string]any
	if err := json.Unmarshal([]byte(exportOut), &exportEnvelope); err != nil {
		t.Fatalf("export json invalid: %v\n%s", err, exportOut)
	}
	exportFacts := exportEnvelope["facts"].(map[string]any)
	if exportEnvelope["command"] != "export.markdown" || exportFacts["notes"] == "0" || exportFacts["receipt_path"] == "" {
		t.Fatalf("export envelope = %#v", exportEnvelope)
	}
	if !fileExists(filepath.Join(outDir, "notes", "research", "beta.md")) || !fileExists(filepath.Join(outDir, attachmentPath)) {
		t.Fatalf("export missing note or attachment: out=%s attachment=%s", exportOut, attachmentPath)
	}
}

func TestNoteCreateBuildsNotebookInformationArchitecture(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")

	out := runCLI(t, "note", "new", "工具笔记", "--group", "work", "--folder", "inbox", "--kind", "reference", "--tags", "pinax,cli", "--body", "正文 #daily", "--slug", "tool-note", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("note json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["project"] != "work" || facts["group"] != "work" || facts["folder"] != "inbox" || facts["kind"] != "reference" || facts["index_updated"] != "true" || facts["daily_index"] == "" {
		t.Fatalf("note facts missing notebook IA: %#v", facts)
	}
	path := filepath.Join(root, "notes", "work", "inbox", "tool-note.md")
	content := readCLIFile(t, path)
	for _, want := range []string{"project: work", "folder: inbox", "kind: reference", "tags: [pinax, cli]", "正文 #daily"} {
		if !strings.Contains(content, want) {
			t.Fatalf("note content missing %q:\n%s", want, content)
		}
	}
	dailyIndexPath := filepath.Join(root, facts["daily_index"].(string))
	dailyIndex := readCLIFile(t, dailyIndexPath)
	for _, want := range []string{"notes/work/inbox/tool-note.md", "#pinax", "group=work", "folder=inbox", "kind=reference"} {
		if !strings.Contains(dailyIndex, want) {
			t.Fatalf("daily index missing %q:\n%s", want, dailyIndex)
		}
	}
	statsOut := runCLI(t, "stats", "--vault", root, "--json")
	var stats map[string]any
	if err := json.Unmarshal([]byte(statsOut), &stats); err != nil {
		t.Fatalf("stats json invalid: %v\n%s", err, statsOut)
	}
	statsFacts := stats["facts"].(map[string]any)
	if statsFacts["notes"] != "1" || statsFacts["index_status"] != "fresh" {
		t.Fatalf("index not fresh after note create: %#v", statsFacts)
	}
}

func TestIndexSearchDatabaseAndFiltersCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "工作", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "note", "new", "认证方案", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--tags", "auth,design", "--body", "OAuth 登录设计 [[Identity]]", "--slug", "auth-design", "--vault", root, "--json")

	initOut := runCLI(t, "index", "init", "--vault", root, "--json")
	var initEnvelope map[string]any
	if err := json.Unmarshal([]byte(initOut), &initEnvelope); err != nil {
		t.Fatalf("index init json invalid: %v\n%s", err, initOut)
	}
	if initEnvelope["command"] != "index.init" || initEnvelope["status"] != "success" {
		t.Fatalf("index init envelope = %#v", initEnvelope)
	}
	initFacts := initEnvelope["facts"].(map[string]any)
	if initFacts["schema_version"] == "" || initFacts["index_status"] == "" || initFacts["path"] != ".pinax/index.sqlite" {
		t.Fatalf("index init facts = %#v", initFacts)
	}

	rebuildOut := runCLI(t, "index", "rebuild", "--vault", root, "--json")
	var rebuildEnvelope map[string]any
	if err := json.Unmarshal([]byte(rebuildOut), &rebuildEnvelope); err != nil {
		t.Fatalf("index rebuild json invalid: %v\n%s", err, rebuildOut)
	}
	rebuildFacts := rebuildEnvelope["facts"].(map[string]any)
	for _, key := range []string{"notes", "tags", "links", "tokens", "dimensions", "schema_version"} {
		if value, ok := rebuildFacts[key]; !ok || value == "" {
			t.Fatalf("index rebuild facts missing %s: %#v", key, rebuildFacts)
		}
	}

	statusOut := runCLI(t, "index", "status", "--vault", root, "--json")
	var statusEnvelope map[string]any
	if err := json.Unmarshal([]byte(statusOut), &statusEnvelope); err != nil {
		t.Fatalf("index status json invalid: %v\n%s", err, statusOut)
	}
	statusFacts := statusEnvelope["facts"].(map[string]any)
	if statusEnvelope["command"] != "index.status" || statusFacts["index_status"] != "fresh" || statusFacts["schema_version"] == "" {
		t.Fatalf("index status envelope = %#v", statusEnvelope)
	}

	searchOut := runCLI(t, "search", "认证", "--tag", "auth", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--limit", "5", "--vault", root, "--json")
	var searchEnvelope map[string]any
	if err := json.Unmarshal([]byte(searchOut), &searchEnvelope); err != nil {
		t.Fatalf("search json invalid: %v\n%s", err, searchOut)
	}
	searchFacts := searchEnvelope["facts"].(map[string]any)
	for _, want := range []string{"engine", "index_status", "total", "returned", "filter.tag", "filter.group", "filter.folder", "filter.kind", "filter.status"} {
		if searchFacts[want] == "" {
			t.Fatalf("search facts missing %s: %#v", want, searchFacts)
		}
	}
	if searchFacts["engine"] != "index" || searchFacts["index_status"] != "fresh" || searchFacts["returned"] != "1" {
		t.Fatalf("search facts = %#v", searchFacts)
	}
	data := searchEnvelope["data"].(map[string]any)
	results := data["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("search results = %#v", data)
	}
	result := results[0].(map[string]any)
	if result["score"] == nil || len(result["matched_fields"].([]any)) == 0 || result["snippet"] == "" {
		t.Fatalf("search result missing score/matches/snippet: %#v", result)
	}

	runCLI(t, "note", "new", "Auth A", "--group", "work", "--folder", "architecture", "--kind", "reference", "--status", "active", "--tags", "auth", "--body", "认证 checklist", "--slug", "a-auth", "--vault", root, "--json")
	sortedOut := runCLI(t, "search", "认证", "--tag", "auth", "--sort", "title", "--limit", "1", "--vault", root, "--json")
	var sortedEnvelope map[string]any
	if err := json.Unmarshal([]byte(sortedOut), &sortedEnvelope); err != nil {
		t.Fatalf("sorted search json invalid: %v\n%s", err, sortedOut)
	}
	sortedFacts := sortedEnvelope["facts"].(map[string]any)
	if sortedFacts["sort"] != "title" || sortedFacts["returned"] != "1" {
		t.Fatalf("sorted search facts = %#v", sortedFacts)
	}
	sortedResults := sortedEnvelope["data"].(map[string]any)["results"].([]any)
	sortedNote := sortedResults[0].(map[string]any)["note"].(map[string]any)
	if sortedNote["title"] != "Auth A" {
		t.Fatalf("sorted search first note = %#v", sortedNote)
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "work", "architecture", "auth-design.md"), strings.Replace(readCLIFile(t, filepath.Join(root, "notes", "work", "architecture", "auth-design.md")), "OAuth", "OAuth2", 1))
	staleOut := runCLI(t, "index", "status", "--vault", root, "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("stale status json invalid: %v\n%s", err, staleOut)
	}
	if staleEnvelope["facts"].(map[string]any)["index_status"] != "stale" {
		t.Fatalf("expected stale index after note change: %#v", staleEnvelope)
	}

	staleSearchOut := runCLI(t, "search", "认证", "--allow-stale", "--vault", root, "--json")
	var staleSearchEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleSearchOut), &staleSearchEnvelope); err != nil {
		t.Fatalf("stale search json invalid: %v\n%s", err, staleSearchOut)
	}
	if staleSearchEnvelope["status"] != "partial" || staleSearchEnvelope["facts"].(map[string]any)["engine"] != "index" || staleSearchEnvelope["facts"].(map[string]any)["index_status"] != "stale" {
		t.Fatalf("stale search envelope = %#v", staleSearchEnvelope)
	}
	if len(staleSearchEnvelope["actions"].([]any)) == 0 {
		t.Fatalf("stale search missing rebuild action: %#v", staleSearchEnvelope)
	}

	invalidDate, err := runCLIExpectError("search", "认证", "--updated-after", "not-a-date", "--vault", root, "--json")
	if err == nil || !strings.Contains(invalidDate, "invalid_date_filter") {
		t.Fatalf("invalid date filter err=%v out=%s", err, invalidDate)
	}

	if err := os.Remove(filepath.Join(root, ".pinax", "index.sqlite")); err != nil {
		t.Fatalf("remove index for fallback search: %v", err)
	}
	fallbackOut := runCLI(t, "search", "认证", "--vault", root, "--json")
	var fallbackEnvelope map[string]any
	if err := json.Unmarshal([]byte(fallbackOut), &fallbackEnvelope); err != nil {
		t.Fatalf("fallback search json invalid: %v\n%s", err, fallbackOut)
	}
	fallbackFacts := fallbackEnvelope["facts"].(map[string]any)
	if fallbackFacts["engine"] != "index" || fallbackFacts["index_status"] != "fresh" || fallbackFacts["index_loaded"] != "lazy_rebuild" || fallbackFacts["returned"] != "2" {
		t.Fatalf("lazy search facts = %#v", fallbackFacts)
	}
	if !fileExists(filepath.Join(root, ".pinax", "index.sqlite")) {
		t.Fatalf("lazy search did not recreate index.sqlite")
	}
}

func TestNoteShowStemAndMetadataPlanQueryResolverContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "stem-target.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_stem\ntitle: Different Title\ntags: []\n---\n\n# Different Title\n")
	showOut := runCLI(t, "note", "show", "stem-target", "--vault", root, "--json")
	var showEnvelope map[string]any
	if err := json.Unmarshal([]byte(showOut), &showEnvelope); err != nil {
		t.Fatalf("note show stem json invalid: %v\n%s", err, showOut)
	}
	showFacts := showEnvelope["facts"].(map[string]any)
	if showEnvelope["command"] != "note.show" || showFacts["path"] != "notes/stem-target.md" || showFacts["resolver.match_field"] != "stem" || showFacts["resolver.candidates"] != "1" {
		t.Fatalf("note show stem envelope = %#v", showEnvelope)
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "adopt-target.md"), "# Adopt Target\n\nraw markdown\n")
	metadataOut := runCLI(t, "metadata", "plan", "adopt-target", "--vault", root, "--json")
	var metadataEnvelope map[string]any
	if err := json.Unmarshal([]byte(metadataOut), &metadataEnvelope); err != nil {
		t.Fatalf("metadata plan query json invalid: %v\n%s", err, metadataOut)
	}
	metadataFacts := metadataEnvelope["facts"].(map[string]any)
	if metadataEnvelope["command"] != "metadata.plan" || metadataFacts["candidates"] != "1" || metadataFacts["planned_updates"] != "1" || metadataFacts["writes"] != "false" || !strings.Contains(metadataOut, "record adopt adopt-target --plan") {
		t.Fatalf("metadata plan query envelope = %#v\n%s", metadataEnvelope, metadataOut)
	}
	if fileExists(filepath.Join(root, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("metadata plan query wrote ledger")
	}

	ambiguousRoot := t.TempDir()
	runCLI(t, "init", ambiguousRoot, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(ambiguousRoot, "notes", "target-a.md"), "# Target A\n")
	writeCLIFixture(t, filepath.Join(ambiguousRoot, "notes", "target-b.md"), "# Target B\n")
	ambiguous, err := runCLIExpectError("metadata", "plan", "target", "--vault", ambiguousRoot, "--json")
	if err == nil {
		t.Fatalf("metadata plan ambiguous succeeded: %s", ambiguous)
	}
	var ambiguousEnvelope map[string]any
	if err := json.Unmarshal([]byte(ambiguous), &ambiguousEnvelope); err != nil {
		t.Fatalf("metadata plan ambiguous json invalid: %v\n%s", err, ambiguous)
	}
	if ambiguousEnvelope["command"] != "metadata.plan" || ambiguousEnvelope["status"] != "failed" || ambiguousEnvelope["error"].(map[string]any)["code"] != "vault_object_ref_ambiguous" || !strings.Contains(ambiguous, "notes/target-a.md") || !strings.Contains(ambiguous, "notes/target-b.md") {
		t.Fatalf("metadata ambiguous envelope = %#v\n%s", ambiguousEnvelope, ambiguous)
	}
}

func TestRecordAdoptQueryPlanContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "yeisme.md"), "# Yeisme\n\nunmanaged markdown\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "other.md"), "# Other\n\nunmanaged markdown\n")

	planOut := runCLI(t, "record", "adopt", "yeisme", "--plan", "--vault", root, "--json")
	var planEnvelope map[string]any
	if err := json.Unmarshal([]byte(planOut), &planEnvelope); err != nil {
		t.Fatalf("record adopt query plan json invalid: %v\n%s", err, planOut)
	}
	planFacts := planEnvelope["facts"].(map[string]any)
	if planEnvelope["command"] != "record.adopt" || planFacts["writes"] != "false" || planFacts["candidates"] != "1" || planFacts["operations"] != "1" || planFacts["adopted"] != "0" {
		t.Fatalf("record adopt query plan envelope = %#v", planEnvelope)
	}
	if !strings.Contains(planOut, "notes/yeisme.md") || !strings.Contains(planOut, "pinax record adopt yeisme --vault") {
		t.Fatalf("record adopt query plan missing operation/action:\n%s", planOut)
	}
	if fileExists(filepath.Join(root, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("record adopt --plan wrote ledger")
	}

	fullPlan := runCLI(t, "record", "adopt", "--plan", "--vault", root, "--json")
	var fullEnvelope map[string]any
	if err := json.Unmarshal([]byte(fullPlan), &fullEnvelope); err != nil {
		t.Fatalf("record adopt full plan json invalid: %v\n%s", err, fullPlan)
	}
	fullFacts := fullEnvelope["facts"].(map[string]any)
	if fullFacts["operations"] != "2" || fullFacts["writes"] != "false" || fileExists(filepath.Join(root, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("record adopt full plan envelope = %#v", fullEnvelope)
	}

	ambiguousRoot := t.TempDir()
	runCLI(t, "init", ambiguousRoot, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(ambiguousRoot, "notes", "yeisme-a.md"), "# Yeisme A\n")
	writeCLIFixture(t, filepath.Join(ambiguousRoot, "notes", "yeisme-b.md"), "# Yeisme B\n")
	ambiguous, err := runCLIExpectError("record", "adopt", "yeisme", "--plan", "--vault", ambiguousRoot, "--json")
	if err == nil {
		t.Fatalf("ambiguous record adopt succeeded: %s", ambiguous)
	}
	var ambiguousEnvelope map[string]any
	if err := json.Unmarshal([]byte(ambiguous), &ambiguousEnvelope); err != nil {
		t.Fatalf("record adopt ambiguous json invalid: %v\n%s", err, ambiguous)
	}
	if ambiguousEnvelope["command"] != "record.adopt" || ambiguousEnvelope["status"] != "failed" || ambiguousEnvelope["error"].(map[string]any)["code"] != "vault_object_ref_ambiguous" || !strings.Contains(ambiguous, "notes/yeisme-a.md") || !strings.Contains(ambiguous, "notes/yeisme-b.md") {
		t.Fatalf("record adopt ambiguous envelope = %#v\n%s", ambiguousEnvelope, ambiguous)
	}
	if fileExists(filepath.Join(ambiguousRoot, ".pinax", "records", "events.jsonl")) {
		t.Fatalf("ambiguous record adopt wrote ledger")
	}
}
func TestRecordHistoryUsesResolverInputCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "History Note", "--body", "history body", "--slug", "history-note", "--vault", root, "--json")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	runCLI(t, "record", "adopt", "--vault", root, "--json")

	out := runCLI(t, "record", "history", "history-note", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("record history json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "record.history" || facts["note_id"] == "" || facts["path"] != "history-note.md" || facts["candidates"] != "1" || facts["match_field"] != "stem" {
		t.Fatalf("record history resolver envelope = %#v\n%s", envelope, out)
	}
}

func TestIndexLookupContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "yeisme.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_yeisme\ntitle: Yeisme Note\ntags: []\n---\n\n# Yeisme Note\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "unmanaged-yeisme.md"), "# Unmanaged Yeisme\n\nraw markdown\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "diagram.md"), "# Diagram\n\nunmanaged diagram markdown\n")
	assetSource := filepath.Join(root, "diagram.png")
	if err := os.WriteFile(assetSource, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 'd', 'i', 'a', 'g'}, 0o644); err != nil {
		t.Fatalf("write asset source: %v", err)
	}
	runCLI(t, "asset", "add", assetSource, "--vault", root, "--json")
	runCLI(t, "index", "refresh", "--vault", root, "--json")

	lookupOut := runCLI(t, "index", "lookup", "yeisme", "--scope", "all", "--vault", root, "--json")
	var lookupEnvelope map[string]any
	if err := json.Unmarshal([]byte(lookupOut), &lookupEnvelope); err != nil {
		t.Fatalf("index lookup json invalid: %v\n%s", err, lookupOut)
	}
	lookupFacts := lookupEnvelope["facts"].(map[string]any)
	if lookupEnvelope["command"] != "index.lookup" || lookupFacts["candidates"] != "2" || lookupFacts["index_status"] == "" {
		t.Fatalf("index lookup envelope = %#v", lookupEnvelope)
	}
	if !strings.Contains(lookupOut, `"object_kind":"note"`) || !strings.Contains(lookupOut, `"object_kind":"file"`) || !strings.Contains(lookupOut, `"managed_status":"registered"`) || !strings.Contains(lookupOut, `"managed_status":"adoptable"`) {
		t.Fatalf("index lookup missing note/file candidates:\n%s", lookupOut)
	}

	assetOut := runCLI(t, "index", "lookup", "diagram", "--kind", "asset", "--scope", "all", "--vault", root, "--agent")
	for _, want := range []string{"command=index.lookup", "fact.kind=asset", "fact.candidates=1", "candidate.1.object_kind=asset", "candidate.1.managed_status=managed", "candidate.1.path=assets/diagram.png"} {
		if !strings.Contains(assetOut, want) {
			t.Fatalf("index lookup asset agent missing %q:\n%s", want, assetOut)
		}
	}

	ambiguousOut := runCLI(t, "index", "lookup", "diagram", "--scope", "all", "--vault", root, "--json")
	var ambiguousEnvelope map[string]any
	if err := json.Unmarshal([]byte(ambiguousOut), &ambiguousEnvelope); err != nil {
		t.Fatalf("index lookup ambiguous json invalid: %v\n%s", err, ambiguousOut)
	}
	ambiguousFacts := ambiguousEnvelope["facts"].(map[string]any)
	if ambiguousFacts["candidates"] != "2" || !strings.Contains(ambiguousOut, `"object_kind":"asset"`) || !strings.Contains(ambiguousOut, `"object_kind":"file"`) {
		t.Fatalf("index lookup ambiguous envelope = %#v\n%s", ambiguousEnvelope, ambiguousOut)
	}

	searchOut := runCLI(t, "search", "Unmanaged Yeisme", "--vault", root, "--json")
	var searchEnvelope map[string]any
	if err := json.Unmarshal([]byte(searchOut), &searchEnvelope); err != nil {
		t.Fatalf("search json invalid: %v\n%s", err, searchOut)
	}
	if searchEnvelope["facts"].(map[string]any)["returned"] != "0" {
		t.Fatalf("unmanaged markdown entered ordinary search: %#v", searchEnvelope)
	}
}

func TestIndexDefaultSummaryAndMachineContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Index Test", "--body", "body", "--slug", "index-contract", "--vault", root, "--json")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove index: %v", err)
	}

	humanOut := runCLI(t, "index", "--vault", root)
	for _, want := range []string{"Status", "Local index", "Next step", "pinax index refresh --vault"} {
		if !strings.Contains(humanOut, want) {
			t.Fatalf("index summary missing %q:\n%s", want, humanOut)
		}
	}
	if fileExists(indexPath) {
		t.Fatalf("default index summary created index.sqlite")
	}

	jsonOut := runCLI(t, "index", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("index summary json invalid: %v\n%s", err, jsonOut)
	}
	if envelope["command"] != "index.summary" || envelope["status"] != "partial" || envelope["mode"] != "json" {
		t.Fatalf("index summary envelope = %#v", envelope)
	}
	facts := envelope["facts"].(map[string]any)
	for _, key := range []string{"index_status", "path", "schema_version", "notes", "recommended_action", "writes"} {
		if _, ok := facts[key]; !ok {
			t.Fatalf("index summary facts missing %s: %#v", key, facts)
		}
	}
	if facts["index_status"] != "missing" || facts["writes"] != "false" || !strings.Contains(fmt.Sprint(facts["recommended_action"]), "pinax index refresh --vault") {
		t.Fatalf("index summary facts = %#v", facts)
	}
	if fileExists(indexPath) {
		t.Fatalf("json index summary created index.sqlite")
	}
	statusOut := runCLI(t, "index", "status", "--vault", root, "--json")
	var statusEnvelope map[string]any
	if err := json.Unmarshal([]byte(statusOut), &statusEnvelope); err != nil {
		t.Fatalf("index status json invalid: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "pinax index refresh --vault") || strings.Contains(statusOut, "delete .pinax/index.sqlite") || strings.Contains(statusOut, "edit .pinax/index.sqlite") {
		t.Fatalf("index status next action unsafe or missing refresh:\n%s", statusOut)
	}

	agentOut := runCLI(t, "index", "--vault", root, "--agent")
	for _, want := range []string{"command=index.summary", "status=partial", "fact.index_status=missing", "fact.writes=false", "action.refresh="} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("index summary agent missing %q:\n%s", want, agentOut)
		}
	}
	for _, localized := range []string{"状态", "推荐下一步", "本地索引"} {
		if strings.Contains(agentOut, localized) {
			t.Fatalf("agent output contains localized prose %q:\n%s", localized, agentOut)
		}
	}
	if fileExists(indexPath) {
		t.Fatalf("agent index summary created index.sqlite")
	}
}

func TestNoteCommandUXCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	inlineOut := runCLI(t, "note", "new", "Inline Note", "--body", "正文", "--tags", "research", "--status", "active", "--dir", "work", "--slug", "inline", "--vault", root, "--json")
	var inlineEnvelope map[string]any
	if err := json.Unmarshal([]byte(inlineOut), &inlineEnvelope); err != nil {
		t.Fatalf("inline note json invalid: %v\n%s", err, inlineOut)
	}
	if inlineEnvelope["command"] != "note.new" || inlineEnvelope["status"] != "success" {
		t.Fatalf("inline envelope = %#v", inlineEnvelope)
	}
	inlineContent := readCLIFile(t, filepath.Join(root, "notes", "work", "inline.md"))
	for _, want := range []string{"title: Inline Note", "tags: [research]", "status: active", "正文"} {
		if !strings.Contains(inlineContent, want) {
			t.Fatalf("inline note missing %q:\n%s", want, inlineContent)
		}
	}

	dryRun := runCLI(t, "note", "create", "Draft", "--dry-run", "--dir", "work", "--vault", root, "--json")
	if !strings.Contains(dryRun, "planned_path") || fileExists(filepath.Join(root, "notes", "work", "draft.md")) {
		t.Fatalf("dry-run did not return plan or wrote file:\n%s", dryRun)
	}

	failed, err := runCLIExpectError("note", "new", "Conflict", "--body", "a", "--from", filepath.Join(root, "source.md"), "--vault", root, "--json")
	if err == nil || !strings.Contains(failed, "note_source_conflict") {
		t.Fatalf("source conflict err=%v out=%s", err, failed)
	}
	missingSource, err := runCLIExpectError("note", "new", "Missing Source", "--from", filepath.Join(root, "missing.md"), "--vault", root, "--json")
	if err == nil || !strings.Contains(missingSource, "internal_error") || fileExists(filepath.Join(root, "notes", "missing-source.md")) {
		t.Fatalf("missing source err=%v out=%s", err, missingSource)
	}

	stdinOut := runCLIWithInput(t, "stdin body", "note", "create", "Stdin Note", "--stdin", "--vault", root, "--json")
	if !strings.Contains(stdinOut, "note.new") || !strings.Contains(readCLIFile(t, filepath.Join(root, "stdin-note.md")), "stdin body") {
		t.Fatalf("stdin create failed:\n%s", stdinOut)
	}

	listOut := runCLI(t, "note", "list", "--tag", "research", "--status", "active", "--recent", "--limit", "1", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("list json invalid: %v\n%s", err, listOut)
	}
	if listEnvelope["command"] != "note.list" || listEnvelope["status"] != "success" {
		t.Fatalf("list envelope = %#v", listEnvelope)
	}
	facts := listEnvelope["facts"].(map[string]any)
	if facts["total"] != "1" || facts["returned"] != "1" || facts["filter.tag"] != "research" {
		t.Fatalf("list facts = %#v", facts)
	}
	humanList := runCLI(t, "note", "list", "--tag", "research", "--vault", root)
	for _, want := range []string{"Highlights", "Path", "Title", "Inline Note", "notes/work/inline.md"} {
		if !strings.Contains(humanList, want) {
			t.Fatalf("human list missing %q:\n%s", want, humanList)
		}
	}

	showByTitle := runCLI(t, "note", "read", "Inline Note", "--vault", root, "--json")
	if !strings.Contains(showByTitle, "notes/work/inline.md") {
		t.Fatalf("show by title failed:\n%s", showByTitle)
	}
	writeCLIFixture(t, filepath.Join(root, "notes", "dupe-a.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_dupe_a\ntitle: Meeting\ntags: []\n---\n\n# Meeting\n")
	writeCLIFixture(t, filepath.Join(root, "notes", "dupe-b.md"), "---\nschema_version: pinax.note.v1\nnote_id: note_dupe_b\ntitle: Meeting\ntags: []\n---\n\n# Meeting\n")
	ambiguous, err := runCLIExpectError("note", "show", "Meeting", "--vault", root, "--json")
	if err == nil || !strings.Contains(ambiguous, "note_ref_ambiguous") || !strings.Contains(ambiguous, "dupe-a.md") {
		t.Fatalf("ambiguous ref err=%v out=%s", err, ambiguous)
	}
	ambiguousAgent, err := runCLIExpectError("note", "show", "Meeting", "--vault", root, "--agent")
	if err == nil || !strings.Contains(ambiguousAgent, "error.code=note_ref_ambiguous") || !strings.Contains(ambiguousAgent, "candidate.1.path=notes/dupe-a.md") || !strings.Contains(ambiguousAgent, "candidate.2.note_id=note_dupe_b") {
		t.Fatalf("ambiguous agent err=%v out=%s", err, ambiguousAgent)
	}

	editorLog := filepath.Join(root, "editor.log")
	editor := writeFakeEditor(t, root, editorLog)
	editOut := runCLI(t, "note", "edit", "Inline Note", "--editor", editor, "--vault", root, "--json")
	if !strings.Contains(editOut, "note.edit") || !strings.Contains(readCLIFile(t, editorLog), "notes/work/inline.md") {
		t.Fatalf("edit output/log invalid:\n%s\nlog=%s", editOut, readCLIFile(t, editorLog))
	}

	oldEditor, hadEditor := os.LookupEnv("EDITOR")
	_ = os.Unsetenv("EDITOR")
	t.Cleanup(func() {
		if hadEditor {
			_ = os.Setenv("EDITOR", oldEditor)
		}
	})
	missingEditor, err := runCLIExpectError("note", "edit", "Inline Note", "--vault", root, "--json")
	if err == nil || !strings.Contains(missingEditor, "editor_not_configured") {
		t.Fatalf("missing editor err=%v out=%s", err, missingEditor)
	}

	renameOut := runCLI(t, "note", "rename", "Inline Note", "Renamed Note", "--vault", root, "--json")
	if !strings.Contains(renameOut, "note.rename") || !fileExists(filepath.Join(root, "notes", "work", "renamed-note.md")) {
		t.Fatalf("rename failed:\n%s", renameOut)
	}
	moveOut := runCLI(t, "note", "move", "Renamed Note", "archive", "--vault", root, "--json")
	if !strings.Contains(moveOut, "note.move") || !fileExists(filepath.Join(root, "notes", "archive", "renamed-note.md")) {
		t.Fatalf("move failed:\n%s", moveOut)
	}
	archiveOut := runCLI(t, "note", "archive", "Renamed Note", "--vault", root, "--json")
	archivedContent := readCLIFile(t, filepath.Join(root, "notes", "archive", "renamed-note.md"))
	if !strings.Contains(archiveOut, "note.archive") || !strings.Contains(archivedContent, "status: archived") {
		t.Fatalf("archive failed:\n%s\n%s", archiveOut, archivedContent)
	}
	propertyOut := runCLI(t, "note", "property", "set", "Renamed Note", "priority", "2", "--vault", root, "--json")
	if !strings.Contains(propertyOut, "note.property") || !strings.Contains(propertyOut, `"property":"priority"`) || !strings.Contains(propertyOut, `"index_updated":"true"`) {
		t.Fatalf("property set output invalid:\n%s", propertyOut)
	}
	withProperty := readCLIFile(t, filepath.Join(root, "notes", "archive", "renamed-note.md"))
	if !strings.Contains(withProperty, "priority: 2") {
		t.Fatalf("property set did not update frontmatter:\n%s", withProperty)
	}
	propertyList := runCLI(t, "note", "list", "--property", "priority", "--strict-properties", "--vault", root, "--json")
	if !strings.Contains(propertyList, "priority") || !strings.Contains(propertyList, "Renamed Note") {
		t.Fatalf("property set did not update property projection:\n%s", propertyList)
	}
	propertyRemove := runCLI(t, "note", "property", "remove", "Renamed Note", "priority", "--vault", root, "--agent")
	for _, want := range []string{"command=note.property", "fact.operation=remove", "fact.property=priority", "fact.index_updated=true"} {
		if !strings.Contains(propertyRemove, want) {
			t.Fatalf("property remove agent missing %q:\n%s", want, propertyRemove)
		}
	}
	withoutProperty := readCLIFile(t, filepath.Join(root, "notes", "archive", "renamed-note.md"))
	if strings.Contains(withoutProperty, "priority:") {
		t.Fatalf("property remove left priority frontmatter:\n%s", withoutProperty)
	}
	tagOut := runCLI(t, "note", "tag", "add", "Renamed Note", "important", "--vault", root, "--json")
	if !strings.Contains(tagOut, "note.tag") || !strings.Contains(readCLIFile(t, filepath.Join(root, "notes", "archive", "renamed-note.md")), "important") {
		t.Fatalf("tag add failed:\n%s", tagOut)
	}
	deleteWithoutApproval, err := runCLIExpectError("note", "delete", "Renamed Note", "--hard", "--vault", root, "--json")
	if err == nil || !strings.Contains(deleteWithoutApproval, "approval_required") {
		t.Fatalf("hard delete without approval err=%v out=%s", err, deleteWithoutApproval)
	}
	deleteOut := runCLI(t, "note", "delete", "Renamed Note", "--yes", "--vault", root, "--json")
	if !strings.Contains(deleteOut, "note.delete") || !strings.Contains(deleteOut, ".pinax/trash/") || fileExists(filepath.Join(root, "notes", "archive", "renamed-note.md")) {
		t.Fatalf("trash delete failed:\n%s", deleteOut)
	}

	help := runCLI(t, "note", "--help")
	for _, want := range []string{"create", "read", "open", "edit", "rename", "move", "archive", "delete", "tag", "property"} {
		if !strings.Contains(help, want) {
			t.Fatalf("note help missing %q:\n%s", want, help)
		}
	}
}

func TestNoteTagBulkManagementCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Alpha", "--tags", "old,keep", "--vault", root, "--json")
	runCLI(t, "note", "new", "Beta", "--tags", "old", "--vault", root, "--json")

	dryRun := runCLI(t, "note", "tags", "rename", "old", "new", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(dryRun, "tag.rename") || !strings.Contains(dryRun, `"dry_run":"true"`) || !strings.Contains(dryRun, `"matched":"2"`) {
		t.Fatalf("tag rename dry-run output invalid:\n%s", dryRun)
	}
	if !strings.Contains(readCLIFile(t, filepath.Join(root, "alpha.md")), "old") {
		t.Fatalf("tag rename dry-run unexpectedly changed note")
	}

	renameOut := runCLI(t, "note", "tags", "rename", "old", "new", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=tag.rename", "fact.old_tag=old", "fact.new_tag=new", "fact.changed=2", "fact.index_updated=true"} {
		if !strings.Contains(renameOut, want) {
			t.Fatalf("tag rename agent missing %q:\n%s", want, renameOut)
		}
	}
	for _, rel := range []string{"alpha.md", "beta.md"} {
		content := readCLIFile(t, filepath.Join(root, rel))
		if !strings.Contains(content, "new") || strings.Contains(content, "old") {
			t.Fatalf("tag rename did not update %s:\n%s", rel, content)
		}
	}

	deleteOut := runCLI(t, "note", "tags", "delete", "new", "--yes", "--vault", root, "--json")
	if !strings.Contains(deleteOut, "tag.delete") || !strings.Contains(deleteOut, `"changed":"2"`) || !strings.Contains(deleteOut, `"index_updated":"true"`) {
		t.Fatalf("tag delete output invalid:\n%s", deleteOut)
	}
	for _, rel := range []string{"alpha.md", "beta.md"} {
		content := readCLIFile(t, filepath.Join(root, rel))
		if strings.Contains(content, "new") {
			t.Fatalf("tag delete did not remove tag from %s:\n%s", rel, content)
		}
	}
}

func TestNoteFolderBulkManagementCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Alpha", "--folder", "inbox", "--slug", "alpha", "--vault", root, "--json")
	runCLI(t, "note", "new", "Beta", "--folder", "inbox", "--slug", "beta", "--vault", root, "--json")

	dryRun := runCLI(t, "note", "folders", "rename", "inbox", "archive", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(dryRun, "folder.rename") || !strings.Contains(dryRun, `"dry_run":"true"`) || !strings.Contains(dryRun, `"matched":"2"`) {
		t.Fatalf("folder rename dry-run output invalid:\n%s", dryRun)
	}
	if !fileExists(filepath.Join(root, "inbox", "alpha.md")) || fileExists(filepath.Join(root, "archive", "alpha.md")) {
		t.Fatalf("folder rename dry-run unexpectedly changed files")
	}

	withoutApproval, err := runCLIExpectError("note", "folders", "rename", "inbox", "archive", "--vault", root, "--json")
	if err == nil || !strings.Contains(withoutApproval, "approval_required") || fileExists(filepath.Join(root, "archive", "alpha.md")) {
		t.Fatalf("folder rename without approval err=%v out=%s", err, withoutApproval)
	}

	renameOut := runCLI(t, "note", "folders", "rename", "inbox", "archive", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.rename", "fact.old_folder=inbox", "fact.new_folder=archive", "fact.changed=2", "fact.index_updated=true"} {
		if !strings.Contains(renameOut, want) {
			t.Fatalf("folder rename agent missing %q:\n%s", want, renameOut)
		}
	}
	for _, rel := range []string{"archive/alpha.md", "archive/beta.md"} {
		content := readCLIFile(t, filepath.Join(root, rel))
		if !strings.Contains(content, "folder: archive") || strings.Contains(content, "folder: inbox") {
			t.Fatalf("folder rename did not update %s:\n%s", rel, content)
		}
	}
	if fileExists(filepath.Join(root, "inbox", "alpha.md")) || fileExists(filepath.Join(root, "inbox", "beta.md")) {
		t.Fatalf("folder rename left old note files")
	}

	listOut := runCLI(t, "note", "list", "--folder", "archive", "--vault", root, "--json")
	if !strings.Contains(listOut, `"filter.folder":"archive"`) || !strings.Contains(listOut, `"total":"2"`) {
		t.Fatalf("folder rename did not refresh list projection:\n%s", listOut)
	}
}

func TestFolderCreateListShowCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	dryRun := runCLI(t, "folder", "create", "spaces/research", "--purpose", "notes", "--dry-run", "--vault", root, "--json")
	var dryEnvelope map[string]any
	if err := json.Unmarshal([]byte(dryRun), &dryEnvelope); err != nil {
		t.Fatalf("folder create dry-run json invalid: %v\n%s", err, dryRun)
	}
	dryFacts := dryEnvelope["facts"].(map[string]any)
	if dryEnvelope["command"] != "folder.create" || dryFacts["folder_path"] != "spaces/research" || dryFacts["purpose"] != "notes" || dryFacts["dry_run"] != "true" || dryFacts["writes"] != "false" {
		t.Fatalf("folder create dry-run facts invalid: %#v\n%s", dryFacts, dryRun)
	}
	if fileExists(filepath.Join(root, "spaces", "research")) {
		t.Fatalf("folder create dry-run unexpectedly created directory")
	}

	createOut := runCLI(t, "folder", "create", "spaces/research", "--purpose", "notes", "--vault", root, "--json")
	var createEnvelope map[string]any
	if err := json.Unmarshal([]byte(createOut), &createEnvelope); err != nil {
		t.Fatalf("folder create json invalid: %v\n%s", err, createOut)
	}
	createFacts := createEnvelope["facts"].(map[string]any)
	if createEnvelope["command"] != "folder.create" || createFacts["folder_path"] != "spaces/research" || createFacts["purpose"] != "notes" || createFacts["managed_status"] != "managed" || createFacts["index_updated"] != "true" {
		t.Fatalf("folder create facts invalid: %#v\n%s", createFacts, createOut)
	}
	if !fileExists(filepath.Join(root, "spaces", "research")) {
		t.Fatalf("folder create did not create directory")
	}
	registry := readCLIFile(t, filepath.Join(root, ".pinax", "folders.json"))
	if !strings.Contains(registry, "spaces/research") || !strings.Contains(registry, "pinax.folders.v1") {
		t.Fatalf("folder registry missing created folder:\n%s", registry)
	}

	showOut := runCLI(t, "folder", "show", "spaces/research", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.show", "fact.folder_path=spaces/research", "fact.purpose=notes", "fact.managed_status=managed"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("folder show agent missing %q:\n%s", want, showOut)
		}
	}

	listOut := runCLI(t, "folder", "list", "--purpose", "notes", "--include-empty", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("folder list json invalid: %v\n%s", err, listOut)
	}
	listFacts := listEnvelope["facts"].(map[string]any)
	if listEnvelope["command"] != "folder.list" || listFacts["folders"] == "" || listFacts["filter.purpose"] != "notes" || !strings.Contains(listOut, "spaces/research") {
		t.Fatalf("folder list output invalid facts=%#v out=%s", listFacts, listOut)
	}

	unsafeOut, err := runCLIExpectError("folder", "create", "../outside", "--vault", root, "--json")
	if err == nil || !strings.Contains(unsafeOut, "unsafe_folder_path") {
		t.Fatalf("unsafe folder create err=%v out=%s", err, unsafeOut)
	}

	repairOut := runCLI(t, "folder", "repair", "--plan", "--vault", root, "--json")
	if !strings.Contains(repairOut, `"command":"folder.repair"`) || !strings.Contains(repairOut, `"writes":"false"`) {
		t.Fatalf("folder repair plan output invalid:\n%s", repairOut)
	}
}

func TestFolderMutationManagementCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "folder", "create", "spaces/research", "--purpose", "notes", "--vault", root, "--json")

	dryRename := runCLI(t, "folder", "rename", "spaces/research", "spaces/archive", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(dryRename, "folder.rename") || !strings.Contains(dryRename, `"dry_run":"true"`) || !strings.Contains(dryRename, `"target_path":"spaces/archive"`) {
		t.Fatalf("folder rename dry-run output invalid:\n%s", dryRename)
	}
	if !fileExists(filepath.Join(root, "spaces", "research")) || fileExists(filepath.Join(root, "spaces", "archive")) {
		t.Fatalf("folder rename dry-run unexpectedly changed directories")
	}
	withoutApproval, err := runCLIExpectError("folder", "rename", "spaces/research", "spaces/archive", "--vault", root, "--json")
	if err == nil || !strings.Contains(withoutApproval, "approval_required") || fileExists(filepath.Join(root, "spaces", "archive")) {
		t.Fatalf("folder rename without approval err=%v out=%s", err, withoutApproval)
	}
	renameOut := runCLI(t, "folder", "rename", "spaces/research", "spaces/archive", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.rename", "fact.folder_path=spaces/research", "fact.target_path=spaces/archive", "fact.renamed=true", "fact.index_updated=true"} {
		if !strings.Contains(renameOut, want) {
			t.Fatalf("folder rename agent missing %q:\n%s", want, renameOut)
		}
	}
	if fileExists(filepath.Join(root, "spaces", "research")) || !fileExists(filepath.Join(root, "spaces", "archive")) {
		t.Fatalf("folder rename did not move directory")
	}

	runCLI(t, "folder", "create", "containers", "--vault", root, "--json")
	moveOut := runCLI(t, "folder", "move", "spaces/archive", "containers", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.move", "fact.folder_path=spaces/archive", "fact.target_path=containers/archive", "fact.moved=true", "fact.index_updated=true"} {
		if !strings.Contains(moveOut, want) {
			t.Fatalf("folder move agent missing %q:\n%s", want, moveOut)
		}
	}
	if fileExists(filepath.Join(root, "spaces", "archive")) || !fileExists(filepath.Join(root, "containers", "archive")) {
		t.Fatalf("folder move did not move directory")
	}

	manualPath := filepath.Join(root, "manual", "assets")
	if err := os.MkdirAll(manualPath, 0o755); err != nil {
		t.Fatalf("create fixture folder: %v", err)
	}
	writeCLIFixture(t, filepath.Join(manualPath, "diagram.txt"), "fixture")
	adoptOut := runCLI(t, "folder", "adopt", "manual/assets", "--purpose", "assets", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.adopt", "fact.folder_path=manual/assets", "fact.purpose=assets", "fact.managed_status=managed", "fact.index_updated=true"} {
		if !strings.Contains(adoptOut, want) {
			t.Fatalf("folder adopt agent missing %q:\n%s", want, adoptOut)
		}
	}
	registry := readCLIFile(t, filepath.Join(root, ".pinax", "folders.json"))
	if !strings.Contains(registry, "manual/assets") || !strings.Contains(registry, "containers/archive") {
		t.Fatalf("folder registry missing adopted or moved folder:\n%s", registry)
	}

	nonEmptyDelete, err := runCLIExpectError("folder", "delete", "manual/assets", "--empty-only", "--yes", "--vault", root, "--json")
	if err == nil || !strings.Contains(nonEmptyDelete, "folder_not_empty") || !fileExists(filepath.Join(manualPath, "diagram.txt")) {
		t.Fatalf("non-empty folder delete err=%v out=%s", err, nonEmptyDelete)
	}
	dryDelete := runCLI(t, "folder", "delete", "containers/archive", "--empty-only", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(dryDelete, "folder.delete") || !strings.Contains(dryDelete, `"dry_run":"true"`) || !fileExists(filepath.Join(root, "containers", "archive")) {
		t.Fatalf("folder delete dry-run invalid:\n%s", dryDelete)
	}
	deleteOut := runCLI(t, "folder", "delete", "containers/archive", "--empty-only", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.delete", "fact.folder_path=containers/archive", "fact.deleted=true", "fact.index_updated=true"} {
		if !strings.Contains(deleteOut, want) {
			t.Fatalf("folder delete agent missing %q:\n%s", want, deleteOut)
		}
	}
	if fileExists(filepath.Join(root, "containers", "archive")) {
		t.Fatalf("folder delete did not remove empty directory")
	}
}

func TestFolderRenameUpdatesContainedNoteMetadataCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Moved Note", "--folder", "spaces/research", "--slug", "moved", "--vault", root, "--json")

	renameOut := runCLI(t, "folder", "rename", "spaces/research", "spaces/archive", "--yes", "--vault", root, "--agent")
	for _, want := range []string{"command=folder.rename", "fact.renamed=true", "fact.updated_notes=1", "fact.index_updated=true"} {
		if !strings.Contains(renameOut, want) {
			t.Fatalf("folder rename note metadata output missing %q:\n%s", want, renameOut)
		}
	}
	content := readCLIFile(t, filepath.Join(root, "spaces", "archive", "moved.md"))
	if !strings.Contains(content, "folder: spaces/archive") || strings.Contains(content, "folder: spaces/research") {
		t.Fatalf("folder rename did not update note frontmatter:\n%s", content)
	}
	listOut := runCLI(t, "note", "list", "--folder", "spaces/archive", "--vault", root, "--json")
	if !strings.Contains(listOut, `"total":"1"`) || !strings.Contains(listOut, "Moved Note") {
		t.Fatalf("folder rename did not refresh note list folder projection:\n%s", listOut)
	}
}

func TestNoteDeletePromptsInHumanModeCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	created := runCLI(t, "note", "new", "Prompt Delete", "--body", "body", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(created), &envelope); err != nil {
		t.Fatalf("created note json invalid: %v\n%s", err, created)
	}
	path := envelope["facts"].(map[string]any)["path"].(string)
	if !fileExists(filepath.Join(root, filepath.FromSlash(path))) {
		t.Fatalf("created note missing: %s", path)
	}

	out := runCLIWithInput(t, "y\n", "note", "delete", path, "--vault", root)
	if !strings.Contains(out, "Confirm move to trash") || !strings.Contains(out, "Note moved to trash") {
		t.Fatalf("interactive delete output missing prompt or success:\n%s", out)
	}
	if fileExists(filepath.Join(root, filepath.FromSlash(path))) || !strings.Contains(out, ".pinax/trash/") {
		t.Fatalf("interactive delete did not move note to trash:\n%s", out)
	}

	cancelCreated := runCLI(t, "note", "new", "Cancel Delete", "--body", "body", "--vault", root, "--json")
	var cancelEnvelope map[string]any
	if err := json.Unmarshal([]byte(cancelCreated), &cancelEnvelope); err != nil {
		t.Fatalf("cancel note json invalid: %v\n%s", err, cancelCreated)
	}
	cancelPath := cancelEnvelope["facts"].(map[string]any)["path"].(string)
	cancelOut := runCLIWithInput(t, "n\n", "note", "delete", cancelPath, "--vault", root)
	if !strings.Contains(cancelOut, "Confirm move to trash") || !strings.Contains(cancelOut, "Canceled") {
		t.Fatalf("interactive cancel output missing prompt or cancel message:\n%s", cancelOut)
	}
	if !fileExists(filepath.Join(root, filepath.FromSlash(cancelPath))) {
		t.Fatalf("interactive cancel changed note")
	}

	second := runCLI(t, "note", "new", "Machine Delete", "--body", "body", "--vault", root, "--json")
	var secondEnvelope map[string]any
	if err := json.Unmarshal([]byte(second), &secondEnvelope); err != nil {
		t.Fatalf("second note json invalid: %v\n%s", err, second)
	}
	secondPath := secondEnvelope["facts"].(map[string]any)["path"].(string)
	jsonOut, err := runCLIExpectError("note", "delete", secondPath, "--vault", root, "--json")
	if err == nil || strings.Contains(jsonOut, "确认移入回收站") || !strings.Contains(jsonOut, "approval_required") {
		t.Fatalf("machine delete should require --yes without prompt: err=%v out=%s", err, jsonOut)
	}
	if !fileExists(filepath.Join(root, filepath.FromSlash(secondPath))) {
		t.Fatalf("machine delete without --yes changed note")
	}
}

func TestNoteCommandHardeningEditorAndOpenCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Editable", "--body", "body", "--vault", root, "--json")

	editorLog := filepath.Join(root, "editor-args.log")
	editor := writeFakeEditor(t, root, editorLog)
	editOut := runCLI(t, "note", "edit", "Editable", "--editor", editor+" --wait", "--vault", root, "--json")
	var editEnvelope map[string]any
	if err := json.Unmarshal([]byte(editOut), &editEnvelope); err != nil {
		t.Fatalf("edit json invalid: %v\n%s", err, editOut)
	}
	if editEnvelope["command"] != "note.edit" || editEnvelope["status"] != "success" {
		t.Fatalf("edit envelope = %#v", editEnvelope)
	}
	facts := editEnvelope["facts"].(map[string]any)
	if facts["editor_executable"] != editor || !strings.Contains(fmt.Sprint(facts["editor_args"]), "--wait") {
		t.Fatalf("editor facts = %#v", facts)
	}
	log := readCLIFile(t, editorLog)
	if !strings.Contains(log, "--wait") || !strings.Contains(log, "editable.md") {
		t.Fatalf("fake editor log missing args/path:\n%s", log)
	}

	newOpenOut := runCLI(t, "note", "new", "Open After Create", "--body", "body", "--open", "--editor", editor+" --wait", "--vault", root, "--json")
	if strings.Count(strings.TrimSpace(newOpenOut), "\n") != 0 {
		t.Fatalf("note new --open should emit one JSON envelope, got:\n%s", newOpenOut)
	}
	var newEnvelope map[string]any
	if err := json.Unmarshal([]byte(newOpenOut), &newEnvelope); err != nil {
		t.Fatalf("new --open json invalid: %v\n%s", err, newOpenOut)
	}
	if newEnvelope["command"] != "note.new" || newEnvelope["status"] != "success" {
		t.Fatalf("new --open envelope = %#v", newEnvelope)
	}
	newFacts := newEnvelope["facts"].(map[string]any)
	if newFacts["opened"] != "true" || newFacts["editor_executable"] != editor || !strings.Contains(fmt.Sprint(newFacts["editor_args"]), "--wait") {
		t.Fatalf("new --open facts = %#v", newFacts)
	}
	log = readCLIFile(t, editorLog)
	if !strings.Contains(log, "open-after-create.md") {
		t.Fatalf("fake editor log missing created note path:\n%s", log)
	}

	agentOut := runCLI(t, "note", "list", "--recent", "--vault", root, "--agent")
	for _, want := range []string{"command=note.list", "fact.recent=true", "fact.sort=updated"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("recent agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func TestAgentOutputMode(t *testing.T) {
	root := t.TempDir()
	out := runCLI(t, "init", root, "--title", "Vault", "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=vault.init", "status=success"} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent output missing %q:\n%s", want, out)
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
}

func TestMissingRequiredArgReturnsHelpfulProjection(t *testing.T) {
	out, err := runCLIExpectError("note", "show", "--json")
	if err == nil {
		t.Fatalf("note show without arg succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json error output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "note.show" {
		t.Fatalf("error envelope = %#v", envelope)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != "argument_required" {
		t.Fatalf("error object = %#v", envelope["error"])
	}
	if !strings.Contains(errorObject["hint"].(string), "pinax note show <note>") {
		t.Fatalf("error hint = %#v", errorObject["hint"])
	}
}

func TestFlagErrorReturnsHelpfulProjection(t *testing.T) {
	out, err := runCLIExpectError("validate", "--json", "--bogus")
	if err == nil {
		t.Fatalf("unknown flag succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json flag error output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "cli.flag" {
		t.Fatalf("flag error envelope = %#v", envelope)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != "flag_error" {
		t.Fatalf("flag error object = %#v", envelope["error"])
	}
}

func TestOutputModesAreMutuallyExclusive(t *testing.T) {
	out, err := runCLIExpectError("version", "--json", "--agent")
	if err == nil {
		t.Fatalf("conflicting output modes succeeded: %s", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json mode conflict output invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "failed" || envelope["command"] != "cli.output_mode" {
		t.Fatalf("mode conflict envelope = %#v", envelope)
	}
	errorObject, ok := envelope["error"].(map[string]any)
	if !ok || errorObject["code"] != "output_mode_conflict" {
		t.Fatalf("mode conflict error = %#v", envelope["error"])
	}
	if !strings.Contains(errorObject["hint"].(string), "Keep only one output mode") {
		t.Fatalf("mode conflict hint = %#v", errorObject["hint"])
	}
}

func TestApplyHelpDocumentsSafetyFlags(t *testing.T) {
	out := runCLI(t, "organize", "apply", "--help")
	for _, want := range []string{"--yes", "--snapshot-message", "saved and reviewed plan from pinax organize plan --save", "version snapshot"} {
		if !strings.Contains(out, want) {
			t.Fatalf("organize apply help missing %q:\n%s", want, out)
		}
	}
}

func TestEventsAndExplainOutputModes(t *testing.T) {
	root := t.TempDir()
	events := runCLI(t, "init", root, "--events")
	lines := strings.Split(strings.TrimSpace(events), "\n")
	if len(lines) != 2 {
		t.Fatalf("events lines = %q", events)
	}
	for i, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event %d invalid: %v\n%s", i, err, events)
		}
		if _, ok := event["error"]; ok && event["error"] == nil {
			t.Fatalf("event %d contains null error field: %#v", i, event)
		}
	}
	if !strings.Contains(lines[0], `"type":"start"`) || !strings.Contains(lines[1], `"type":"end"`) {
		t.Fatalf("events missing start/end:\n%s", events)
	}

	explain := runCLI(t, "validate", "--vault", root, "--explain")
	for _, want := range []string{"Conclusion:", "Evidence:", "Confidence:", "Recommended next step:"} {
		if !strings.Contains(explain, want) {
			t.Fatalf("explain output missing %q:\n%s", want, explain)
		}
	}
}

func TestIndexRefreshContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Managed Note", "--body", "managed body", "--slug", "managed", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "unmanaged.md"), "# Unmanaged\n\nraw markdown\n")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove index: %v", err)
	}

	missingOut := runCLI(t, "index", "refresh", "--vault", root, "--json")
	var missingEnvelope map[string]any
	if err := json.Unmarshal([]byte(missingOut), &missingEnvelope); err != nil {
		t.Fatalf("refresh missing json invalid: %v\n%s", err, missingOut)
	}
	missingFacts := missingEnvelope["facts"].(map[string]any)
	if missingEnvelope["command"] != "index.refresh" || missingEnvelope["status"] != "success" || missingFacts["index_status"] != "fresh" || missingFacts["scanned"] != "1" || missingFacts["indexed"] != "1" || missingFacts["failed"] != "0" {
		t.Fatalf("refresh missing envelope = %#v", missingEnvelope)
	}
	if !fileExists(indexPath) {
		t.Fatalf("refresh did not create missing index")
	}
	unmanagedSearch := runCLI(t, "search", "Unmanaged", "--vault", root, "--json")
	var unmanagedEnvelope map[string]any
	if err := json.Unmarshal([]byte(unmanagedSearch), &unmanagedEnvelope); err != nil {
		t.Fatalf("unmanaged search json invalid: %v\n%s", err, unmanagedSearch)
	}
	if unmanagedEnvelope["facts"].(map[string]any)["returned"] != "0" {
		t.Fatalf("unmanaged markdown entered index: %#v", unmanagedEnvelope)
	}

	freshOut := runCLI(t, "index", "refresh", "--vault", root, "--json")
	var freshEnvelope map[string]any
	if err := json.Unmarshal([]byte(freshOut), &freshEnvelope); err != nil {
		t.Fatalf("refresh fresh json invalid: %v\n%s", err, freshOut)
	}
	freshFacts := freshEnvelope["facts"].(map[string]any)
	if freshFacts["skipped"] != "1" || freshFacts["changed"] != "0" || freshFacts["indexed"] != "0" {
		t.Fatalf("refresh fresh facts = %#v", freshFacts)
	}

	changedSinceOut, changedSinceErr := runCLIExpectError("index", "refresh", "--changed-since", "rev_1", "--vault", root, "--json")
	if changedSinceErr == nil {
		t.Fatalf("changed-since refresh unexpectedly succeeded:\n%s", changedSinceOut)
	}
	var changedSinceEnvelope map[string]any
	if err := json.Unmarshal([]byte(changedSinceOut), &changedSinceEnvelope); err != nil {
		t.Fatalf("changed-since refresh json invalid: %v\n%s", err, changedSinceOut)
	}
	changedSinceError := changedSinceEnvelope["error"].(map[string]any)
	if changedSinceEnvelope["command"] != "index.refresh" || changedSinceEnvelope["status"] != "failed" || changedSinceError["code"] != "version_changed_paths_unavailable" {
		t.Fatalf("changed-since refresh envelope = %#v", changedSinceEnvelope)
	}

	managedPath := filepath.Join(root, "managed.md")
	writeCLIFixture(t, managedPath, strings.Replace(readCLIFile(t, managedPath), "managed body", "managed body changed", 1))
	staleOut := runCLI(t, "index", "refresh", "--vault", root, "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("refresh stale json invalid: %v\n%s", err, staleOut)
	}
	staleFacts := staleEnvelope["facts"].(map[string]any)
	if staleFacts["changed"] != "1" || staleFacts["indexed"] != "1" || staleFacts["index_status"] != "fresh" {
		t.Fatalf("refresh stale facts = %#v", staleFacts)
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "broken.md"), "---\nschema_version: pinax.note.v1\ntitle: Broken\n---\n\n# Broken\n")
	partialOut := runCLI(t, "index", "refresh", "--vault", root, "--json")
	var partialEnvelope map[string]any
	if err := json.Unmarshal([]byte(partialOut), &partialEnvelope); err != nil {
		t.Fatalf("refresh partial json invalid: %v\n%s", err, partialOut)
	}
	partialFacts := partialEnvelope["facts"].(map[string]any)
	if partialEnvelope["command"] != "index.refresh" || partialEnvelope["status"] != "partial" || partialFacts["failed"] != "1" || partialFacts["index_status"] != "partial" {
		t.Fatalf("refresh partial envelope = %#v", partialEnvelope)
	}
	if !strings.Contains(partialOut, "notes/broken.md") || !strings.Contains(partialOut, "pinax index doctor --vault") || !strings.Contains(partialOut, "pinax index rebuild --vault") {
		t.Fatalf("refresh partial missing evidence/actions:\n%s", partialOut)
	}
	partialAgent := runCLI(t, "index", "refresh", "--vault", root, "--agent")
	for _, want := range []string{"command=index.refresh", "status=partial", "fact.failed=1", "fact.index_status=partial", "action.doctor=", "action.rebuild="} {
		if !strings.Contains(partialAgent, want) {
			t.Fatalf("refresh partial agent missing %q:\n%s", want, partialAgent)
		}
	}
}

func TestIndexExplainCommandCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Explain Note", "--body", "explain body", "--slug", "explain", "--vault", root, "--json")

	help := runCLI(t, "index", "--help")
	if !strings.Contains(help, "explain") || !strings.Contains(help, "lookup") || !strings.Contains(help, "changed-since") {
		t.Fatalf("index help missing 7.6 commands/flags:\n%s", help)
	}

	out := runCLI(t, "index", "explain", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("index explain json invalid: %v\n%s", err, out)
	}
	facts := envelope["facts"].(map[string]any)
	if envelope["command"] != "index.explain" || facts["path"] != ".pinax/index.sqlite" || facts["index_status"] == "" {
		t.Fatalf("index explain envelope = %#v\n%s", envelope, out)
	}
	if !strings.Contains(out, "pinax search <query> --vault") && !strings.Contains(out, "pinax index refresh --vault") {
		t.Fatalf("index explain missing runnable action:\n%s", out)
	}
}

func TestIndexDoctorContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	runCLI(t, "note", "new", "Doctor Note", "--body", "doctor body", "--slug", "doctor", "--vault", root, "--json")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove index: %v", err)
	}

	missingOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var missingEnvelope map[string]any
	if err := json.Unmarshal([]byte(missingOut), &missingEnvelope); err != nil {
		t.Fatalf("doctor missing json invalid: %v\n%s", err, missingOut)
	}
	missingFacts := missingEnvelope["facts"].(map[string]any)
	if missingEnvelope["command"] != "index.doctor" || missingEnvelope["status"] != "partial" || missingFacts["issue_codes"] != "index_missing" || missingFacts["issues.total"] != "1" {
		t.Fatalf("doctor missing envelope = %#v", missingEnvelope)
	}
	if !strings.Contains(missingOut, "pinax index refresh --vault") || strings.Contains(missingOut, "delete .pinax/index.sqlite") {
		t.Fatalf("doctor missing action unsafe/missing:\n%s", missingOut)
	}

	runCLI(t, "index", "refresh", "--vault", root, "--json")
	freshOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var freshEnvelope map[string]any
	if err := json.Unmarshal([]byte(freshOut), &freshEnvelope); err != nil {
		t.Fatalf("doctor fresh json invalid: %v\n%s", err, freshOut)
	}
	freshFacts := freshEnvelope["facts"].(map[string]any)
	if freshEnvelope["status"] != "success" || freshFacts["index_status"] != "fresh" || freshFacts["issues.total"] != "0" {
		t.Fatalf("doctor fresh envelope = %#v\n%s", freshEnvelope, freshOut)
	}

	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	managedPath := filepath.Join(root, "doctor.md")
	writeCLIFixture(t, managedPath, strings.Replace(readCLIFile(t, managedPath), "doctor body", "doctor body changed", 1))
	staleOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var staleEnvelope map[string]any
	if err := json.Unmarshal([]byte(staleOut), &staleEnvelope); err != nil {
		t.Fatalf("doctor stale json invalid: %v\n%s", err, staleOut)
	}
	staleFacts := staleEnvelope["facts"].(map[string]any)
	if staleFacts["issue_codes"] != "index_stale" || !strings.Contains(staleOut, "changed_note=doctor.md") || !strings.Contains(staleOut, "pinax index refresh --vault") {
		t.Fatalf("doctor stale envelope = %#v\n%s", staleEnvelope, staleOut)
	}

	if err := os.WriteFile(indexPath, []byte("not sqlite"), 0o644); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}
	corruptOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var corruptEnvelope map[string]any
	if err := json.Unmarshal([]byte(corruptOut), &corruptEnvelope); err != nil {
		t.Fatalf("doctor corrupt json invalid: %v\n%s", err, corruptOut)
	}
	corruptFacts := corruptEnvelope["facts"].(map[string]any)
	if corruptFacts["issue_codes"] != "index_unreadable" || !strings.Contains(corruptOut, "pinax index repair --vault") || strings.Contains(corruptOut, "delete .pinax/index.sqlite") {
		t.Fatalf("doctor corrupt envelope = %#v\n%s", corruptEnvelope, corruptOut)
	}
	corruptAgent := runCLI(t, "index", "doctor", "--vault", root, "--agent")
	for _, want := range []string{"command=index.doctor", "status=partial", "fact.issue_codes=index_unreadable", "issue.1.code=index_unreadable", "action.repair="} {
		if !strings.Contains(corruptAgent, want) {
			t.Fatalf("doctor corrupt agent missing %q:\n%s", want, corruptAgent)
		}
	}
}

func TestIndexRepairContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Repair Note", "--body", "repair body", "--slug", "repair", "--vault", root, "--json")
	runCLI(t, "index", "rebuild", "--vault", root, "--json")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.WriteFile(indexPath, []byte("not sqlite"), 0o644); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}

	dryRunOut := runCLI(t, "index", "repair", "--vault", root, "--kind", "recreate", "--dry-run", "--json")
	var dryRunEnvelope map[string]any
	if err := json.Unmarshal([]byte(dryRunOut), &dryRunEnvelope); err != nil {
		t.Fatalf("repair dry-run json invalid: %v\n%s", err, dryRunOut)
	}
	dryRunFacts := dryRunEnvelope["facts"].(map[string]any)
	if dryRunEnvelope["command"] != "index.repair" || dryRunFacts["dry_run"] != "true" || dryRunFacts["writes"] != "false" || dryRunFacts["operations"] != "1" || !strings.Contains(dryRunOut, "recreate") {
		t.Fatalf("repair dry-run envelope = %#v\n%s", dryRunEnvelope, dryRunOut)
	}
	if got := readCLIFile(t, indexPath); got != "not sqlite" {
		t.Fatalf("dry-run modified index: %q", got)
	}

	failedOut, err := runCLIExpectError("index", "repair", "--vault", root, "--kind", "recreate", "--json")
	if err == nil || !strings.Contains(failedOut, "approval_required") {
		t.Fatalf("repair without approval err=%v out=%s", err, failedOut)
	}
	if got := readCLIFile(t, indexPath); got != "not sqlite" {
		t.Fatalf("approval failure modified index: %q", got)
	}

	applyOut := runCLI(t, "index", "repair", "--vault", root, "--kind", "recreate", "--yes", "--json")
	var applyEnvelope map[string]any
	if err := json.Unmarshal([]byte(applyOut), &applyEnvelope); err != nil {
		t.Fatalf("repair apply json invalid: %v\n%s", err, applyOut)
	}
	applyFacts := applyEnvelope["facts"].(map[string]any)
	if applyFacts["dry_run"] != "false" || applyFacts["writes"] != "true" || applyFacts["index_status"] != "fresh" || applyFacts["operations"] != "1" {
		t.Fatalf("repair apply facts = %#v", applyFacts)
	}
	if !strings.Contains(applyOut, ".pinax/index-backups/") {
		t.Fatalf("repair apply missing backup evidence:\n%s", applyOut)
	}
	doctorOut := runCLI(t, "index", "doctor", "--vault", root, "--json")
	var doctorEnvelope map[string]any
	if err := json.Unmarshal([]byte(doctorOut), &doctorEnvelope); err != nil {
		t.Fatalf("doctor after repair json invalid: %v\n%s", err, doctorOut)
	}
	if doctorEnvelope["status"] != "success" || doctorEnvelope["facts"].(map[string]any)["index_status"] != "fresh" {
		t.Fatalf("doctor after repair = %#v", doctorEnvelope)
	}
}

func TestIndexMachineOutputContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Machine Note", "--body", "machine body", "--slug", "machine", "--vault", root, "--json")
	indexPath := filepath.Join(root, ".pinax", "index.sqlite")
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove index: %v", err)
	}

	jsonStdout, jsonStderr, jsonErr := runCLISeparate("index", "doctor", "--vault", root, "--json")
	if jsonErr != nil || jsonStderr != "" {
		t.Fatalf("index doctor json err=%v stderr=%q stdout=%s", jsonErr, jsonStderr, jsonStdout)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonStdout), &envelope); err != nil {
		t.Fatalf("index doctor stdout is not JSON only: %v\n%s", err, jsonStdout)
	}
	if strings.Contains(jsonStdout, "\n\n") || strings.Contains(jsonStdout, "状态") || envelope["command"] != "index.doctor" {
		t.Fatalf("index doctor json contract invalid:\n%s", jsonStdout)
	}

	agentOut := runCLI(t, "index", "doctor", "--vault", root, "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=index.doctor", "status=partial", "fact.issue_codes=index_missing", "issue.1.code=index_missing"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("index doctor agent missing %q:\n%s", want, agentOut)
		}
	}
	for _, forbidden := range []string{"状态", "重点", "本地索引", "推荐下一步"} {
		if strings.Contains(agentOut, forbidden) {
			t.Fatalf("index doctor agent contains localized prose %q:\n%s", forbidden, agentOut)
		}
	}
	assertMachineOutputClean(t, agentOut)

	eventsOut := runCLI(t, "index", "repair", "--vault", root, "--kind", "recreate", "--dry-run", "--events")
	eventLines := strings.Split(strings.TrimSpace(eventsOut), "\n")
	if len(eventLines) != 2 {
		t.Fatalf("index repair events line count = %d\n%s", len(eventLines), eventsOut)
	}
	for i, line := range eventLines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("index repair event %d invalid: %v\n%s", i, err, eventsOut)
		}
	}
	if !strings.Contains(eventLines[0], `"type":"start"`) || !strings.Contains(eventLines[1], `"type":"end"`) || strings.Contains(eventsOut, "Status") || strings.Contains(eventsOut, "状态") {
		t.Fatalf("index repair events contract invalid:\n%s", eventsOut)
	}

	explainOut := runCLI(t, "index", "--vault", root, "--explain")
	for _, want := range []string{"Conclusion:", "Evidence:", "Confidence:", "Recommended next step:"} {
		if !strings.Contains(explainOut, want) {
			t.Fatalf("index explain missing %q:\n%s", want, explainOut)
		}
	}
	if !strings.Contains(explainOut, ".pinax/index.sqlite") || !strings.Contains(explainOut, "pinax index refresh --vault") {
		t.Fatalf("index explain missing evidence/action:\n%s", explainOut)
	}
}

func TestNotebookCoreOutputContractAndHelp(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "note", "new", "Contract Note", "--body", "body", "--tags", "contract", "--vault", root, "--json")

	rootHelp := runCLI(t, "--help")
	for _, want := range []string{"journal", "inbox", "view", "import", "export", "index", "organize"} {
		if !strings.Contains(rootHelp, want) {
			t.Fatalf("root help missing %q:\n%s", want, rootHelp)
		}
	}
	for _, hidden := range []string{"\n  daily ", "\n  weekly ", "\n  monthly "} {
		if strings.Contains(rootHelp, hidden) {
			t.Fatalf("root help should hide compatibility command %q:\n%s", strings.TrimSpace(hidden), rootHelp)
		}
	}
	noteHelp := runCLI(t, "note", "--help")
	for _, want := range []string{"links", "backlinks", "orphans", "attach", "attachments"} {
		if !strings.Contains(noteHelp, want) {
			t.Fatalf("note help missing %q:\n%s", want, noteHelp)
		}
	}
	searchHelp := runCLI(t, "search", "--help")
	for _, want := range []string{"--link-target", "--has-attachment", "--allow-stale"} {
		if !strings.Contains(searchHelp, want) {
			t.Fatalf("search help missing %q:\n%s", want, searchHelp)
		}
	}
	indexHelp := runCLI(t, "index", "--help")
	for _, want := range []string{"status", "refresh", "doctor", "rebuild", "sync", "repair", "low-cost maintenance", "full reset", "pinax index", "pinax index refresh --vault ./my-notes", "pinax index doctor --vault ./my-notes", "pinax index rebuild --vault ./my-notes"} {
		if !strings.Contains(indexHelp, want) {
			t.Fatalf("index help missing %q:\n%s", want, indexHelp)
		}
	}
	commandsStart := strings.Index(indexHelp, "Available Commands")
	commandsEnd := strings.Index(indexHelp, "Flags")
	if commandsStart < 0 || commandsEnd <= commandsStart {
		t.Fatalf("index help command section not found:\n%s", indexHelp)
	}
	commandSection := indexHelp[commandsStart:commandsEnd]
	last := -1
	for _, want := range []string{"status", "refresh", "doctor", "rebuild", "sync", "repair"} {
		pos := strings.Index(commandSection, want)
		if pos <= last {
			t.Fatalf("index help command %q is out of workflow order:\n%s", want, indexHelp)
		}
		last = pos
	}
	organizeHelp := runCLI(t, "organize", "--help")
	for _, want := range []string{"plan", "list", "apply"} {
		if !strings.Contains(organizeHelp, want) {
			t.Fatalf("organize help missing %q:\n%s", want, organizeHelp)
		}
	}

	humanOut := runCLI(t, "view", "list", "--vault", root)
	if !strings.Contains(humanOut, "Highlights") || strings.Contains(humanOut, "状态:") || strings.Contains(humanOut, "成功") || strings.HasPrefix(strings.TrimSpace(humanOut), "{") {
		t.Fatalf("view list human output invalid:\n%s", humanOut)
	}
	agentOut, err := runCLIExpectError("import", "markdown", filepath.Join(root, "missing"), "--dry-run", "--vault", root, "--agent")
	if err == nil {
		t.Fatalf("missing import source succeeded: %s", agentOut)
	}
	for _, want := range []string{"mode=agent", "command=import.markdown", "status=failed", "error.code=import_source_missing"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("import error agent output missing %q:\n%s", want, agentOut)
		}
	}
	if strings.Contains(agentOut, "状态:") || strings.Contains(agentOut, "重点:") || strings.Contains(agentOut, "raw") {
		t.Fatalf("agent output leaked prose/debug text:\n%s", agentOut)
	}
	jsonOut, err := runCLIExpectError("note", "attach", "Contract Note", filepath.Join(root, "missing.png"), "--vault", root, "--json")
	if err == nil {
		t.Fatalf("missing attachment succeeded: %s", jsonOut)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &envelope); err != nil {
		t.Fatalf("attachment error json invalid: %v\n%s", err, jsonOut)
	}
	errorObject := envelope["error"].(map[string]any)
	if envelope["command"] != "note.attach" || envelope["status"] != "failed" || errorObject["code"] != "attachment_source_missing" {
		t.Fatalf("attachment error envelope = %#v", envelope)
	}
}

func TestHumanOutputIsPolishedForNotebookViewsAndHelp(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "project", "create", "work", "--name", "Work", "--notes-prefix", "notes/work", "--vault", root, "--json")
	runCLI(t, "project", "create", "personal", "--name", "Personal", "--notes-prefix", "notes/personal", "--vault", root, "--json")
	runCLI(t, "note", "new", "Work Note", "--group", "work", "--kind", "reference", "--tags", "work", "--body", "body", "--vault", root, "--json")
	runCLI(t, "note", "new", "Personal Note", "--group", "personal", "--kind", "reference", "--tags", "personal", "--body", "body", "--vault", root, "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "raw.md"), "# Raw Note\n\nbody\n")

	groupOut := runCLI(t, "group", "list", "--vault", root)
	for _, want := range []string{"━━━━━━━━", "────────", "Highlights", "Metric", "Value", "Group", "Count", "work", "personal"} {
		if !strings.Contains(groupOut, want) {
			t.Fatalf("group list polished output missing %q:\n%s", want, groupOut)
		}
	}
	for _, old := range []string{"摘要", "统计", "列表", "状态:", "事实:", "dimension=group, dimensions=", "dimension=group"} {
		if strings.Contains(groupOut, old) {
			t.Fatalf("group list still uses old prose %q:\n%s", old, groupOut)
		}
	}

	helpOut := runCLI(t, "metadata")
	for _, want := range []string{"Summary", "Usage", "Available Commands", "Flags", "Global Flags", "pinax metadata [command] --help"} {
		if !strings.Contains(helpOut, want) {
			t.Fatalf("metadata help missing %q:\n%s", want, helpOut)
		}
	}
	for _, old := range []string{"简介", "用法", "可用命令", "参数", "全局参数"} {
		if strings.Contains(helpOut, old) {
			t.Fatalf("metadata help still contains English cobra heading %q:\n%s", old, helpOut)
		}
	}

	writeCLIFixture(t, filepath.Join(root, "notes", "needs-metadata.md"), "---\nschema_version: pinax.note.v1\ntitle: Needs Metadata\n---\n\n# Needs Metadata\n\nbody\n")
	planOut := runCLI(t, "metadata", "plan", "--vault", root)
	for _, want := range []string{"━━━━━━━━", "────────", "Highlights", "Metadata plan generated.", "Metric", "Value", "Planned updates", "Next step"} {
		if !strings.Contains(planOut, want) {
			t.Fatalf("metadata plan polished output missing %q:\n%s", want, planOut)
		}
	}
	for _, old := range []string{"Pinax", "摘要", "统计", "状态:", "重点:", "事实: planned_updates=", "planned_updates="} {
		if strings.Contains(planOut, old) {
			t.Fatalf("metadata plan still uses old prose %q:\n%s", old, planOut)
		}
	}
}

func TestBackendProviderCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	help := runCLI(t, "backend", "--help")
	for _, want := range []string{"list", "add", "show", "doctor", "capabilities", "diff", "push", "pull", "remove", "object"} {
		if !strings.Contains(help, want) {
			t.Fatalf("backend help missing %q:\n%s", want, help)
		}
	}

	// backend add s3
	addOut := runCLI(t, "backend", "add", "s3", "work-s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax/", "--profile", "work", "--vault", root, "--json")
	var addEnvelope map[string]any
	if err := json.Unmarshal([]byte(addOut), &addEnvelope); err != nil {
		t.Fatalf("backend add json invalid: %v\n%s", err, addOut)
	}
	if addEnvelope["command"] != "backend.add" || addEnvelope["status"] != "success" {
		t.Fatalf("backend add envelope = %#v", addEnvelope)
	}
	addFacts := addEnvelope["facts"].(map[string]any)
	if addFacts["name"] != "work-s3" || addFacts["kind"] != "s3" {
		t.Fatalf("backend add facts = %#v", addFacts)
	}
	if strings.Contains(strings.ToLower(addOut), "secret") || strings.Contains(strings.ToLower(addOut), "access_key") {
		t.Fatalf("backend add output leaked secret-like material:\n%s", addOut)
	}

	// backend list
	listOut := runCLI(t, "backend", "list", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("backend list json invalid: %v\n%s", err, listOut)
	}
	if listEnvelope["command"] != "backend.list" || listEnvelope["status"] != "success" {
		t.Fatalf("backend list envelope = %#v", listEnvelope)
	}
	listFacts := listEnvelope["facts"].(map[string]any)
	if listFacts["backends"] != "1" || listFacts["default_backend"] != "work-s3" {
		t.Fatalf("backend list facts = %#v", listFacts)
	}

	// backend ls is the short alias for backend list, not object listing.
	lsAliasOut := runCLI(t, "backend", "ls", "--vault", root, "--json")
	var lsAliasEnvelope map[string]any
	if err := json.Unmarshal([]byte(lsAliasOut), &lsAliasEnvelope); err != nil {
		t.Fatalf("backend ls alias json invalid: %v\n%s", err, lsAliasOut)
	}
	if lsAliasEnvelope["command"] != "backend.list" || lsAliasEnvelope["status"] != "success" {
		t.Fatalf("backend ls alias envelope = %#v", lsAliasEnvelope)
	}
	if lsAliasEnvelope["facts"].(map[string]any)["backends"] != "1" {
		t.Fatalf("backend ls alias facts = %#v", lsAliasEnvelope["facts"])
	}

	legacyLS, err := runCLIExpectError("backend", "ls", "--name", "work-s3", "--vault", root, "--json")
	if err == nil || !strings.Contains(legacyLS, "flag_error") {
		t.Fatalf("backend ls --name should not keep legacy object semantics err=%v out=%s", err, legacyLS)
	}

	objectListMissing, err := runCLIExpectError("backend", "object", "list", "--vault", root, "--json")
	if err == nil || !strings.Contains(objectListMissing, "argument_required") || !strings.Contains(objectListMissing, "backend object list <name> [prefix]") {
		t.Fatalf("backend object list missing name err=%v out=%s", err, objectListMissing)
	}

	// backend show
	statusOut := runCLI(t, "backend", "show", "work-s3", "--vault", root, "--json")
	var statusEnvelope map[string]any
	if err := json.Unmarshal([]byte(statusOut), &statusEnvelope); err != nil {
		t.Fatalf("backend show json invalid: %v\n%s", err, statusOut)
	}
	if statusEnvelope["command"] != "backend.show" {
		t.Fatalf("backend show envelope = %#v", statusEnvelope)
	}

	// backend doctor
	doctorOut := runCLI(t, "backend", "doctor", "work-s3", "--vault", root, "--json")
	var doctorEnvelope map[string]any
	if err := json.Unmarshal([]byte(doctorOut), &doctorEnvelope); err != nil {
		t.Fatalf("backend doctor json invalid: %v\n%s", err, doctorOut)
	}
	if doctorEnvelope["command"] != "backend.doctor" {
		t.Fatalf("backend doctor envelope = %#v", doctorEnvelope)
	}

	// backend capabilities
	capOut := runCLI(t, "backend", "capabilities", "work-s3", "--vault", root, "--agent")
	for _, want := range []string{"command=backend.capabilities", "status=success", "fact.name=work-s3", "fact.kind=s3"} {
		if !strings.Contains(capOut, want) {
			t.Fatalf("backend capabilities agent output missing %q:\n%s", want, capOut)
		}
	}

	// backend diff
	diffOut := runCLI(t, "backend", "diff", "work-s3", "--vault", root, "--json")
	var diffEnvelope map[string]any
	if err := json.Unmarshal([]byte(diffOut), &diffEnvelope); err != nil {
		t.Fatalf("backend diff json invalid: %v\n%s", err, diffOut)
	}
	if diffEnvelope["command"] != "backend.diff" {
		t.Fatalf("backend diff envelope = %#v", diffEnvelope)
	}

	// backend push dry-run
	pushDryRun := runCLI(t, "backend", "push", "work-s3", "--dry-run", "--vault", root, "--json")
	if !strings.Contains(pushDryRun, "backend.push") || !strings.Contains(pushDryRun, `"dry_run":true`) {
		t.Fatalf("backend push dry-run output invalid:\n%s", pushDryRun)
	}

	// backend push without approval
	pushFail, err := runCLIExpectError("backend", "push", "work-s3", "--vault", root, "--json")
	if err == nil || !strings.Contains(pushFail, "approval_required") {
		t.Fatalf("backend push without approval err=%v out=%s", err, pushFail)
	}

	// backend add rclone
	rcloneOut := runCLI(t, "backend", "add", "rclone", "work-drive", "--remote", "workdrive:pinax", "--vault", root, "--json")
	if !strings.Contains(rcloneOut, "backend.add") {
		t.Fatalf("backend add rclone output = %s", rcloneOut)
	}

	// backend remove
	removeOut := runCLI(t, "backend", "remove", "work-drive", "--vault", root, "--json")
	if !strings.Contains(removeOut, "backend.remove") {
		t.Fatalf("backend remove output = %s", removeOut)
	}
	// verify removed
	listAfterRemove := runCLI(t, "backend", "list", "--vault", root, "--json")
	var listAfterEnvelope map[string]any
	if err := json.Unmarshal([]byte(listAfterRemove), &listAfterEnvelope); err != nil {
		t.Fatalf("backend list after remove json invalid: %v\n%s", err, listAfterRemove)
	}
	if listAfterEnvelope["facts"].(map[string]any)["backends"] != "1" {
		t.Fatalf("expected 1 backend after remove: %s", listAfterRemove)
	}

	// backend add without name
	noName, err := runCLIExpectError("backend", "add", "s3", "--bucket", "b", "--region", "r", "--vault", root, "--json")
	if err == nil || !strings.Contains(noName, "argument_required") {
		t.Fatalf("backend add without name err=%v out=%s", err, noName)
	}

	// backend add invalid kind
	badKind, err := runCLIExpectError("backend", "add", "ftp", "x", "--vault", root, "--json")
	if err == nil || !strings.Contains(badKind, "backend_kind_invalid") {
		t.Fatalf("backend add invalid kind err=%v out=%s", err, badKind)
	}

	// backend add s3 missing required fields
	missingS3, err := runCLIExpectError("backend", "add", "s3", "bad-s3", "--vault", root, "--json")
	if err == nil || !strings.Contains(missingS3, "backend_config_incomplete") {
		t.Fatalf("backend add s3 missing fields err=%v out=%s", err, missingS3)
	}

	// backend show not found
	notFound, err := runCLIExpectError("backend", "show", "nonexistent", "--vault", root, "--json")
	if err == nil || !strings.Contains(notFound, "backend_not_found") {
		t.Fatalf("backend show not found err=%v out=%s", err, notFound)
	}

	// legacy storage compatibility: storage commands still work
	storageOut := runCLI(t, "storage", "set-s3", "--bucket", "legacy-bucket", "--region", "us-east-1", "--vault", root, "--json")
	if !strings.Contains(storageOut, "storage.set_s3") {
		t.Fatalf("storage set-s3 still works:\n%s", storageOut)
	}
}

func TestFeishuDeliveryCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out := runCLI(t, "briefing", "deliver", "feishu", "--webhook", "https://open.feishu.cn/open-apis/bot/v2/hook/raw-token", "--secret-ref", "env://FEISHU_WEBHOOK", "--title", "Daily briefing", "--text", "AI tooling update", "--dry-run", "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "briefing.deliver.feishu", "success")
	if strings.Contains(out, "raw-token") || strings.Contains(out, "FEISHU_WEBHOOK") {
		t.Fatalf("feishu dry-run leaked secret:\n%s", out)
	}
	if !strings.Contains(out, "\"remote_write\":\"false\"") {
		t.Fatalf("feishu dry-run missing remote_write false:\n%s", out)
	}
}

func TestBriefingRecipeCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	initOut := runCLI(t, "briefing", "recipe", "init", "--vault", root, "--json")
	assertJSONCommandStatus(t, initOut, "briefing.recipe.init", "success")
	if !strings.Contains(initOut, "AI research") || strings.Contains(initOut, "webhook") {
		t.Fatalf("recipe init output invalid:\n%s", initOut)
	}
	setOut := runCLI(t, "briefing", "recipe", "set", "--topic", "AI tooling", "--limit", "7", "--source", "fake:ai", "--vault", root, "--json")
	assertJSONCommandStatus(t, setOut, "briefing.recipe.set", "success")
	showOut := runCLI(t, "briefing", "recipe", "show", "--vault", root, "--agent")
	for _, want := range []string{"command=briefing.recipe.show", "fact.topic=\"AI tooling\"", "fact.limit=7", "fact.sources=2"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("recipe show missing %q:\n%s", want, showOut)
		}
	}
}

func TestCloudOutputContractModes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha\nsecret-token body\n")
	runCLI(t, "cloud", "login", "--endpoint", "https://cloud.example.test", "--workspace", "ws_123", "--device", "dev_laptop", "--secret-ref", "op://pinax/cloud-token", "--vault", root, "--json")
	agentOut := runCLI(t, "cloud", "status", "--vault", root, "--agent")
	for _, want := range []string{"spec_version=1.0", "mode=agent", "command=cloud.status", "status=success", "fact.configured=true", "fact.session_status=active"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("cloud status --agent missing %q:\n%s", want, agentOut)
		}
	}
	assertMachineOutputClean(t, agentOut)
	eventsOut := runCLI(t, "cloud", "doctor", "--vault", root, "--events")
	assertNDJSONEvents(t, eventsOut, "cloud.doctor")
	assertMachineOutputClean(t, eventsOut)
	explainOut := runCLI(t, "sync", "push", "--target", "cloud", "--dry-run", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--explain")
	if !strings.Contains(explainOut, "Conclusion") || !strings.Contains(explainOut, "Evidence") || strings.Contains(explainOut, "secret-token") || strings.Contains(explainOut, "cloud-token") {
		t.Fatalf("sync explain contract invalid:\n%s", explainOut)
	}
	conflict, err := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_2", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("conflict sync succeeded: %s", conflict)
	}
	assertJSONErrorCode(t, conflict, "REVISION_CONFLICT")
	assertMachineOutputClean(t, conflict)
}

func assertNDJSONEvents(t *testing.T, out, command string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("events output too short:\n%s", out)
	}
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event json invalid: %v\n%s", err, line)
		}
		if event["command"] != command {
			t.Fatalf("event command = %#v want %s", event, command)
		}
	}
}

func TestSyncRunReceiptsLogsStatusAndRedactionCLI(t *testing.T) {
	root := t.TempDir()
	objectRoot := t.TempDir()
	rawPath := "notes/raw-secret-path.md"
	forbidden := []string{"PLAINTEXT_NOTE_BODY", "raw-secret-path.md", "raw-token-123", "Authorization", "Cookie", "op://pinax/secret-ref", "provider payload", "provider stderr"}
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, rawPath), "# Secret\n\nPLAINTEXT_NOTE_BODY Authorization: Bearer raw-token-123 Cookie: session=abc provider payload provider stderr\n")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "ws_secret", "--device", "dev_secret", "--secret-ref", "op://pinax/secret-ref", "--vault", root, "--json")

	stdout, stderr, err := runCLISeparate("sync", "push", "--target", "cloud", "--yes", "--path-policy", "hash", "--vault", root, "--json")
	if err != nil || stderr != "" {
		t.Fatalf("sync push err=%v stderr=%q stdout=%s", err, stderr, stdout)
	}
	assertJSONCommandStatus(t, stdout, "sync.push", "success")
	assertNoForbiddenSyncLeak(t, stdout+stderr, forbidden)

	statePath := filepath.Join(root, ".pinax", "sync-state.json")
	state := readCLIFile(t, statePath)
	assertNoForbiddenSyncLeak(t, state, forbidden)
	var stateJSON map[string]any
	if err := json.Unmarshal([]byte(state), &stateJSON); err != nil {
		t.Fatalf("sync-state json invalid: %v\n%s", err, state)
	}
	runID, _ := stateJSON["last_sync_run_id"].(string)
	if stateJSON["schema_version"] != "pinax.sync_state.v1" || runID == "" || stateJSON["last_synced_revision"] == "" || stateJSON["runs"] != nil || stateJSON["remote_write"] != nil {
		t.Fatalf("sync-state not current-state only: %#v", stateJSON)
	}

	receiptPath := filepath.Join(root, ".pinax", "sync-runs", time.Now().UTC().Format("2006"), time.Now().UTC().Format("01"), runID+".json")
	receipt := readCLIFile(t, receiptPath)
	assertNoForbiddenSyncLeak(t, receipt, forbidden)
	var receiptJSON map[string]any
	if err := json.Unmarshal([]byte(receipt), &receiptJSON); err != nil {
		t.Fatalf("receipt json invalid: %v\n%s", err, receipt)
	}
	for _, key := range []string{"run_id", "command", "target", "direction", "status", "remote_write", "local_write", "backend_kind", "transport", "workspace_id", "vault_id", "device_id", "request_id", "revision_id", "manifest_blob_id", "counts", "timings_ms", "actions", "redaction", "created_at"} {
		if _, ok := receiptJSON[key]; !ok {
			t.Fatalf("receipt missing %s: %#v", key, receiptJSON)
		}
	}
	if receiptJSON["schema_version"] != "pinax.sync_run.v1" || receiptJSON["status"] != "success" || receiptJSON["remote_write"] != true {
		t.Fatalf("receipt schema/status invalid: %#v", receiptJSON)
	}

	events := readCLIFile(t, filepath.Join(root, ".pinax", "events.jsonl"))
	assertNoForbiddenSyncLeak(t, events, forbidden)
	if !strings.Contains(events, runID) || strings.Contains(events, "manifest_blob_id") || strings.Contains(events, "provider payload") {
		t.Fatalf("events are not a safe run-linked summary:\n%s", events)
	}

	var objectText strings.Builder
	if err := filepath.WalkDir(objectRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		objectText.WriteString(filepath.ToSlash(path))
		objectText.WriteByte('\n')
		objectText.WriteString(readCLIFile(t, path))
		objectText.WriteByte('\n')
		return nil
	}); err != nil {
		t.Fatalf("walk object store: %v", err)
	}
	assertNoForbiddenSyncLeak(t, objectText.String(), forbidden)

	for _, mode := range [][]string{{"--json"}, {"--agent"}, {"--events"}, {"--explain"}} {
		out := runCLI(t, append([]string{"sync", "logs", "show", runID, "--vault", root}, mode...)...)
		assertNoForbiddenSyncLeak(t, out, forbidden)
		if !strings.Contains(out, runID) {
			t.Fatalf("logs show %v missing run id:\n%s", mode, out)
		}
	}
	for _, args := range [][]string{
		{"sync", "logs", "list", "--vault", root},
		{"sync", "logs", "tail", "--limit", "5", "--vault", root},
		{"sync", "logs", "prune", "--keep", "200", "--vault", root},
	} {
		for _, mode := range []string{"--json", "--agent", "--events", "--explain"} {
			out := runCLI(t, append(args, mode)...)
			assertNoForbiddenSyncLeak(t, out, forbidden)
		}
	}
	statusOut := runCLI(t, "sync", "status", "--vault", root, "--json")
	assertNoForbiddenSyncLeak(t, statusOut, forbidden)
	if !strings.Contains(statusOut, runID) {
		t.Fatalf("sync status output missing run id:\n%s", statusOut)
	}
}

func TestSyncRunReceiptsCoverPartialFailedApprovalAndPruneCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha\nbody\n")
	runCLI(t, "cloud", "login", "--endpoint", "https://cloud.example.test", "--workspace", "ws", "--device", "dev", "--secret-ref", "env://PINAX_SECRET", "--vault", root, "--json")

	approvalOut, approvalErr := runCLIExpectError("sync", "push", "--target", "cloud", "--vault", root, "--json")
	if approvalErr == nil || !strings.Contains(approvalOut, "approval_required") {
		t.Fatalf("approval-required sync output invalid err=%v out=%s", approvalErr, approvalOut)
	}
	unavailableOut, unavailableErr := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	if unavailableErr == nil || !strings.Contains(unavailableOut, "cloud_secret_unavailable") || strings.Contains(unavailableOut, "PINAX_SECRET") {
		t.Fatalf("unavailable sync output invalid err=%v out=%s", unavailableErr, unavailableOut)
	}
	failedOut, failedErr := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_2", "--vault", root, "--json")
	if failedErr == nil || !strings.Contains(failedOut, "REVISION_CONFLICT") {
		t.Fatalf("failed conflict sync output invalid err=%v out=%s", failedErr, failedOut)
	}

	logsOut := runCLI(t, "sync", "logs", "list", "--vault", root, "--json")
	for _, want := range []string{"approval_required", "cloud_secret_unavailable", "failed", "pinax.sync_run.v1"} {
		if !strings.Contains(logsOut, want) {
			t.Fatalf("logs list missing %q:\n%s", want, logsOut)
		}
	}
	preview := runCLI(t, "sync", "logs", "prune", "--keep", "1", "--vault", root, "--json")
	if !strings.Contains(preview, "\"dry_run\":true") || !strings.Contains(preview, "delete_candidates") {
		t.Fatalf("prune preview invalid:\n%s", preview)
	}
	pruned := runCLI(t, "sync", "logs", "prune", "--keep", "1", "--yes", "--vault", root, "--json")
	if !strings.Contains(pruned, "\"deleted\":") {
		t.Fatalf("prune apply invalid:\n%s", pruned)
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "sync-state.json")); err != nil {
		t.Fatalf("prune deleted sync-state: %v", err)
	}
}

func assertNoForbiddenSyncLeak(t *testing.T, got string, forbidden []string) {
	t.Helper()
	for _, value := range forbidden {
		if strings.Contains(got, value) {
			t.Fatalf("sync output leaked %q:\n%s", value, got)
		}
	}
}

func TestSyncRunPathRedactionPoliciesCLI(t *testing.T) {
	for _, tc := range []struct {
		policy      string
		wantPath    bool
		wantHash    bool
		wantOmitted bool
	}{
		{policy: "default", wantPath: true},
		{policy: "hash", wantHash: true},
		{policy: "omitted", wantOmitted: true},
	} {
		t.Run(tc.policy, func(t *testing.T) {
			root := t.TempDir()
			runCLI(t, "init", root, "--title", "Vault", "--json")
			writeCLIFixture(t, filepath.Join(root, "notes", "policy-secret.md"), "# Policy\nbody\n")
			runCLI(t, "cloud", "login", "--endpoint", "https://cloud.example.test", "--workspace", "ws", "--device", "dev", "--secret-ref", "env://PINAX_SECRET", "--vault", root, "--json")
			out := runCLI(t, "sync", "diff", "--target", "cloud", "--path-policy", tc.policy, "--vault", root, "--json")
			state := readCLIFile(t, filepath.Join(root, ".pinax", "sync-state.json"))
			var stateJSON map[string]any
			if err := json.Unmarshal([]byte(state), &stateJSON); err != nil {
				t.Fatalf("state json invalid: %v", err)
			}
			runID := stateJSON["last_sync_run_id"].(string)
			receipt := readCLIFile(t, filepath.Join(root, ".pinax", "sync-runs", time.Now().UTC().Format("2006"), time.Now().UTC().Format("01"), runID+".json"))
			combined := out + receipt
			if tc.wantPath && !strings.Contains(combined, "notes/policy-secret.md") {
				t.Fatalf("default policy omitted path:\n%s", combined)
			}
			if !tc.wantPath && strings.Contains(combined, "policy-secret.md") {
				t.Fatalf("%s policy leaked path:\n%s", tc.policy, combined)
			}
			if tc.wantHash && !strings.Contains(combined, "path_sha256:") {
				t.Fatalf("hash policy missing hash:\n%s", combined)
			}
			if tc.wantOmitted && strings.Contains(combined, "path_sha256:") {
				t.Fatalf("omitted policy kept hash:\n%s", combined)
			}
		})
	}
}

func TestSyncCloudPlannerCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "# Alpha\nbody\n")
	serverRevision := "rev_1"
	writeServerJSON := func(w http.ResponseWriter, status int, payload any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("write server json: %v", err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer cloud-token" {
			t.Fatalf("server transport authorization header = %q", got)
		}
		workspacePath := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(workspacePath, "/revision"):
			writeServerJSON(w, http.StatusOK, map[string]any{"revision_id": serverRevision, "manifest_blob_id": "manifest_initial"})
		case r.Method == http.MethodPost && strings.HasSuffix(workspacePath, "/blobs:batchCheck"):
			writeServerJSON(w, http.StatusOK, map[string]any{"missing_blob_ids": []string{"blob_"}})
		case r.Method == http.MethodPut && strings.Contains(workspacePath, "/blobs/"):
			writeServerJSON(w, http.StatusCreated, map[string]any{"status": "stored"})
		case r.Method == http.MethodPost && strings.HasSuffix(workspacePath, "/revisions:commit"):
			if got := r.Header.Get("Idempotency-Key"); got == "" {
				t.Fatalf("server transport missing idempotency key")
			}
			serverRevision = "rev_server"
			writeServerJSON(w, http.StatusOK, map[string]any{"revision_id": serverRevision, "manifest_blob_id": "manifest_server"})
		default:
			t.Fatalf("unexpected server transport request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	runCLI(t, "cloud", "login", "--endpoint", server.URL, "--workspace", "ws_123", "--device", "dev_laptop", "--secret-ref", "plain:cloud-token", "--vault", root, "--json")
	diffOut := runCLI(t, "sync", "diff", "--target", "cloud", "--dry-run", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, diffOut, "sync.diff", "success")
	if !strings.Contains(diffOut, "\"dry_run\":\"true\"") || !strings.Contains(diffOut, "upload_blob") {
		t.Fatalf("sync diff missing dry-run plan:\n%s", diffOut)
	}
	pushDryRun := runCLI(t, "sync", "push", "--target", "cloud", "--dry-run", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, pushDryRun, "sync.push", "success")
	if !strings.Contains(pushDryRun, "upload_manifest") || strings.Contains(pushDryRun, "\"remote_write\":true") {
		t.Fatalf("sync push dry-run plan invalid:\n%s", pushDryRun)
	}
	pushApply := runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, pushApply, "sync.push", "success")
	if !strings.Contains(pushApply, "\"remote_write\":true") || strings.Contains(pushApply, "cloud_api_unimplemented") || !strings.Contains(pushApply, "rev_server") {
		t.Fatalf("sync push --yes did not use server transport durable commit:\n%s", pushApply)
	}

	objectRoot := t.TempDir()
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "ws_file", "--device", "dev_file", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", root, "--json")
	directPush := runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, directPush, "sync.push", "success")
	if !strings.Contains(directPush, "\"remote_write\":true") || strings.Contains(directPush, "cloud_api_unimplemented") {
		t.Fatalf("direct cloud push did not complete durable remote write:\n%s", directPush)
	}
	stateReceipt := readCLIFile(t, filepath.Join(root, ".pinax", "sync-state.json"))
	if !strings.Contains(stateReceipt, "\"last_sync_run_id\"") || !strings.Contains(stateReceipt, "\"backend_kind\": \"embedded\"") || strings.Contains(stateReceipt, "\"remote_write\"") {
		t.Fatalf("direct cloud push current state invalid:\n%s", stateReceipt)
	}

	profileRoot := filepath.Join(root, "xdg")
	t.Setenv("XDG_CONFIG_HOME", profileRoot)
	runCLI(t, "profile", "add", "cloud-work", "--endpoint", server.URL, "--workspace", "ws_profile", "--device", "dev_profile", "--secret-ref", "plain:cloud-token")
	profilePush := runCLI(t, "sync", "push", "--target", "cloud-work", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, profilePush, "sync.push", "success")
	if !strings.Contains(profilePush, "ws_profile") || !strings.Contains(profilePush, "dev_profile") || !strings.Contains(profilePush, "\"remote_write\":true") || strings.Contains(profilePush, "cloud_api_unimplemented") {
		t.Fatalf("sync push with profile target did not use server transport:\n%s", profilePush)
	}
	pullDryRun := runCLI(t, "sync", "pull", "--target", "cloud", "--dry-run", "--base-revision", "rev_1", "--remote-revision", "rev_1", "--vault", root, "--json")
	assertJSONCommandStatus(t, pullDryRun, "sync.pull", "success")
	if !strings.Contains(pullDryRun, "download_manifest") {
		t.Fatalf("sync pull dry-run plan invalid:\n%s", pullDryRun)
	}
	conflict, err := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_2", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("conflict push succeeded: %s", conflict)
	}
	assertJSONErrorCode(t, conflict, "REVISION_CONFLICT")
	for _, want := range []string{"pinax sync conflicts list --vault " + root + " --json", "pinax sync conflicts diff <file>", "pinax sync conflicts resolve <file>"} {
		if !strings.Contains(conflict, want) {
			t.Fatalf("revision conflict json missing action %q:\n%s", want, conflict)
		}
	}
	conflictAgent, agentErr := runCLIExpectError("sync", "push", "--target", "cloud", "--yes", "--base-revision", "rev_1", "--remote-revision", "rev_2", "--vault", root, "--agent")
	if agentErr == nil {
		t.Fatalf("conflict push agent succeeded: %s", conflictAgent)
	}
	for _, want := range []string{"command=sync.push", "error.code=REVISION_CONFLICT", "action.list=", "pinax sync conflicts list --vault " + root + " --json", "action.diff=", "pinax sync conflicts diff <file>", "action.resolve=", "pinax sync conflicts resolve <file>"} {
		if !strings.Contains(conflictAgent, want) {
			t.Fatalf("revision conflict agent missing action %q:\n%s", want, conflictAgent)
		}
	}
}

func TestSyncTargetCompletionAndInitUsesExistingCloudConfigCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	targetCompletion := runCLI(t, "__complete", "sync", "pull", "--target", "")
	for _, want := range []string{"cloud\tconfigured Cloud Sync backend", "s3\tS3-compatible direct backend", "git\tGit backend"} {
		if !strings.Contains(targetCompletion, want) {
			t.Fatalf("sync target completion missing %q:\n%s", want, targetCompletion)
		}
	}
	runCLI(t, "cloud", "backend", "set", "s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax-sync/", "--endpoint", "http://127.0.0.1:9000", "--workspace", "ec", "--device", "dev", "--vault", root, "--json")
	initOut := runCLI(t, "sync", "init", "--vault", root, "--json")
	assertJSONCommandStatus(t, initOut, "sync.init", "success")
	for _, want := range []string{"\"backend_kind\":\"s3-direct\"", "s3://notes/pinax-sync", "\"workspace\":\"ec\"", "\"device\":\"dev\""} {
		if !strings.Contains(initOut, want) {
			t.Fatalf("sync init did not reuse cloud config %q:\n%s", want, initOut)
		}
	}
}

func TestCloudStateCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	missing, err := runCLIExpectError("cloud", "status", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("cloud status without config succeeded: %s", missing)
	}
	assertJSONErrorCode(t, missing, "cloud_not_configured")
	loginOut := runCLI(t, "cloud", "login", "--endpoint", "https://cloud.example.test", "--workspace", "ws_123", "--device", "dev_laptop", "--secret-ref", "op://pinax/cloud-token", "--vault", root, "--json")
	if strings.Contains(loginOut, "cloud-token") || strings.Contains(loginOut, "Authorization") {
		t.Fatalf("cloud login leaked secret reference/token:\n%s", loginOut)
	}
	assertJSONCommandStatus(t, loginOut, "cloud.login", "success")
	statusOut := runCLI(t, "cloud", "status", "--vault", root, "--json")
	assertJSONCommandStatus(t, statusOut, "cloud.status", "success")
	if !strings.Contains(statusOut, "\"configured\":\"true\"") || !strings.Contains(statusOut, "dev_laptop") {
		t.Fatalf("cloud status missing facts:\n%s", statusOut)
	}
	doctorOut := runCLI(t, "cloud", "doctor", "--vault", root, "--json")
	assertJSONCommandStatus(t, doctorOut, "cloud.doctor", "success")
	logoutOut := runCLI(t, "cloud", "logout", "--vault", root, "--json")
	assertJSONCommandStatus(t, logoutOut, "cloud.logout", "success")
	loggedOut := runCLI(t, "cloud", "status", "--vault", root, "--json")
	if !strings.Contains(loggedOut, "logged_out") {
		t.Fatalf("cloud status after logout missing logged_out:\n%s", loggedOut)
	}
}

func TestCloudBackendSetS3CLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out := runCLI(t, "cloud", "backend", "set", "s3", "--bucket", "notes", "--region", "us-east-1", "--prefix", "pinax-sync/", "--endpoint", "http://10.10.1.102:9010", "--profile", "work", "--workspace", "personal", "--device", "laptop", "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "cloud.backend.set", "success")
	for _, want := range []string{"\"backend_kind\":\"s3-direct\"", "s3://notes/pinax-sync", "\"s3\":{", "\"endpoint\":\"http://10.10.1.102:9010\"", "\"path_style\":true", "personal", "laptop"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cloud backend set s3 missing %q:\n%s", want, out)
		}
	}
	for _, leaked := range []string{"access_key", "secret_key", "Authorization", "Cookie", "AKIA", "refresh_token"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("cloud backend set s3 leaked %q:\n%s", leaked, out)
		}
	}
	configYAML := readCLIFile(t, filepath.Join(root, ".pinax", "cloud", "config.yaml"))
	for _, want := range []string{"backend_kind: s3-direct", "bucket: notes", "prefix: pinax-sync/", "endpoint: http://10.10.1.102:9010", "profile: work", "path_style: true", "secret_ref: profile://work"} {
		if !strings.Contains(configYAML, want) {
			t.Fatalf("cloud yaml config missing %q:\n%s", want, configYAML)
		}
	}
	for _, escaped := range []string{"http%3A", "?endpoint=", "&profile="} {
		if strings.Contains(configYAML, escaped) {
			t.Fatalf("cloud yaml config contains escaped endpoint fragment %q:\n%s", escaped, configYAML)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".pinax", "cloud", "config.json")); !os.IsNotExist(err) {
		t.Fatalf("cloud backend set s3 should write config.yaml as primary config, err=%v", err)
	}
	status := runCLI(t, "cloud", "status", "--vault", root, "--json")
	assertJSONCommandStatus(t, status, "cloud.status", "success")
	if !strings.Contains(status, "\"backend_kind\":\"s3-direct\"") || !strings.Contains(status, "s3://notes/pinax-sync") {
		t.Fatalf("cloud status missing s3 backend facts:\n%s", status)
	}
	doctor := runCLI(t, "cloud", "doctor", "--vault", root, "--json")
	assertJSONCommandStatus(t, doctor, "cloud.doctor", "success")
	for _, want := range []string{"\"backend_kind\":\"s3-direct\"", "\"auth_boundary\":\"provider_credentials\"", "\"server_audit\":false"} {
		if !strings.Contains(doctor, want) {
			t.Fatalf("cloud doctor missing direct boundary %q:\n%s", want, doctor)
		}
	}
}

func TestCloudBackendSetRcloneCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out := runCLI(t, "cloud", "backend", "set", "rclone", "--remote", "onedrive:PinaxSync", "--workspace", "personal", "--device", "laptop", "--vault", root, "--json")
	assertJSONCommandStatus(t, out, "cloud.backend.set", "success")
	for _, want := range []string{"\"backend_kind\":\"rclone-direct\"", "rclone://onedrive/PinaxSync", "personal", "laptop"} {
		if !strings.Contains(out, want) {
			t.Fatalf("cloud backend set rclone missing %q:\n%s", want, out)
		}
	}
	for _, leaked := range []string{"refresh_token", "Authorization", "Cookie", "client_secret"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("cloud backend set rclone leaked %q:\n%s", leaked, out)
		}
	}
	doctor := runCLI(t, "cloud", "doctor", "--vault", root, "--json")
	assertJSONCommandStatus(t, doctor, "cloud.doctor", "success")
	if !strings.Contains(doctor, "\"auth_boundary\":\"provider_credentials\"") || !strings.Contains(doctor, "\"server_audit\":false") {
		t.Fatalf("cloud doctor missing rclone boundary:\n%s", doctor)
	}
}

func TestDirectCloudPushPullCLI(t *testing.T) {
	objectRoot := t.TempDir()
	deviceA := t.TempDir()
	deviceB := t.TempDir()
	runCLI(t, "init", deviceA, "--title", "Device A", "--json")
	runCLI(t, "init", deviceB, "--title", "Device B", "--json")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nfrom device A\n")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "personal", "--device", "laptop", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", deviceA, "--json")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "personal", "--device", "desktop", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", deviceB, "--json")
	push := runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	assertJSONCommandStatus(t, push, "sync.push", "success")
	pull := runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--json")
	assertJSONCommandStatus(t, pull, "sync.pull", "success")
	if !strings.Contains(pull, "\"remote_write\":false") || !strings.Contains(pull, "\"files_applied\":1") {
		t.Fatalf("direct pull output invalid:\n%s", pull)
	}
	got := readCLIFile(t, filepath.Join(deviceB, "notes", "alpha.md"))
	if !strings.Contains(got, "from device A") {
		t.Fatalf("pulled note missing remote body:\n%s", got)
	}
	writeCLIFixture(t, filepath.Join(deviceB, "notes", "alpha.md"), "# Alpha\n\nlocal desktop edit\n")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nupdated from device A\n")
	pushAgain := runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	assertJSONCommandStatus(t, pushAgain, "sync.push", "success")
	pullConflict := runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--json")
	assertJSONCommandStatus(t, pullConflict, "sync.pull", "success")
	updated := readCLIFile(t, filepath.Join(deviceB, "notes", "alpha.md"))
	if !strings.Contains(updated, "updated from device A") {
		t.Fatalf("pulled trunk missing update:\n%s", updated)
	}
	conflicts, err := filepath.Glob(filepath.Join(deviceB, "notes", "alpha.*.conflict.md"))
	if err != nil || len(conflicts) != 1 {
		t.Fatalf("expected one conflict copy, got %v err=%v", conflicts, err)
	}
	conflictBody := readCLIFile(t, conflicts[0])
	if !strings.Contains(conflictBody, "local desktop edit") {
		t.Fatalf("conflict copy lost local edit:\n%s", conflictBody)
	}
}

func TestSyncConflictsCommandsUseProjectionOutputModesAndReceiptsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	mainRel := filepath.ToSlash(filepath.Join("notes", "alpha.md"))
	conflictRel := filepath.ToSlash(filepath.Join("notes", "alpha.20260612010203.conflict.md"))
	writeCLIFixture(t, filepath.Join(root, filepath.FromSlash(mainRel)), "# Alpha\n\nremote trunk\n")
	writeCLIFixture(t, filepath.Join(root, filepath.FromSlash(conflictRel)), "# Alpha\n\nlocal edit\n")
	listDefault := runCLI(t, "sync", "conflicts", "list", "--vault", root)
	if strings.HasPrefix(strings.TrimSpace(listDefault), "{") || !strings.Contains(listDefault, conflictRel) || !strings.Contains(listDefault, "pinax sync conflicts list --vault "+root+" --json") {
		t.Fatalf("conflict list default output invalid:\n%s", listDefault)
	}

	listJSON := runCLI(t, "sync", "conflicts", "list", "--vault", root, "--json")
	assertJSONCommandStatus(t, listJSON, "sync.conflicts.list", "success")
	if !strings.Contains(listJSON, conflictRel) || !strings.Contains(listJSON, mainRel) || !strings.Contains(listJSON, "pinax sync conflicts diff "+conflictRel) || !strings.Contains(listJSON, "pinax sync conflicts resolve "+conflictRel) {
		t.Fatalf("conflict list json missing conflict paths/actions:\n%s", listJSON)
	}

	listAgent := runCLI(t, "sync", "conflicts", "list", "--vault", root, "--agent")
	for _, want := range []string{"mode=agent", "command=sync.conflicts.list", "fact.conflict.1.file=" + conflictRel, "action.diff=", "action.resolve="} {
		if !strings.Contains(listAgent, want) {
			t.Fatalf("conflict list agent missing %q:\n%s", want, listAgent)
		}
	}

	diffJSON := runCLI(t, "sync", "conflicts", "diff", conflictRel, "--vault", root, "--json")
	assertJSONCommandStatus(t, diffJSON, "sync.conflicts.diff", "success")
	if !strings.Contains(diffJSON, "--- "+mainRel) || !strings.Contains(diffJSON, "+++ "+conflictRel) || !strings.Contains(diffJSON, "pinax sync conflicts resolve "+conflictRel) {
		t.Fatalf("conflict diff json missing stable diff/actions:\n%s", diffJSON)
	}

	showEvents := runCLI(t, "sync", "conflicts", "show", conflictRel, "--vault", root, "--events")
	events := parseNDJSONEvents(t, showEvents)
	if !hasEventType(events, "start") || !hasEventType(events, "end") || !strings.Contains(showEvents, "sync.conflicts.show") || strings.Contains(showEvents, "local edit") {
		t.Fatalf("conflict show events should be structural and redacted: %#v\n%s", events, showEvents)
	}

	showExplain := runCLI(t, "sync", "conflicts", "show", conflictRel, "--vault", root, "--explain")
	if !strings.Contains(showExplain, "Recommended next step: pinax sync conflicts diff "+conflictRel) || strings.Contains(showExplain, "local edit") {
		t.Fatalf("conflict show explain missing next action or leaked body:\n%s", showExplain)
	}

	withoutYes, err := runCLIExpectError("sync", "conflicts", "resolve", conflictRel, "--keep-remote", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("conflict resolve without --yes succeeded: %s", withoutYes)
	}
	assertJSONErrorCode(t, withoutYes, "approval_required")
	if !fileExists(filepath.Join(root, filepath.FromSlash(conflictRel))) {
		t.Fatalf("conflict file was removed without --yes")
	}

	resolveJSON := runCLI(t, "sync", "conflicts", "resolve", conflictRel, "--keep-remote", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, resolveJSON, "sync.conflicts.resolve", "success")
	if !strings.Contains(resolveJSON, "\"receipt_path\"") || !strings.Contains(resolveJSON, "sync-conflicts/receipts/") || strings.Contains(resolveJSON, "local edit") || strings.Contains(resolveJSON, "remote trunk") {
		t.Fatalf("conflict resolve json missing safe receipt or leaked body:\n%s", resolveJSON)
	}
	if fileExists(filepath.Join(root, filepath.FromSlash(conflictRel))) {
		t.Fatalf("conflict file still exists after keep-remote resolve")
	}
	eventLog := readCLIFile(t, filepath.Join(root, ".pinax", "events.jsonl"))
	if !strings.Contains(eventLog, "sync.conflict.resolve") || strings.Contains(eventLog, "local edit") || strings.Contains(eventLog, "remote trunk") {
		t.Fatalf("resolve event missing or leaked body:\n%s", eventLog)
	}
}

func TestSyncConflictNextActionsAppearInSyncJSONAndAgentOutputsCLI(t *testing.T) {
	objectRoot := t.TempDir()
	deviceA := t.TempDir()
	deviceB := t.TempDir()
	runCLI(t, "init", deviceA, "--title", "Device A", "--json")
	runCLI(t, "init", deviceB, "--title", "Device B", "--json")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nfrom A\n")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "personal", "--device", "laptop", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", deviceA, "--json")
	runCLI(t, "cloud", "login", "--endpoint", "file://"+objectRoot, "--workspace", "personal", "--device", "desktop", "--secret-ref", "env://PINAX_TEST_SECRET", "--vault", deviceB, "--json")
	runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--json")

	writeCLIFixture(t, filepath.Join(deviceB, "notes", "alpha.md"), "# Alpha\n\nlocal JSON conflict\n")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nremote JSON conflict\n")
	runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	pullJSON := runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--json")
	assertJSONCommandStatus(t, pullJSON, "sync.pull", "success")
	for _, want := range []string{"\"conflicts\":\"1\"", "pinax sync conflicts list --vault " + deviceB + " --json", "pinax sync conflicts diff notes/alpha.", "pinax sync conflicts resolve notes/alpha."} {
		if !strings.Contains(pullJSON, want) {
			t.Fatalf("sync pull json missing conflict action %q:\n%s", want, pullJSON)
		}
	}

	writeCLIFixture(t, filepath.Join(deviceB, "notes", "alpha.md"), "# Alpha\n\nlocal agent conflict\n")
	writeCLIFixture(t, filepath.Join(deviceA, "notes", "alpha.md"), "# Alpha\n\nremote agent conflict\n")
	runCLI(t, "sync", "push", "--target", "cloud", "--yes", "--vault", deviceA, "--json")
	pullAgent := runCLI(t, "sync", "pull", "--target", "cloud", "--yes", "--vault", deviceB, "--agent")
	for _, want := range []string{"command=sync.pull", "fact.conflicts=1", "action.list=", "pinax sync conflicts list --vault " + deviceB + " --json", "action.diff=", "action.resolve="} {
		if !strings.Contains(pullAgent, want) {
			t.Fatalf("sync pull agent missing conflict action %q:\n%s", want, pullAgent)
		}
	}
}

func TestSyncConflictsCobraLayerDoesNotResolveFilesDirectlyCLI(t *testing.T) {
	source := readCLIFile(t, filepath.Join("..", "..", "internal", "cli", "sync_conflicts_cmd.go"))
	for _, forbidden := range []string{"os.Rename", "os.Remove", "fmt.Println", "fmt.Printf"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("sync conflicts Cobra layer still contains %s", forbidden)
		}
	}
}

func assertJSONErrorCode(t *testing.T, out, code string) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, out)
	}
	errorValue, ok := envelope["error"].(map[string]any)
	if !ok || errorValue["code"] != code {
		t.Fatalf("error code = %#v, want %s", envelope["error"], code)
	}
}

func assertJSONCommandStatus(t *testing.T, out, command, status string) {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, out)
	}
	if envelope["command"] != command || envelope["status"] != status {
		t.Fatalf("envelope command/status = %#v", envelope)
	}
}

func TestBackendLegacyStorageProjection(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	// Only set legacy storage.json, no backends.json yet.
	runCLI(t, "storage", "set-s3", "--bucket", "notes", "--region", "us-east-1", "--vault", root, "--json")
	// Remove any backends.json that might have been created.
	if err := os.Remove(filepath.Join(root, ".pinax", "backends.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove backends.json: %v", err)
	}
	listOut := runCLI(t, "backend", "list", "--vault", root, "--json")
	var listEnvelope map[string]any
	if err := json.Unmarshal([]byte(listOut), &listEnvelope); err != nil {
		t.Fatalf("backend list legacy json invalid: %v\n%s", err, listOut)
	}
	facts := listEnvelope["facts"].(map[string]any)
	if facts["backends"] != "1" {
		t.Fatalf("expected 1 backend from legacy storage: %s", listOut)
	}
}

func TestPlanningWorkflowsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	help := runCLI(t, "plan", "--help")
	for _, want := range []string{"daily", "weekly", "monthly", "actions", "snapshot"} {
		if !strings.Contains(help, want) {
			t.Fatalf("plan help missing %q:\n%s", want, help)
		}
	}

	// plan daily dry-run
	dailyDryRun := runCLI(t, "plan", "daily", "--dry-run", "--vault", root, "--json")
	var dailyEnvelope map[string]any
	if err := json.Unmarshal([]byte(dailyDryRun), &dailyEnvelope); err != nil {
		t.Fatalf("plan daily json invalid: %v\n%s", err, dailyDryRun)
	}
	if dailyEnvelope["command"] != "plan.daily" || dailyEnvelope["status"] != "success" {
		t.Fatalf("plan daily envelope = %#v", dailyEnvelope)
	}
	dailyFacts := dailyEnvelope["facts"].(map[string]any)
	if dailyFacts["period"] != "daily" || dailyFacts["dry_run"] != "true" || dailyFacts["max_commitments"] != "3" {
		t.Fatalf("plan daily facts = %#v", dailyFacts)
	}

	// plan daily with save
	dailyOut := runCLI(t, "plan", "daily", "--save", "--yes", "--vault", root, "--json")
	var dailySaveEnvelope map[string]any
	if err := json.Unmarshal([]byte(dailyOut), &dailySaveEnvelope); err != nil {
		t.Fatalf("plan daily save json invalid: %v\n%s", err, dailyOut)
	}
	if dailySaveEnvelope["command"] != "plan.daily" {
		t.Fatalf("plan daily save envelope = %#v", dailySaveEnvelope)
	}
	dailySaveFacts := dailySaveEnvelope["facts"].(map[string]any)
	if dailySaveFacts["dry_run"] != nil {
		t.Fatalf("plan daily save should not be dry_run: %#v", dailySaveFacts)
	}
	if dailySaveFacts["saved_path"] == "" || !strings.Contains(dailySaveFacts["saved_path"].(string), ".pinax/planning/snapshots/") {
		t.Fatalf("plan daily saved_path invalid: %#v", dailySaveFacts)
	}

	// plan weekly
	weeklyOut := runCLI(t, "plan", "weekly", "--dry-run", "--vault", root, "--json")
	var weeklyEnvelope map[string]any
	if err := json.Unmarshal([]byte(weeklyOut), &weeklyEnvelope); err != nil {
		t.Fatalf("plan weekly json invalid: %v\n%s", err, weeklyOut)
	}
	weeklyFacts := weeklyEnvelope["facts"].(map[string]any)
	if weeklyFacts["period"] != "weekly" || weeklyFacts["max_commitments"] != "7" {
		t.Fatalf("plan weekly facts = %#v", weeklyFacts)
	}

	// plan monthly
	monthlyOut := runCLI(t, "plan", "monthly", "--dry-run", "--vault", root, "--json")
	var monthlyEnvelope map[string]any
	if err := json.Unmarshal([]byte(monthlyOut), &monthlyEnvelope); err != nil {
		t.Fatalf("plan monthly json invalid: %v\n%s", err, monthlyOut)
	}
	monthlyFacts := monthlyEnvelope["facts"].(map[string]any)
	if monthlyFacts["period"] != "monthly" || monthlyFacts["max_commitments"] != "15" {
		t.Fatalf("plan monthly facts = %#v", monthlyFacts)
	}

	// plan actions dry-run
	actionsDryRun := runCLI(t, "plan", "actions", "--from", "daily", "--vault", root, "--json")
	var actionsEnvelope map[string]any
	if err := json.Unmarshal([]byte(actionsDryRun), &actionsEnvelope); err != nil {
		t.Fatalf("plan actions json invalid: %v\n%s", err, actionsDryRun)
	}
	if actionsEnvelope["command"] != "plan.actions" {
		t.Fatalf("plan actions envelope = %#v", actionsEnvelope)
	}

	// plan actions with save
	actionsSave := runCLI(t, "plan", "actions", "--from", "daily", "--save", "--vault", root, "--json")
	var actionsSaveEnvelope map[string]any
	if err := json.Unmarshal([]byte(actionsSave), &actionsSaveEnvelope); err != nil {
		t.Fatalf("plan actions save json invalid: %v\n%s", err, actionsSave)
	}
	actionsSaveFacts := actionsSaveEnvelope["facts"].(map[string]any)
	if actionsSaveFacts["saved_path"] == "" || !strings.Contains(actionsSaveFacts["saved_path"].(string), ".pinax/planning/actions/") {
		t.Fatalf("plan actions saved_path invalid: %#v", actionsSaveFacts)
	}

	// plan snapshot
	snapshotOut := runCLI(t, "plan", "snapshot", "--vault", root, "--json")
	var snapshotEnvelope map[string]any
	if err := json.Unmarshal([]byte(snapshotOut), &snapshotEnvelope); err != nil {
		t.Fatalf("plan snapshot json invalid: %v\n%s", err, snapshotOut)
	}
	if snapshotEnvelope["command"] != "plan.snapshot" {
		t.Fatalf("plan snapshot envelope = %#v", snapshotEnvelope)
	}
	snapshotFacts := snapshotEnvelope["facts"].(map[string]any)
	if snapshotFacts["snapshot_id"] == "" || snapshotFacts["saved_path"] == "" {
		t.Fatalf("plan snapshot facts = %#v", snapshotFacts)
	}

	// plan daily agent output
	agentOut := runCLI(t, "plan", "daily", "--dry-run", "--vault", root, "--agent")
	for _, want := range []string{"command=plan.daily", "status=success", "fact.period=daily", "fact.dry_run=true"} {
		if !strings.Contains(agentOut, want) {
			t.Fatalf("plan daily agent output missing %q:\n%s", want, agentOut)
		}
	}
}

func assertSameCommandAndFacts(t *testing.T, left, right, command string) {
	t.Helper()
	var leftEnvelope map[string]any
	if err := json.Unmarshal([]byte(left), &leftEnvelope); err != nil {
		t.Fatalf("left json invalid: %v\n%s", err, left)
	}
	var rightEnvelope map[string]any
	if err := json.Unmarshal([]byte(right), &rightEnvelope); err != nil {
		t.Fatalf("right json invalid: %v\n%s", err, right)
	}
	if leftEnvelope["command"] != command || rightEnvelope["command"] != command {
		t.Fatalf("commands = %#v %#v", leftEnvelope["command"], rightEnvelope["command"])
	}
	if fmt.Sprint(leftEnvelope["facts"]) != fmt.Sprint(rightEnvelope["facts"]) {
		t.Fatalf("facts differ:\nleft=%#v\nright=%#v", leftEnvelope["facts"], rightEnvelope["facts"])
	}
}

func runDashboardUntilCanceled(t *testing.T, root string) (string, string) {
	t.Helper()
	cmd := newRootCommand()
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"dashboard", "--vault", root, "--port", "0"})
	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetContext(ctx)
	time.AfterFunc(50*time.Millisecond, cancel)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dashboard command failed: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	return out.String(), errOut.String()
}

func runAPIServeUntilCanceled(t *testing.T, _ string, args ...string) (string, string, error) {
	t.Helper()
	cmd := newRootCommand()
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetContext(ctx)
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func runCLIWithInput(t *testing.T, input string, args ...string) string {
	t.Helper()
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("pinax %v failed: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeFakeEditor(t *testing.T, root, logPath string) string {
	t.Helper()
	path := filepath.Join(root, "fake-editor.sh")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + shellQuoteForTest(logPath) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake editor: %v", err)
	}
	return path
}

func shellQuoteForTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func runCLISeparate(args ...string) (string, string, error) {
	cmd := newRootCommand()
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func runCLI(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runCLIExpectError(args...)
	if err != nil {
		t.Fatalf("pinax %v failed: %v\n%s", args, err, out)
	}
	return out
}

func completionValueLines(out string) []string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "Completion ended") {
			continue
		}
		value, _, _ := strings.Cut(line, "\t")
		values = append(values, value)
	}
	return values
}

func runCLIInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	return runCLI(t, args...)
}

func runCLIExpectError(args ...string) (string, error) {
	cmd := newRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func readCLIFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(b)
}

func writeCLIFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func pinaxNoteFixture(id, title, tags, body string) string {
	if strings.TrimSpace(tags) == "" {
		tags = "[]"
	}
	return fmt.Sprintf("---\nschema_version: pinax.note.v1\nnote_id: %s\ntitle: %s\ntags: %s\n---\n\n# %s\n\n%s", id, title, tags, title, body)
}

func TestAssetVersionProviderRedactionContractsCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	payload := "pinax-binary raw-diff secret-token provider-payload"
	source := filepath.Join(root, "payload.bin")
	writeCLIFixture(t, source, payload)
	addOut := runCLI(t, "asset", "add", source, "--vault", root, "--json")
	showOut := runCLI(t, "asset", "show", "payload.bin", "--vault", root, "--json")
	for _, out := range []string{addOut, showOut} {
		for _, forbidden := range []string{"pinax-binary", "raw-diff", "secret-token", "provider-payload"} {
			if strings.Contains(out, forbidden) {
				t.Fatalf("asset output leaked %q:\n%s", forbidden, out)
			}
		}
	}

	diffOut, err := runCLIExpectError("version", "diff", "--base", "raw-diff-secret", "--target", "provider-payload", "--vault", root, "--json")
	if err == nil {
		t.Fatalf("version diff unexpectedly succeeded: %s", diffOut)
	}
	for _, forbidden := range []string{"raw-diff-secret", "provider-payload", "@@", "Authorization"} {
		if strings.Contains(diffOut, forbidden) {
			t.Fatalf("version diff output leaked %q:\n%s", forbidden, diffOut)
		}
	}

	deliverOut := runCLI(t, "briefing", "deliver", "feishu", "--webhook", "https://open.feishu.cn/open-apis/bot/v2/hook/raw-token", "--secret-ref", "env://FEISHU_WEBHOOK", "--title", "Daily briefing", "--text", "AI tooling update", "--dry-run", "--vault", root, "--json")
	for _, forbidden := range []string{"raw-token", "FEISHU_WEBHOOK", "Authorization", "Cookie"} {
		if strings.Contains(deliverOut, forbidden) {
			t.Fatalf("provider output leaked %q:\n%s", forbidden, deliverOut)
		}
	}
}

func TestTemplateListPackTemplateListUseCaseTemplateRecommendTemplateRecommendFallbackCLI(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	starter := runCLI(t, "template", "list", "--pack", "starter", "--vault", root, "--json")
	if !strings.Contains(starter, "note.quick") || !strings.Contains(starter, `"filter.pack":"starter"`) {
		t.Fatalf("template list starter output missing starter facts:\n%s", starter)
	}
	meetings := runCLI(t, "template", "list", "--use-case", "meeting", "--vault", root, "--json")
	if !strings.Contains(meetings, "meeting.notes") || !strings.Contains(meetings, `"filter.use_case":"meeting"`) {
		t.Fatalf("template list use-case output missing meeting facts:\n%s", meetings)
	}
	recommended := runCLI(t, "template", "recommend", "--intent", "meeting", "--vault", root, "--json")
	if !strings.Contains(recommended, `"command":"template.recommend"`) || !strings.Contains(recommended, `"primary":"meeting.notes"`) {
		t.Fatalf("template recommend meeting output = %s", recommended)
	}
	fallback := runCLI(t, "template", "recommend", "--intent", "unknown-intent", "--vault", root, "--json")
	if !strings.Contains(fallback, `"primary":"note.quick"`) && !strings.Contains(fallback, `"primary":"inbox.capture"`) {
		t.Fatalf("template recommend fallback output = %s", fallback)
	}
}

func TestTemplateCompletionJournalTemplateCompletionIndexTemplateCompletionNoteTemplateCompletion(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	inspect := runCLI(t, "__complete", "template", "inspect", "--vault", root, "")
	for _, want := range []string{"journal.daily\tbuiltin journal_template", "index.home\tbuiltin index_template", "ShellCompDirectiveNoFileComp"} {
		if !strings.Contains(inspect, want) {
			t.Fatalf("template inspect completion missing %q:\n%s", want, inspect)
		}
	}
	journal := runCLI(t, "__complete", "journal", "daily", "show", "--vault", root, "--template", "")
	if !strings.Contains(journal, "journal.daily\tbuiltin journal_template") || !strings.Contains(journal, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("journal template completion = %s", journal)
	}
	index := runCLI(t, "__complete", "index", "page", "preview", "home", "--vault", root, "--template", "")
	if !strings.Contains(index, "index.home\tbuiltin index_template") || !strings.Contains(index, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("index template completion = %s", index)
	}
	note := runCLI(t, "__complete", "note", "add", "Demo", "--vault", root, "--template", "")
	if !strings.Contains(note, "meeting.notes\tbuiltin note_template") || !strings.Contains(note, "note.quick\tbuiltin note_template") {
		t.Fatalf("note template completion = %s", note)
	}
	deleteOut := runCLI(t, "__complete", "template", "delete", "--vault", root, "")
	if strings.Contains(deleteOut, "journal.daily") || !strings.Contains(deleteOut, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("template delete should only complete local templates:\n%s", deleteOut)
	}
}

func TestTemplateFlagCompletionTemplateVarCompletionTemplateRunCompletion(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	engine := runCLI(t, "__complete", "template", "create", "demo", "--engine", "")
	if !strings.Contains(engine, "simple\tengine") || !strings.Contains(engine, "go-template\tengine") {
		t.Fatalf("template engine completion = %s", engine)
	}
	body := strings.Join([]string{"---", "schema_version: pinax.template.v2", "engine: go-template", "variables:", "  url:", "    required: true", "---", "# {{ .Vars.url }}"}, "\n")
	runCLI(t, "template", "create", "video-study", "--body", body, "--vault", root, "--json")
	vars := runCLI(t, "__complete", "template", "render", "video-study", "--vault", root, "--var", "")
	if !strings.Contains(vars, "url=\trequired string") || !strings.Contains(vars, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("template var completion = %s", vars)
	}
	runCLI(t, "template", "render", "video-study", "--var", "url=https://example.test", "--save-run", "video-run", "--vault", root, "--json")
	runs := runCLI(t, "__complete", "template", "render", "video-study", "--vault", root, "--run", "")
	if !strings.Contains(runs, "video-run") || !strings.Contains(runs, "render-run") || !strings.Contains(runs, "ShellCompDirectiveNoFileComp") {
		t.Fatalf("template run completion = %s", runs)
	}
}

func TestTokenCLICreateListRevoke(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	// List empty
	listOut := runCLI(t, "token", "list", "--vault", root)
	if !strings.Contains(listOut, "No tokens.") {
		t.Fatalf("expected empty token list, got: %s", listOut)
	}

	// Create token
	createOut := runCLI(t, "token", "create", "--label", "test-agent", "--scope", "read", "--vault", root)
	if !strings.Contains(createOut, "Token ID:") || !strings.Contains(createOut, "Secret:") {
		t.Fatalf("token create output missing ID or Secret: %s", createOut)
	}
	// Extract token ID
	lines := strings.Split(createOut, "\n")
	var tokenID string
	for _, line := range lines {
		if strings.HasPrefix(line, "Token ID:") {
			tokenID = strings.TrimSpace(strings.TrimPrefix(line, "Token ID:"))
		}
	}
	if tokenID == "" {
		t.Fatalf("failed to extract token ID from: %s", createOut)
	}

	// List with token
	listOut = runCLI(t, "token", "list", "--vault", root)
	if !strings.Contains(listOut, "test-agent") {
		t.Fatalf("token list should show test-agent: %s", listOut)
	}
	if !strings.Contains(listOut, tokenID) {
		t.Fatalf("token list should show ID %s: %s", tokenID, listOut)
	}

	// Revoke token
	revokeOut := runCLI(t, "token", "revoke", tokenID, "--vault", root)
	if !strings.Contains(revokeOut, "Revoked token:") {
		t.Fatalf("token revoke output: %s", revokeOut)
	}

	// List should be empty again
	listOut = runCLI(t, "token", "list", "--vault", root)
	if !strings.Contains(listOut, "No tokens.") {
		t.Fatalf("expected empty list after revoke, got: %s", listOut)
	}
}

func TestTokenCLICreateWithExpiry(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	createOut := runCLI(t, "token", "create", "--label", "temp", "--scope", "read,write", "--expires", "30d", "--vault", root)
	if !strings.Contains(createOut, "Secret:") {
		t.Fatalf("token create with expiry: %s", createOut)
	}
}

func TestTokenCLIMachineModesUseProjectionAndDoNotPrintSecret(t *testing.T) {
	for _, mode := range []string{"--json", "--agent"} {
		root := t.TempDir()
		runCLI(t, "init", root, "--title", "Vault", "--json")
		out := runCLI(t, "token", "create", "--label", "machine", "--scope", "read", "--vault", root, mode)
		if strings.Contains(out, "Secret:") || strings.Contains(out, "Save this secret") || strings.Contains(out, "请妥善保存") {
			t.Fatalf("token create %s printed human secret text: %s", mode, out)
		}
		if mode == "--json" {
			var envelope map[string]any
			if err := json.Unmarshal([]byte(out), &envelope); err != nil {
				t.Fatalf("token create --json did not emit JSON envelope: %v\n%s", err, out)
			}
			if envelope["command"] != "token.create" || envelope["status"] != "success" {
				t.Fatalf("token create json envelope = %#v", envelope)
			}
		} else if !strings.Contains(out, "command=token.create") || !strings.Contains(out, "status=success") {
			t.Fatalf("token create --agent output = %s", out)
		}
	}
}

func TestTokenCLIRotate(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	// Create token
	createOut := runCLI(t, "token", "create", "--label", "rotate-me", "--vault", root)
	lines := strings.Split(createOut, "\n")
	var oldID string
	for _, line := range lines {
		if strings.HasPrefix(line, "Token ID:") {
			oldID = strings.TrimSpace(strings.TrimPrefix(line, "Token ID:"))
		}
	}

	// Rotate token
	rotateOut := runCLI(t, "token", "rotate", oldID, "--vault", root)
	if !strings.Contains(rotateOut, "New token ID:") || !strings.Contains(rotateOut, "Secret:") {
		t.Fatalf("token rotate output: %s", rotateOut)
	}
	if !strings.Contains(rotateOut, "Rotated from:") {
		t.Fatalf("token rotate missing rotated-from: %s", rotateOut)
	}

	// Old token should be gone
	listOut := runCLI(t, "token", "list", "--vault", root)
	if strings.Contains(listOut, oldID) {
		t.Fatalf("old token should be revoked after rotate: %s", listOut)
	}
}

func TestProfileCLIAddListRemove(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	runCLI(t, "init", root, "--title", "Vault", "--json")

	// List empty
	listOut := runCLI(t, "profile", "list", "--vault", root)
	if !strings.Contains(listOut, "No profiles.") {
		t.Fatalf("expected empty profile list, got: %s", listOut)
	}

	// Add profile
	addOut := runCLI(t, "profile", "add", "my-s3", "--endpoint", "s3://bucket/path", "--workspace", "default", "--vault", root)
	if !strings.Contains(addOut, "Added profile:") {
		t.Fatalf("profile add output: %s", addOut)
	}

	// List with profile
	listOut = runCLI(t, "profile", "list", "--vault", root)
	if !strings.Contains(listOut, "my-s3") {
		t.Fatalf("profile list should show my-s3: %s", listOut)
	}

	// Show profile
	showOut := runCLI(t, "profile", "show", "my-s3", "--vault", root)
	if !strings.Contains(showOut, "my-s3") || !strings.Contains(showOut, "s3://bucket/path") {
		t.Fatalf("profile show output: %s", showOut)
	}

	// Remove profile
	removeOut := runCLI(t, "profile", "remove", "my-s3", "--vault", root)
	if !strings.Contains(removeOut, "Deleted profile:") {
		t.Fatalf("profile remove output: %s", removeOut)
	}

	// List should be empty again
	listOut = runCLI(t, "profile", "list", "--vault", root)
	if !strings.Contains(listOut, "No profiles.") {
		t.Fatalf("expected empty list after remove, got: %s", listOut)
	}
}

func TestProfileCLIAddRequiresEndpoint(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	_, err := runCLIExpectError("profile", "add", "bad", "--vault", root)
	if err == nil {
		t.Fatal("expected error when adding profile without --endpoint")
	}
}

func TestBackendObjectListAndStatCommandsReadLocalBlobStore(t *testing.T) {
	root := t.TempDir()
	backendRoot := filepath.Join(root, "backend-store")
	runCLI(t, "init", root, "--title", "Vault", "--json")
	runCLI(t, "backend", "add", "local", "work-local", "--root", backendRoot, "--vault", root, "--json")
	if err := os.MkdirAll(filepath.Join(backendRoot, "pinax"), 0o700); err != nil {
		t.Fatalf("mkdir backend fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backendRoot, "pinax", "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write backend fixture: %v", err)
	}

	lsOut := runCLI(t, "backend", "object", "list", "work-local", "pinax", "--vault", root, "--json")
	var lsEnvelope map[string]any
	if err := json.Unmarshal([]byte(lsOut), &lsEnvelope); err != nil {
		t.Fatalf("backend ls json invalid: %v\n%s", err, lsOut)
	}
	if lsEnvelope["command"] != "backend.object.list" || !strings.Contains(lsOut, "manifest.json") {
		t.Fatalf("backend object list did not include local object: %#v\n%s", lsEnvelope, lsOut)
	}

	statOut := runCLI(t, "backend", "object", "stat", "work-local", "pinax/manifest.json", "--vault", root, "--json")
	var statEnvelope map[string]any
	if err := json.Unmarshal([]byte(statOut), &statEnvelope); err != nil {
		t.Fatalf("backend stat json invalid: %v\n%s", err, statOut)
	}
	if statEnvelope["command"] != "backend.object.stat" || !strings.Contains(statOut, "revision") {
		t.Fatalf("backend object stat did not include revision: %#v\n%s", statEnvelope, statOut)
	}
}
