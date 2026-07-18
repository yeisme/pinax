package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncManifestAuditAndPlanCommands(t *testing.T) {
	root := t.TempDir()
	writeCLIFixture(t, filepath.Join(root, "notes", "alpha.md"), "---\nschema_version: pinax.note.v1\nnote_id: 01982d84-2b48-7000-8000-000000000031\ntitle: Alpha\n---\n\n# Alpha\n")
	audit := runCLI(t, "sync", "manifest", "audit", "--device-id", "device-a", "--vault", root, "--json")
	for _, expected := range []string{`"command":"sync.manifest.audit"`, `"eligible":"true"`, `"writes":"false"`} {
		if !strings.Contains(audit, expected) {
			t.Fatalf("audit output missing %q:\n%s", expected, audit)
		}
	}
	plan := runCLI(t, "sync", "manifest", "plan", "--device-id", "device-a", "--save", "--vault", root, "--json")
	for _, expected := range []string{`"command":"sync.manifest.plan"`, `"saved":"true"`, `manifest-plan-`} {
		if !strings.Contains(plan, expected) {
			t.Fatalf("plan output missing %q:\n%s", expected, plan)
		}
	}
}
