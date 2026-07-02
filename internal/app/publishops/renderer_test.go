package publishops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRendererAdapterUsesFakeExecutableAndRedactsStderr(t *testing.T) {
	root := t.TempDir()
	fake := filepath.Join(root, "renderer")
	logPath := filepath.Join(root, "renderer.log")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\n' \"$*\" >> \"$PINAX_TEST_RENDERER_LOG\"\necho 'Authorization: Bearer raw-token token=raw path=/home/user/vault' >&2\nmkdir -p \"$4\"\nprintf '%s\n' '<html>ok</html>' > \"$4/index.html\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINAX_TEST_RENDERER_LOG", logPath)
	bundleRoot := filepath.Join(root, "bundle")
	outDir := filepath.Join(root, "out")
	if err := os.MkdirAll(bundleRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := (RendererAdapter{Executable: fake, Timeout: time.Second}).RenderStatic(context.Background(), RendererRequest{BundleRoot: bundleRoot, OutDir: outDir, BaseURL: "/", Theme: "builtin:pinax-encyclopedia", RendererVersion: "test"})
	if err != nil {
		t.Fatalf("render static: %v", err)
	}
	if result.CallID == "" || result.DurationMS <= 0 || result.Stderr == "" {
		t.Fatalf("renderer result missing call metadata: %#v", result)
	}
	if strings.Contains(result.Stderr, "raw-token") || strings.Contains(result.Stderr, "/home/user/vault") || !strings.Contains(result.Stderr, "[REDACTED]") {
		t.Fatalf("stderr was not redacted: %#v", result)
	}
	log := mustReadPublishOpsFile(t, logPath)
	for _, want := range []string{"--bundle", bundleRoot, "--out", outDir, "--base-url", "/", "--theme", "builtin:pinax-encyclopedia", "--renderer-version", "test"} {
		if !strings.Contains(log, want) {
			t.Fatalf("renderer log missing %q: %s", want, log)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "index.html")); err != nil {
		t.Fatalf("fake renderer output missing: %v", err)
	}
}
