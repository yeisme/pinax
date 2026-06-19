package publishops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHugoAdapterUsesFakeExecutableAndRedactsStderr(t *testing.T) {
	root := t.TempDir()
	fake := filepath.Join(root, "hugo")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nif [ \"$1\" = \"version\" ]; then echo 'hugo v0.130.0'; exit 0; fi\necho 'Authorization: Bearer raw-token token=raw path=notes/private.md' >&2\nmkdir -p \"$4\"\nprintf '%s\n' '<html>ok</html>' > \"$4/index.html\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := HugoAdapter{Executable: fake, Timeout: time.Second}

	version, err := adapter.Version(context.Background())
	if err != nil || version.Version != "hugo v0.130.0" || version.CallID == "" {
		t.Fatalf("version = %#v err=%v", version, err)
	}
	source := filepath.Join(root, "stage")
	dest := filepath.Join(root, "out")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Build(context.Background(), source, dest)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if result.CallID == "" || result.DurationMS <= 0 {
		t.Fatalf("build result missing call metadata: %#v", result)
	}
	if !strings.Contains(result.Stderr, "[REDACTED]") || strings.Contains(result.Stderr, "raw-token") || strings.Contains(result.Stderr, "notes/private.md") {
		t.Fatalf("stderr was not redacted: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(dest, "index.html")); err != nil {
		t.Fatalf("fake hugo output missing: %v", err)
	}
}

func TestHugoAdapterFailureMatrixUsesStableRedactedResults(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing-hugo")
	if result, err := (HugoAdapter{Executable: missing, Timeout: time.Second}).Build(context.Background(), root, filepath.Join(root, "out")); err == nil || result.CallID == "" || result.DurationMS == 0 {
		t.Fatalf("missing hugo should return call metadata and error: result=%#v err=%v", result, err)
	}

	failing := filepath.Join(root, "hugo-fail")
	if err := os.WriteFile(failing, []byte("#!/bin/sh\necho 'Authorization: Bearer raw-token token=raw path=/home/user/vault' >&2\nexit 42\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := (HugoAdapter{Executable: failing, Timeout: time.Second}).Build(context.Background(), root, filepath.Join(root, "out"))
	if err == nil || result.CallID == "" || result.Stderr == "" {
		t.Fatalf("non-zero hugo should return redacted stderr metadata: result=%#v err=%v", result, err)
	}
	if strings.Contains(result.Stderr, "raw-token") || strings.Contains(result.Stderr, "/home/user/vault") {
		t.Fatalf("non-zero stderr was not redacted: %#v", result)
	}

	slow := filepath.Join(root, "hugo-slow")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nsleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err = (HugoAdapter{Executable: slow, Timeout: time.Millisecond}).Build(context.Background(), root, filepath.Join(root, "out"))
	if err == nil || !strings.Contains(err.Error(), "timed out") || result.CallID == "" {
		t.Fatalf("timeout should return call metadata and timeout error: result=%#v err=%v", result, err)
	}
}
