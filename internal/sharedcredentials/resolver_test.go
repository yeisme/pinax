package sharedcredentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileResolver(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configDir)
	secretPath := filepath.Join(configDir, "yeisme", "credentialctl", "secrets", "openai", "personal-default")
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte("sk-shared"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewResolver()
	if err != nil {
		t.Fatal(err)
	}
	ref, err := ParseRef("yeisme-credential://openai/personal-default")
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := resolver.Resolve("pinax", "embedding", ref)
	if err != nil {
		t.Fatal(err)
	}
	if string(resolution.Secret) != "sk-shared" || resolution.Backend != "file" {
		t.Fatalf("unexpected resolution: %#v", resolution)
	}
	if _, err := resolver.Resolve("other", "embedding", ref); err == nil {
		t.Fatal("expected disallowed consumer error")
	}
}

func TestParseRefRejectsPathTraversal(t *testing.T) {
	for _, value := range []string{
		"yeisme-credential://../secret",
		"yeisme-credential://openai/..",
		"yeisme-credential://openai/path\\escape",
	} {
		if _, err := ParseRef(value); err == nil {
			t.Fatalf("ParseRef(%q) should fail", value)
		}
	}
}
