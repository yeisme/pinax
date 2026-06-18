package e2e

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestDemo(t *testing.T) {
	runDemoTestScript(t,
		"testdata/demo/scripts/demo_diagnose.txt",
		"testdata/demo/scripts/demo_plan_snapshot_apply.txt",
		"testdata/demo/scripts/demo_restore.txt",
	)
}

func TestDemoPlanSnapshotApply(t *testing.T) {
	runDemoTestScript(t, "testdata/demo/scripts/demo_plan_snapshot_apply.txt")
}

func TestDemoRestore(t *testing.T) {
	runDemoTestScript(t, "testdata/demo/scripts/demo_restore.txt")
}

func runDemoTestScript(t *testing.T, files ...string) {
	t.Helper()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	testscript.Run(t, testscript.Params{
		Files: files,
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"validate-json-envelope":        cmdValidateJSONEnvelope,
			"validate-agent-format":         cmdValidateAgentFormat,
			"validate-clean-stdout":         cmdValidateCleanStdout,
			"prepare-demo-vault":            makeCmdPrepareDemoVault(repoRoot),
			"capture-demo-baseline":         cmdCaptureDemoBaseline,
			"seed-demo-git-baseline":        cmdSeedDemoGitBaseline,
			"assert-demo-doctor":            cmdAssertDemoDoctor,
			"assert-demo-plan":              cmdAssertDemoPlan,
			"assert-demo-apply":             cmdAssertDemoApply,
			"assert-demo-manual-unchanged":  cmdAssertDemoManualUnchanged,
			"assert-demo-metadata-patched":  cmdAssertDemoMetadataPatched,
			"assert-demo-post-apply-doctor": cmdAssertDemoPostApplyDoctor,
			"assert-demo-restore-plan":      cmdAssertDemoRestorePlan,
			"assert-demo-restore-apply":     cmdAssertDemoRestoreApply,
			"assert-file-equals":            cmdAssertFileEquals,
		},
		Setup: func(env *testscript.Env) error {
			env.Vars = append(env.Vars,
				"PATH="+sharedBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"PINAX_REPO_ROOT="+repoRoot,
				"NO_COLOR=1",
			)
			return nil
		},
	})
}

func makeCmdPrepareDemoVault(repoRoot string) func(ts *testscript.TestScript, neg bool, args []string) {
	return func(ts *testscript.TestScript, neg bool, args []string) {
		noNeg(ts, neg, "prepare-demo-vault")
		if len(args) != 1 {
			ts.Fatalf("usage: prepare-demo-vault <destination>")
		}
		src := filepath.Join(repoRoot, "examples", "messy-vault")
		dst := ts.MkAbs(args[0])
		ts.Check(os.RemoveAll(dst))
		ts.Check(copyTree(src, dst))
		stalePath := filepath.Join(dst, "notes", "archive", "old-spec.md")
		staleTime := time.Now().Add(-120 * 24 * time.Hour)
		ts.Check(os.Chtimes(stalePath, staleTime, staleTime))
	}
}

func cmdCaptureDemoBaseline(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "capture-demo-baseline")
	if len(args) != 2 {
		ts.Fatalf("usage: capture-demo-baseline <vault> <baseline-dir>")
	}
	vault := ts.MkAbs(args[0])
	baseline := ts.MkAbs(args[1])
	ts.Check(os.RemoveAll(baseline))
	for _, rel := range demoTrackedNotePaths() {
		src := filepath.Join(vault, filepath.FromSlash(rel))
		dst := filepath.Join(baseline, filepath.FromSlash(rel))
		ts.Check(os.MkdirAll(filepath.Dir(dst), 0o755))
		data, err := os.ReadFile(src)
		ts.Check(err)
		ts.Check(os.WriteFile(dst, data, 0o644))
	}
}

func cmdSeedDemoGitBaseline(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "seed-demo-git-baseline")
	if len(args) != 1 {
		ts.Fatalf("usage: seed-demo-git-baseline <vault>")
	}
	vault := ts.MkAbs(args[0])
	for _, cmd := range [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "pinax-demo@example.invalid"},
		{"git", "config", "user.name", "Pinax Demo"},
		{"git", "add", "."},
		{"git", "commit", "-m", "dogfood demo baseline"},
	} {
		runDemoSetupCommand(ts, vault, cmd[0], cmd[1:]...)
	}
}

func cmdAssertDemoDoctor(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "assert-demo-doctor")
	if len(args) != 1 {
		ts.Fatalf("usage: assert-demo-doctor <doctor-json>")
	}
	envelope := readJSONEnvelope(ts, args[0])
	if envelope["command"] != "vault.doctor" || envelope["status"] != "partial" {
		ts.Fatalf("doctor envelope command/status mismatch: %#v", envelope)
	}
	issues := issueCodes(ts, envelope)
	for _, code := range []string{"broken_link", "orphan_note", "missing_tags", "duplicate_title", "empty_note", "stale_note"} {
		if issues[code] == 0 {
			ts.Fatalf("doctor output missing issue code %q: %#v", code, issues)
		}
	}
	assertIssuePath(ts, envelope, "broken_link", "notes/research/auth-design.md")
	assertIssuePath(ts, envelope, "orphan_note", "notes/research/api-notes.md")
	assertIssuePath(ts, envelope, "missing_tags", "notes/research/meeting-2026.md")
	assertIssuePath(ts, envelope, "duplicate_title", "notes/projects/pinax-plan.md")
	assertIssuePath(ts, envelope, "empty_note", "notes/inbox/random-thought.md")
	assertIssuePath(ts, envelope, "stale_note", "notes/archive/old-spec.md")
}

