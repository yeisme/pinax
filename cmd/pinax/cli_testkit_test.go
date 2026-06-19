package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

func jsonParseFacts(t *testing.T, out string) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("json invalid: %v\n%s", err, out)
	}
	return envelope["facts"].(map[string]any)
}

// TestProofLoopRunPreviewEmitsRunIDAndStageFacts 证明 proof loop run preview 模式产出
// 单一 projection 携带 proof_loop_run_id、有序 stage facts、saved plan paths 与 snapshot
// next action，且不写 vault（没有 apply）。

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

func assertNoForbiddenSyncLeak(t *testing.T, got string, forbidden []string) {
	t.Helper()
	for _, value := range forbidden {
		if strings.Contains(got, value) {
			t.Fatalf("sync output leaked %q:\n%s", value, got)
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
	leftFacts := normalizedAliasFacts(leftEnvelope["facts"])
	rightFacts := normalizedAliasFacts(rightEnvelope["facts"])
	if fmt.Sprint(leftFacts) != fmt.Sprint(rightFacts) {
		t.Fatalf("facts differ:\nleft=%#v\nright=%#v", leftFacts, rightFacts)
	}
}

func normalizedAliasFacts(raw any) map[string]any {
	facts, ok := raw.(map[string]any)
	if !ok {
		return map[string]any{"value": raw}
	}
	normalized := make(map[string]any, len(facts))
	for key, value := range facts {
		if key == "scan_duration_ms" {
			continue
		}
		normalized[key] = value
	}
	return normalized
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
