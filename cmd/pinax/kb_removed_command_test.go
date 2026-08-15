package main

import (
	"strings"
	"testing"
)

func TestKBCommandsAreRemoved(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")

	for _, args := range [][]string{
		{"kb"},
		{"kb", "doctor", "--vault", root, "--json"},
		{"kb", "search", "semantic query", "--vault", root, "--json"},
		{"kb", "provider", "list", "--vault", root, "--json"},
	} {
		out, err := runCLIExpectError(args...)
		if err == nil {
			t.Fatalf("%v unexpectedly succeeded:\n%s", args, out)
		}
		if !strings.Contains(err.Error(), "unknown command") {
			t.Fatalf("%v should report unknown command after KB removal: err=%v out=%s", args, err, out)
		}
		if strings.Contains(out+err.Error(), "kb_decoupled") {
			t.Fatalf("%v still returns the removed kb_decoupled compat error:\n%s", args, out)
		}
	}
}
