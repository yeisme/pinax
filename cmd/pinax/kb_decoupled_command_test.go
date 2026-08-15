package main

import (
	"strings"
	"testing"
)

func TestKBCompatibilityCommandsFailClosedWithoutVectorRuntime(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	for _, args := range [][]string{
		{"kb", "doctor", "--vault", root, "--json"},
		{"kb", "search", "semantic query", "--vault", root, "--json"},
		{"kb", "provider", "list", "--vault", root, "--json"},
	} {
		out, err := runCLIExpectError(args...)
		if err == nil {
			t.Fatalf("%v unexpectedly succeeded:\n%s", args, out)
		}
		for _, want := range []string{`"code":"kb_decoupled"`, `"vector_runtime":"removed"`, `"rag_owner":"external"`, "pinax export markdown"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%v missing %q:\n%s", args, want, out)
			}
		}
	}
}
