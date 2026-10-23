package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

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
