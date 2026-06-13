package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultNoteRootCLIPath(t *testing.T) {
	root := t.TempDir()
	cmd := NewRootCommand("test")
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"note", "add", "demo", "--body", "body", "--slug", "demo", "--vault", root, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute note add: %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), `"path":"demo.md"`) || !strings.Contains(out.String(), `"planned_path":"demo.md"`) {
		t.Fatalf("note add json output = %s", out.String())
	}
	if !fileExistsCLI(filepath.Join(root, "demo.md")) || fileExistsCLI(filepath.Join(root, "notes", "demo.md")) {
		t.Fatalf("note add wrote unexpected path; stdout=%s", out.String())
	}
	if strings.TrimSpace(errOut.String()) != "" {
		t.Fatalf("note add wrote stderr: %s", errOut.String())
	}
}

func fileExistsCLI(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
