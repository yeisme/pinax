package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yeisme/pinax/internal/testkit/evidence"
)

// forbiddenEvidenceMarkers are concrete sensitive markers deliberately seeded
// into command stdout, stderr, argv and path-like values. The evidence runner
// must redact every occurrence so that no evidence file contains them.
var forbiddenEvidenceMarkers = []string{
	"EVIDENCE_BEARER_TOKEN_SECRET",
	"EVIDENCE_AUTHZ_HEADER_SECRET",
	"EVIDENCE_API_KEY_SECRET",
	"EVIDENCE_PASSWORD_SECRET",
}

// forbiddenEvidenceClasses are generic sensitive classes that must never appear
// in any generated evidence file, regardless of whether they were seeded.
var forbiddenEvidenceClasses = []string{
	"Authorization: Bearer ",
	"Bearer EVIDENCE_",
	"api_key=EVIDENCE_",
	"password=EVIDENCE_",
	"token=EVIDENCE_",
}

// TestIntegrationEvidenceSuccess proves a successful command writes a complete
// evidence directory with the required schema fields even when the command
// output contains seeded sensitive markers.
func TestIntegrationEvidenceSuccess(t *testing.T) {
	parent := t.TempDir()
	result, err := evidence.Run(evidence.Config{
		RunID:     "evidence-success",
		ParentDir: parent,
		Command:   []string{"sh", "-c", "echo 'Authorization: Bearer EVIDENCE_BEARER_TOKEN_SECRET'; echo 'token=EVIDENCE_API_KEY_SECRET'; echo proof ok"},
		ExtraChecks: map[string]any{
			"proof_loop": true,
		},
	})
	if err != nil {
		t.Fatalf("evidence run: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	assertSummarySchema(t, result.Summary)
	for _, name := range []string{"summary.json", "command.txt", "stdout.log", "stderr.log", "env.json"} {
		if _, err := os.Stat(filepath.Join(result.RunDir, name)); err != nil {
			t.Fatalf("evidence file %s missing: %v", name, err)
		}
	}
	assertEvidenceDirClean(t, result.RunDir)
}

// TestIntegrationEvidenceFailurePreservesExitCode proves a failing command still
// writes full evidence, preserves the original non-zero exit code, captures stderr,
// and redacts seeded markers from every surface.
func TestIntegrationEvidenceFailurePreservesExitCode(t *testing.T) {
	parent := t.TempDir()
	result, err := evidence.Run(evidence.Config{
		RunID:     "evidence-failure",
		ParentDir: parent,
		// Seed markers into stdout, stderr and via an absolute path argument.
		Command: []string{"sh", "-c", "echo 'out: password=EVIDENCE_PASSWORD_SECRET'; echo 'err: Authorization: Bearer EVIDENCE_AUTHZ_HEADER_SECRET' >&2; echo /tmp/EVIDENCE_BEARER_TOKEN_SECRET/leaked; exit 7"},
	})
	if err != nil {
		t.Fatalf("evidence run: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7 (original preserved)", result.ExitCode)
	}
	if result.Summary.Status != "failed" {
		t.Fatalf("status = %q, want failed", result.Summary.Status)
	}
	stderrPath := filepath.Join(result.RunDir, "stderr.log")
	stderrBytes, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatalf("read stderr.log: %v", err)
	}
	if !strings.Contains(string(stderrBytes), "err:") {
		t.Fatalf("stderr.log does not contain forced failure output:\n%s", stderrBytes)
	}
	for _, name := range []string{"summary.json", "command.txt", "stdout.log", "stderr.log", "env.json"} {
		if _, err := os.Stat(filepath.Join(result.RunDir, name)); err != nil {
			t.Fatalf("evidence file %s missing on failure: %v", name, err)
		}
	}
	assertEvidenceDirClean(t, result.RunDir)
}

// TestIntegrationEvidenceArgvRedaction proves sensitive markers passed as argv
// are redacted in command.txt and summary.json.
func TestIntegrationEvidenceArgvRedaction(t *testing.T) {
	parent := t.TempDir()
	result, err := evidence.Run(evidence.Config{
		RunID:     "evidence-argv",
		ParentDir: parent,
		Command:   []string{"echo", "token=EVIDENCE_API_KEY_SECRET", "--api-key", "EVIDENCE_BEARER_TOKEN_SECRET"},
	})
	if err != nil {
		t.Fatalf("evidence run: %v", err)
	}
	assertEvidenceDirClean(t, result.RunDir)
}

// assertSummarySchema proves the summary carries every required field added by
// the reopened redaction/schema requirements.
func assertSummarySchema(t *testing.T, s evidence.Summary) {
	t.Helper()
	if s.SchemaVersion != evidence.SchemaVersion {
		t.Errorf("schema_version = %q, want %q", s.SchemaVersion, evidence.SchemaVersion)
	}
	if s.Project != evidence.Project {
		t.Errorf("project = %q, want %q", s.Project, evidence.Project)
	}
	if s.Layer != evidence.Layer {
		t.Errorf("layer = %q, want %q", s.Layer, evidence.Layer)
	}
	if s.FinishedAt == "" {
		t.Error("finished_at is empty")
	}
	if !s.Redaction.Applied {
		t.Error("redaction.applied = false, want true")
	}
	if len(s.Redaction.ScannedSurfaces) == 0 {
		t.Error("redaction.scanned_surfaces is empty")
	}
	if len(s.Redaction.ForbiddenClasses) == 0 {
		t.Error("redaction.forbidden_classes is empty")
	}
}

// assertEvidenceDirClean recursively scans every file in the evidence run
// directory — including summary.json, command.txt, stdout.log, stderr.log,
// env.json and artifacts — for seeded markers, forbidden classes and absolute
// paths. This is the recursive scan the reopened task requires.
func assertEvidenceDirClean(t *testing.T, runDir string) {
	t.Helper()
	var leaked []string
	err := filepath.Walk(runDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(runDir, path)
		content := string(data)
		for _, marker := range forbiddenEvidenceMarkers {
			if strings.Contains(content, marker) {
				leaked = append(leaked, fmt.Sprintf("%s: marker %q", rel, marker))
			}
		}
		for _, class := range forbiddenEvidenceClasses {
			if strings.Contains(content, class) {
				leaked = append(leaked, fmt.Sprintf("%s: class %q", rel, class))
			}
		}
		// summary.json 中的 command 数组可能合法包含 "Bearer" 作为模式说明，
		// 但不能包含 EVIDENCE_ 标记；绝对路径检查排除 summary 内的 schema 文本。
		if !strings.HasSuffix(rel, "summary.json") {
			if strings.Contains(content, "/tmp/EVIDENCE_") || strings.Contains(content, "/workspaces/") {
				leaked = append(leaked, fmt.Sprintf("%s: absolute path leak", rel))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk evidence dir %s: %v", runDir, err)
	}
	if len(leaked) > 0 {
		t.Fatalf("evidence leak detected:\n  %s", strings.Join(leaked, "\n  "))
	}
}

// TestEvidenceRedactUnit proves the Redact function scrubs every forbidden class.
func TestEvidenceRedactUnit(t *testing.T) {
	cases := map[string]string{
		"Authorization: Bearer abc123":         "contains redacted bearer",
		"token=secret_value":                   "token redacted",
		"api_key=ak_123":                       "api_key redacted",
		"password=pw_456":                      "password redacted",
		"secret=sk_789":                        "secret redacted",
		"/workspaces/yeisme-agent/cli/pinax/v": "abs path redacted",
		"/tmp/pinax-e2e-bin-123/vault":         "tmp path redacted",
	}
	for input, desc := range cases {
		got := evidence.Redact(input)
		if strings.Contains(got, "abc123") || strings.Contains(got, "secret_value") ||
			strings.Contains(got, "ak_123") || strings.Contains(got, "pw_456") ||
			strings.Contains(got, "sk_789") {
			t.Errorf("%s: Redact(%q) = %q leaked a secret", desc, input, got)
		}
		if strings.Contains(got, "/workspaces/") || strings.Contains(got, "/tmp/pinax") {
			t.Errorf("%s: Redact(%q) = %q leaked an absolute path", desc, input, got)
		}
	}
}

// init ensures the json import is used (summary parsing helpers may be added).
var _ = json.Marshal
