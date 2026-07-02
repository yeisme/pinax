package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	for key, want := range map[string]string{"version_backend": "local", "snapshot_supported": "true", "changed_paths_supported": "false", "read_at_revision_supported": "true"} {
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

// TestVersionRestoreApplyRevertsBadLocalApply 证明坏本地 apply 可通过 CLI 安全回滚：
// 基线 git commit → 坏改动 → 生成 restore plan → apply 恢复到基线内容，
// 全程 local_write=true / remote_write=false，且不带 --yes 时拒绝写入。

func TestVersionRestoreApplyRevertsBadLocalApply(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	notePath := filepath.Join(root, "notes", "alpha.md")
	writeCLIFixture(t, notePath, "# Alpha\n\noriginal baseline content\n")
	snapshotOut := runCLI(t, "version", "snapshot", "--vault", root, "--message", "baseline before apply", "--json")
	snapshotID := jsonParseFacts(t, snapshotOut)["snapshot_id"].(string)
	// 坏本地 apply：覆盖成损坏内容。
	writeCLIFixture(t, notePath, "# Alpha\n\ncorrupted by bad apply\n")

	planOut := runCLI(t, "version", "restore", "notes/alpha.md", "--revision", snapshotID, "--plan", "--vault", root, "--json")
	var planEnvelope map[string]any
	if err := json.Unmarshal([]byte(planOut), &planEnvelope); err != nil {
		t.Fatalf("restore plan json invalid: %v\n%s", err, planOut)
	}
	planFacts := planEnvelope["facts"].(map[string]any)
	planID, ok := planFacts["plan_id"].(string)
	if !ok || planID == "" {
		t.Fatalf("restore plan missing plan_id: %#v", planFacts)
	}
	if planFacts["version_backend"] != "local" || planFacts["content_hash"] == "" {
		t.Fatalf("restore plan missing local content evidence: %#v", planFacts)
	}

	// 不带 --yes 必须拒绝写入并给出审批提示。
	refusedOut, refusedErr := runCLIExpectError("version", "restore", "apply", "--vault", root, "--plan", planID, "--json")
	if refusedErr == nil {
		t.Fatalf("restore apply without --yes succeeded:\n%s", refusedOut)
	}
	if !strings.Contains(refusedOut, "approval_required") || !strings.Contains(refusedOut, "--yes") {
		t.Fatalf("restore apply refusal missing approval guidance:\n%s", refusedOut)
	}

	// 带 --yes 恢复到基线内容。
	applyOut := runCLI(t, "version", "restore", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	var applyEnvelope map[string]any
	if err := json.Unmarshal([]byte(applyOut), &applyEnvelope); err != nil {
		t.Fatalf("restore apply json invalid: %v\n%s", err, applyOut)
	}
	applyFacts := applyEnvelope["facts"].(map[string]any)
	if applyEnvelope["command"] != "version.restore.apply" || applyEnvelope["status"] != "success" {
		t.Fatalf("restore apply envelope = %#v\n%s", applyEnvelope, applyOut)
	}
	if applyFacts["local_write"] != "true" || applyFacts["remote_write"] != "false" {
		t.Fatalf("restore apply must emit local_write=true remote_write=false: %#v", applyFacts)
	}
	restored := readCLIFile(t, notePath)
	if !strings.Contains(restored, "original baseline content") || strings.Contains(restored, "corrupted by bad apply") {
		t.Fatalf("restore apply did not revert to baseline content:\n%s", restored)
	}
	// receipt evidence 写入。
	receipt, _ := applyEnvelope["data"].(map[string]any)["receipt"].(string)
	if receipt == "" || !strings.Contains(receipt, ".pinax/receipts/") {
		t.Fatalf("restore apply missing receipt path: %#v", applyEnvelope["data"])
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(receipt))); err != nil {
		t.Fatalf("restore receipt file missing: %v", err)
	}
}

func TestVersionRestoreApplyUsesLocalSnapshotWithoutGitCommit(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	noteRel := "notes/local-only.md"
	notePath := filepath.Join(root, filepath.FromSlash(noteRel))
	writeCLIFixture(t, notePath, "# Local Only\n\noriginal pinax snapshot content\n")

	snapshotOut := runCLI(t, "version", "snapshot", "--vault", root, "--message", "pinax managed baseline", "--json")
	snapshotID := jsonParseFacts(t, snapshotOut)["snapshot_id"].(string)
	writeCLIFixture(t, notePath, "# Local Only\n\ncorrupted without git commit\n")

	planOut := runCLI(t, "version", "restore", noteRel, "--revision", snapshotID, "--plan", "--vault", root, "--json")
	planFacts := jsonParseFacts(t, planOut)
	if planFacts["version_backend"] != "local" {
		t.Fatalf("restore plan should use local backend: %#v", planFacts)
	}
	if _, hasGitCommit := planFacts["git_commit"]; hasGitCommit {
		t.Fatalf("local-only restore plan should not require git commit: %#v", planFacts)
	}
	planID := planFacts["plan_id"].(string)

	applyOut := runCLI(t, "version", "restore", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	applyFacts := jsonParseFacts(t, applyOut)
	if applyFacts["version_backend"] != "local" || applyFacts["local_write"] != "true" || applyFacts["remote_write"] != "false" {
		t.Fatalf("restore apply facts = %#v", applyFacts)
	}
	restored := readCLIFile(t, notePath)
	if !strings.Contains(restored, "original pinax snapshot content") || strings.Contains(restored, "corrupted without git commit") {
		t.Fatalf("restore apply did not use Pinax snapshot content:\n%s", restored)
	}
}

// TestVersionRestoreApplyRefusesStalePlan 证明 vault 在 plan 生成后被改动时 apply 拒绝执行。

func TestVersionRestoreApplyRefusesStalePlan(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	notePath := filepath.Join(root, "notes", "beta.md")
	writeCLIFixture(t, notePath, "# Beta\n\nfirst\n")
	snapshotOut := runCLI(t, "version", "snapshot", "--vault", root, "--message", "baseline", "--json")
	snapshotID := jsonParseFacts(t, snapshotOut)["snapshot_id"].(string)
	writeCLIFixture(t, notePath, "# Beta\n\nsecond\n")
	planOut := runCLI(t, "version", "restore", "notes/beta.md", "--revision", snapshotID, "--plan", "--vault", root, "--json")
	planID := jsonParseFacts(t, planOut)["plan_id"].(string)
	// plan 生成后再改动 vault，vault hash 漂移。
	writeCLIFixture(t, notePath, "# Beta\n\nthird drift\n")
	staleOut, staleErr := runCLIExpectError("version", "restore", "apply", "--vault", root, "--plan", planID, "--yes", "--json")
	if staleErr == nil {
		t.Fatalf("stale restore apply succeeded:\n%s", staleOut)
	}
	if !strings.Contains(staleOut, "restore_plan_stale") {
		t.Fatalf("stale restore apply missing restore_plan_stale:\n%s", staleOut)
	}
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
