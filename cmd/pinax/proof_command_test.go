package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestProofLoopRunPreviewEmitsRunIDAndStageFacts(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), pinaxNoteFixture("note_alpha", "Alpha", "[]", "alpha body\n"))
	runCLI(t, "index", "sync", "--vault", root, "--json")

	out := runCLI(t, "proof", "loop", "run", "--vault", root, "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("proof loop run json invalid: %v\n%s", err, out)
	}
	if envelope["command"] != "proof.loop.run" || envelope["status"] != "success" {
		t.Fatalf("proof loop run envelope = %#v\n%s", envelope, out)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["proof_loop_run_id"] == nil || facts["proof_loop_run_id"] == "" {
		t.Fatalf("proof loop run missing proof_loop_run_id: %#v", facts)
	}
	if facts["mode"] != "preview" {
		t.Fatalf("proof loop run mode = %v, want preview", facts["mode"])
	}
	for _, stage := range []string{"capture", "diagnose", "plan"} {
		found := false
		for k := range facts {
			if strings.HasPrefix(k, stage+".") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("proof loop run missing stage facts for %q: %#v", stage, facts)
		}
	}
	if facts["plan.repair_plan_id"] == nil || facts["plan.repair_plan_id"] == "" {
		t.Fatalf("proof loop run missing saved repair plan id: %#v", facts)
	}
	data := envelope["data"].(map[string]any)
	stages, _ := data["stages"].([]any)
	if len(stages) == 0 {
		t.Fatalf("proof loop run missing ordered stages in data: %#v", data)
	}
	// preview 必须有 snapshot next action（提示 apply 前先快照）。
	actions := envelope["actions"].([]any)
	hasSnapshotAction := false
	for _, action := range actions {
		cmd, _ := action.(map[string]any)["command"].(string)
		if strings.Contains(cmd, "version snapshot") {
			hasSnapshotAction = true
		}
	}
	if !hasSnapshotAction {
		t.Fatalf("proof loop run preview missing snapshot next action: %#v", actions)
	}
}

// TestProofLoopRunApplyRequiresYes 证明 --apply 不带 --yes 时拒绝写入。

func TestProofLoopRunApplyRequiresYes(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	out, err := runCLIExpectError("proof", "loop", "run", "--vault", root, "--apply", "--json")
	if err == nil {
		t.Fatalf("proof loop run --apply without --yes succeeded:\n%s", out)
	}
	if !strings.Contains(out, "approval_required") || !strings.Contains(out, "--yes") {
		t.Fatalf("proof loop run --apply missing approval guidance:\n%s", out)
	}
}

// TestProofLoopRunApplyExecutesAfterFreshSnapshot 证明 --apply --yes 先 fresh snapshot 再 apply。

func TestProofLoopRunApplyExecutesAfterFreshSnapshot(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, filepath.Join(root, "No Tags.md"), "# No Tags\n\nbody without tags\n")
	runCLI(t, "index", "sync", "--vault", root, "--json")

	out := runCLI(t, "proof", "loop", "run", "--vault", root, "--apply", "--yes", "--json")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("proof loop run apply json invalid: %v\n%s", err, out)
	}
	if envelope["status"] != "success" {
		t.Fatalf("proof loop run apply status = %v\n%s", envelope["status"], out)
	}
	facts := envelope["facts"].(map[string]any)
	if facts["mode"] != "apply" {
		t.Fatalf("proof loop run apply mode = %v, want apply", facts["mode"])
	}
	if facts["apply.snapshot"] != "true" {
		t.Fatalf("proof loop run apply missing fresh snapshot fact: %#v", facts)
	}
}