func cmdAssertDemoPlan(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "assert-demo-plan")
	if len(args) != 1 {
		ts.Fatalf("usage: assert-demo-plan <repair-plan-json>")
	}
	envelope := readJSONEnvelope(ts, args[0])
	if envelope["command"] != "repair.plan" || envelope["status"] != "partial" {
		ts.Fatalf("repair plan envelope command/status mismatch: %#v", envelope)
	}
	operations := envelopeOperations(ts, envelope)
	assertOperation(ts, operations, "missing_tags", "tags_patch", "automatic", "notes/research/meeting-2026.md")
	assertOperation(ts, operations, "broken_link", "link_resolution", "manual_review", "notes/research/auth-design.md")
	assertOperation(ts, operations, "orphan_note", "orphan_review", "manual_review", "notes/research/api-notes.md")
	assertOperation(ts, operations, "duplicate_title", "manual_review", "manual_review", "notes/projects/pinax-plan.md")
	assertOperation(ts, operations, "empty_note", "manual_review", "manual_review", "notes/inbox/random-thought.md")
	assertOperation(ts, operations, "stale_note", "archive_status_patch", "automatic", "notes/archive/old-spec.md")
}

func cmdAssertDemoApply(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "assert-demo-apply")
	if len(args) != 1 {
		ts.Fatalf("usage: assert-demo-apply <repair-apply-json>")
	}
	envelope := readJSONEnvelope(ts, args[0])
	if envelope["command"] != "repair.apply" || envelope["status"] != "success" {
		ts.Fatalf("repair apply envelope command/status mismatch: %#v", envelope)
	}
	facts := stringMap(envelope["facts"])
	if facts["applied"] == "" || facts["plan_id"] == "" {
		ts.Fatalf("repair apply missing applied count or plan id: %#v", facts)
	}
}

func cmdAssertDemoManualUnchanged(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "assert-demo-manual-unchanged")
	if len(args) != 2 {
		ts.Fatalf("usage: assert-demo-manual-unchanged <baseline-dir> <vault>")
	}
	baseline := ts.MkAbs(args[0])
	vault := ts.MkAbs(args[1])
	for _, rel := range []string{
		"notes/research/auth-design.md",
		"notes/research/api-notes.md",
		"notes/projects/pinax-plan.md",
		"notes/projects/pinax-plan-2.md",
		"notes/inbox/random-thought.md",
	} {
		assertSameFile(ts, filepath.Join(baseline, filepath.FromSlash(rel)), filepath.Join(vault, filepath.FromSlash(rel)))
	}
}

func cmdAssertDemoMetadataPatched(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "assert-demo-metadata-patched")
	if len(args) != 2 {
		ts.Fatalf("usage: assert-demo-metadata-patched <before-file> <after-file>")
	}
	before := []byte(ts.ReadFile(args[0]))
	after := []byte(ts.ReadFile(args[1]))
	if string(before) == string(after) {
		ts.Fatalf("metadata file did not change after repair apply: %s", args[1])
	}
	afterText := string(after)
	for _, want := range []string{"schema_version: pinax.note.v1", "note_id: note_demo_meeting_2026", "title: Meeting 2026", "tags: []"} {
		if !strings.Contains(afterText, want) {
			ts.Fatalf("metadata patch missing %q in %s:\n%s", want, args[1], afterText)
		}
	}
}

func cmdAssertDemoPostApplyDoctor(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "assert-demo-post-apply-doctor")
	if len(args) != 1 {
		ts.Fatalf("usage: assert-demo-post-apply-doctor <doctor-json>")
	}
	envelope := readJSONEnvelope(ts, args[0])
	issues := issueCodes(ts, envelope)
	for _, code := range []string{"broken_link", "orphan_note", "duplicate_title", "empty_note"} {
		if issues[code] == 0 {
			ts.Fatalf("manual-review issue %q disappeared after low-risk apply: %#v", code, issues)
		}
	}
}

func cmdAssertDemoRestorePlan(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "assert-demo-restore-plan")
	if len(args) != 1 {
		ts.Fatalf("usage: assert-demo-restore-plan <restore-plan-json>")
	}
	envelope := readJSONEnvelope(ts, args[0])
	if envelope["command"] != "version.restore" || envelope["status"] != "success" {
		ts.Fatalf("restore plan envelope command/status mismatch: %#v", envelope)
	}
	facts := stringMap(envelope["facts"])
	for _, key := range []string{"plan_id", "git_commit", "path", "revision"} {
		if facts[key] == "" {
			ts.Fatalf("restore plan missing fact %q: %#v", key, facts)
		}
	}
	if facts["path"] != "notes/research/meeting-2026.md" {
		ts.Fatalf("restore plan path = %q", facts["path"])
	}
}

