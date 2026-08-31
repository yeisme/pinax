package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunWritesCompleteRedactedFailureEvidenceWithOriginalExitCode(t *testing.T) {
	parent := t.TempDir()
	result, err := Run(Config{
		RunID:      "failure-evidence",
		ParentDir:  parent,
		Command:    []string{"sh", "-c", "printf 'Authorization: Bearer secret-token /tmp/private RAW_PROMPT_SENTINEL\n' >&2; exit 7"},
		Layer:      "component",
		PassStatus: "passed",
	})
	if err != nil {
		t.Fatalf("evidence run: %v", err)
	}
	if result.ExitCode != 7 || result.Summary.Status != "failed" || result.Summary.Layer != "component" {
		t.Fatalf("result = %#v", result)
	}
	runDir := filepath.Join(parent, "failure-evidence")
	for _, rel := range []string{"summary.json", "command.txt", "stdout.log", "stderr.log", "env.json", "artifacts/README.txt"} {
		if _, statErr := os.Stat(filepath.Join(runDir, rel)); statErr != nil {
			t.Fatalf("missing evidence file %s: %v", rel, statErr)
		}
	}
	stderr, err := os.ReadFile(filepath.Join(runDir, "stderr.log"))
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	for _, forbidden := range []string{"secret-token", "Authorization: Bearer", "/tmp/private", "RAW_PROMPT_SENTINEL"} {
		if string(stderr) == "" || contains(string(stderr), forbidden) {
			t.Fatalf("stderr contains forbidden %q: %s", forbidden, stderr)
		}
	}
	if result.Summary.Checks["redaction_scan_passed"] != true || !result.Summary.Redaction.ScanPassed {
		t.Fatalf("redaction scan = %#v", result.Summary)
	}
}

func TestRunUsesPassedStatusForSuccessfulComponentProfile(t *testing.T) {
	parent := t.TempDir()
	result, err := Run(Config{RunID: "passed-evidence", ParentDir: parent, Command: []string{"sh", "-c", "exit 0"}, PassStatus: "passed", Layer: "component"})
	if err != nil {
		t.Fatalf("evidence run: %v", err)
	}
	if result.ExitCode != 0 || result.Summary.Status != "passed" || result.Summary.Layer != "component" {
		t.Fatalf("result = %#v", result)
	}
}

func TestScanRedactedRunDirRejectsResidualSensitivePayload(t *testing.T) {
	runDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runDir, "stdout.log"), []byte("Bearer raw-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	passed, issues := scanRedactedRunDir(runDir)
	if passed || issues != 1 {
		t.Fatalf("scan passed=%v issues=%d", passed, issues)
	}
}

func contains(value, needle string) bool {
	for i := 0; i+len(needle) <= len(value); i++ {
		if value[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
