package main

import (
	"strings"
	"testing"
)

func TestSyncKeysReportsDerivationAndMigrationHint(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, root+"/notes/alpha.md", "# Alpha\n")
	objectRoot := t.TempDir()
	runCLI(t, "capsa", "login", "--endpoint", "file://"+objectRoot, "--workspace", "w_keys", "--device", "dev", "--secret-ref", "plain:secret-k", "--encryption-secret-ref", "plain:secret-k", "--vault", root, "--json")

	keysBefore := runCLI(t, "sync", "keys", "--vault", root, "--json")
	assertJSONCommandStatus(t, keysBefore, "sync.keys", "success")
	if !strings.Contains(keysBefore, `"remote_state":"empty"`) {
		t.Fatalf("empty remote keys output:\n%s", keysBefore)
	}

	runCLI(t, "sync", "push", "--target", "capsa", "--yes", "--vault", root, "--json")
	keysAfter := runCLI(t, "sync", "keys", "--vault", root, "--json")
	for _, want := range []string{`"remote_derivation":"v2"`, `"reencryption_required":"false"`} {
		if !strings.Contains(keysAfter, want) {
			t.Fatalf("post-push keys missing %s:\n%s", want, keysAfter)
		}
	}
	if strings.Contains(keysAfter, `"name":"reencrypt"`) {
		t.Fatalf("v2 remote should not suggest re-encryption:\n%s", keysAfter)
	}
}

// TestSyncKeysDetectsLegacyRemoteAndReencrypts simulates a pre-v2 remote:
// a manifest envelope encrypted with the legacy derivation must be flagged,
// then one push re-encrypts it and keys reports v2.
func TestSyncKeysDetectsLegacyRemoteAndReencrypts(t *testing.T) {
	root := t.TempDir()
	runCLI(t, "init", root, "--title", "Vault", "--json")
	writeCLIFixture(t, root+"/notes/alpha.md", "# Alpha legacy\n")
	objectRoot := t.TempDir()
	runCLI(t, "capsa", "login", "--endpoint", "file://"+objectRoot, "--workspace", "w_legacy", "--device", "dev", "--secret-ref", "plain:secret-k", "--encryption-secret-ref", "plain:secret-k", "--vault", root, "--json")
	runCLI(t, "sync", "push", "--target", "capsa", "--yes", "--vault", root, "--json")

	// Rewrite the remote manifest envelope under the legacy derivation.
	reencryptRemoteManifestLegacy(t, root, objectRoot, "w_legacy")

	keysLegacy := runCLI(t, "sync", "keys", "--vault", root, "--json")
	for _, want := range []string{`"remote_derivation":"legacy"`, `"reencryption_required":"true"`, `"name":"reencrypt"`} {
		if !strings.Contains(keysLegacy, want) {
			t.Fatalf("legacy keys output missing %s:\n%s", want, keysLegacy)
		}
	}

	// Pull still reads legacy data, proving read compatibility.
	pull := runCLI(t, "sync", "pull", "--target", "capsa", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, pull, "sync.pull", "success")

	push := runCLI(t, "sync", "push", "--target", "capsa", "--yes", "--vault", root, "--json")
	assertJSONCommandStatus(t, push, "sync.push", "success")
	keysV2 := runCLI(t, "sync", "keys", "--vault", root, "--json")
	if !strings.Contains(keysV2, `"remote_derivation":"v2"`) {
		t.Fatalf("post-reencrypt keys should report v2:\n%s", keysV2)
	}
}