func cmdAssertDemoRestoreApply(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "assert-demo-restore-apply")
	if len(args) != 1 {
		ts.Fatalf("usage: assert-demo-restore-apply <restore-apply-json>")
	}
	envelope := readJSONEnvelope(ts, args[0])
	if envelope["command"] != "version.restore.apply" || envelope["status"] != "success" {
		ts.Fatalf("restore apply envelope command/status mismatch: %#v", envelope)
	}
	facts := stringMap(envelope["facts"])
	if facts["local_write"] != "true" || facts["remote_write"] != "false" {
		ts.Fatalf("restore apply write facts mismatch: %#v", facts)
	}
}

func cmdAssertFileEquals(ts *testscript.TestScript, neg bool, args []string) {
	noNeg(ts, neg, "assert-file-equals")
	if len(args) != 2 {
		ts.Fatalf("usage: assert-file-equals <want> <got>")
	}
	assertSameFile(ts, ts.MkAbs(args[0]), ts.MkAbs(args[1]))
}

func noNeg(ts *testscript.TestScript, neg bool, name string) {
	if neg {
		ts.Fatalf("%s does not support negation", name)
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func runDemoSetupCommand(ts *testscript.TestScript, dir, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		ts.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, string(out))
	}
}

func demoTrackedNotePaths() []string {
	return []string{
		"notes/research/auth-design.md",
		"notes/research/api-notes.md",
		"notes/research/meeting-2026.md",
		"notes/projects/pinax-plan.md",
		"notes/projects/pinax-plan-2.md",
		"notes/inbox/random-thought.md",
		"notes/archive/old-spec.md",
	}
}

func readJSONEnvelope(ts *testscript.TestScript, path string) map[string]any {
	content := ts.ReadFile(path)
	var envelope map[string]any
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		ts.Fatalf("invalid JSON envelope %s: %v\n%s", path, err, content)
	}
	return envelope
}

func issueCodes(ts *testscript.TestScript, envelope map[string]any) map[string]int {
	issues := envelopeIssues(ts, envelope)
	codes := make(map[string]int, len(issues))
	for _, issue := range issues {
		code, _ := issue["issue_code"].(string)
		if code != "" {
			codes[code]++
		}
	}
	return codes
}

func envelopeIssues(ts *testscript.TestScript, envelope map[string]any) []map[string]any {
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		ts.Fatalf("envelope missing data object: %#v", envelope)
	}
	rawIssues, ok := data["issues"].([]any)
	if !ok {
		ts.Fatalf("envelope missing data.issues array: %#v", data)
	}
	issues := make([]map[string]any, 0, len(rawIssues))
	for _, raw := range rawIssues {
		issue, ok := raw.(map[string]any)
		if !ok {
			ts.Fatalf("issue is not an object: %#v", raw)
		}
		issues = append(issues, issue)
	}
	return issues
}

func envelopeOperations(ts *testscript.TestScript, envelope map[string]any) []map[string]any {
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		ts.Fatalf("envelope missing data object: %#v", envelope)
	}
	rawOperations, ok := data["operations"].([]any)
	if !ok {
		ts.Fatalf("envelope missing data.operations array: %#v", data)
	}
	operations := make([]map[string]any, 0, len(rawOperations))
	for _, raw := range rawOperations {
		operation, ok := raw.(map[string]any)
		if !ok {
			ts.Fatalf("operation is not an object: %#v", raw)
		}
		operations = append(operations, operation)
	}
	return operations
}

func assertIssuePath(ts *testscript.TestScript, envelope map[string]any, code, path string) {
	for _, issue := range envelopeIssues(ts, envelope) {
		if issue["issue_code"] == code && issue["path"] == path {
			return
		}
	}
	ts.Fatalf("issue %s for %s not found", code, path)
}

func assertOperation(ts *testscript.TestScript, operations []map[string]any, issueCode, kind, mode, path string) {
	for _, operation := range operations {
		if operation["issue_code"] == issueCode && operation["kind"] == kind && operation["mode"] == mode && operation["path"] == path {
			return
		}
	}
	ts.Fatalf("operation issue=%s kind=%s mode=%s path=%s not found in %#v", issueCode, kind, mode, path, operations)
}

func assertSameFile(ts *testscript.TestScript, wantPath, gotPath string) {
	want, err := os.ReadFile(wantPath)
	ts.Check(err)
	got, err := os.ReadFile(gotPath)
	ts.Check(err)
	if string(want) != string(got) {
		ts.Fatalf("file mismatch %s != %s\n--- want ---\n%s\n--- got ---\n%s", wantPath, gotPath, string(want), string(got))
	}
}

func stringMap(raw any) map[string]string {
	result := map[string]string{}
	values, ok := raw.(map[string]any)
	if !ok {
		return result
	}
	for key, value := range values {
		result[key] = fmt.Sprint(value)
	}
	return result
}
