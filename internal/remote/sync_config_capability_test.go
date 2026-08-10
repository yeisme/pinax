package remote

import (
	"strings"
	"testing"
)

// TestSyncConfigCapabilityRequiresRoundTrip verifies task 6.8: a declaration
// carrying requires.capabilities round-trips through YAML parse + Validate so a
// migrated repository can declare the capabilities a binary must support.
func TestSyncConfigCapabilityRequiresRoundTrip(t *testing.T) {
	root := t.TempDir()
	decl := "schema_version: \"" + SyncConfigSchemaVersion + "\"\n" +
		"backend:\n  kind: s3-direct\n  endpoint: s3://bucket/main/\n" +
		"  s3:\n    bucket: bucket\n    endpoint: https://cos.example.com\n    region: ap-shanghai\n    credential_mode: repository-encrypted\n" +
		"workspace:\n  workspace_id: ws-cap\n" +
		"secrets:\n  credential_id: cred-1\n  encryption_key_id: enc-1\n" +
		"requires:\n  capabilities:\n    - repository-encrypted-s3-v1\n    - capsa-remote-commit-v1\n    - pull-only-bootstrap-v1\n"
	if err := writeTestDeclaration(root, decl); err != nil {
		t.Fatalf("write declaration: %v", err)
	}
	cfg, err := LoadSyncConfig(root)
	if err != nil {
		t.Fatalf("LoadSyncConfig: %v", err)
	}
	if len(cfg.Requires.Capabilities) != 3 {
		t.Fatalf("expected 3 capabilities, got %v", cfg.Requires.Capabilities)
	}
	for _, want := range []string{"repository-encrypted-s3-v1", "capsa-remote-commit-v1", "pull-only-bootstrap-v1"} {
		found := false
		for _, got := range cfg.Requires.Capabilities {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("capability %q not round-tripped: %v", want, cfg.Requires.Capabilities)
		}
	}
}

// TestSyncConfigCapabilityUnknownFieldRejected verifies the strict unknown-field
// rejection remains active: a typo'd requires key is rejected so an old binary
// cannot silently ignore a capability it does not understand.
func TestSyncConfigCapabilityUnknownFieldRejected(t *testing.T) {
	root := t.TempDir()
	decl := "schema_version: \"" + SyncConfigSchemaVersion + "\"\n" +
		"backend:\n  kind: s3-direct\n  endpoint: s3://bucket/main/\n" +
		"workspace:\n  workspace_id: ws-cap\n" +
		"secrets:\n  encryption_key_id: enc-1\n" +
		"requires:\n  capabiliteez: [\"bogus\"]\n"
	if err := writeTestDeclaration(root, decl); err != nil {
		t.Fatalf("write declaration: %v", err)
	}
	if _, err := LoadSyncConfig(root); err == nil {
		t.Fatal("expected unknown-field rejection for malformed requires block")
	} else if !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "field") {
		t.Fatalf("unexpected error: %v", err)
	}
}
