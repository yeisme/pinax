package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain shrinks PBKDF2 iterations for the whole test binary; key
// derivation at 600k costs 150-400ms per call and dominates the cloud/sync
// test suites. Both v2 and legacy derivations shrink consistently, so KeyID
// relationships (and the legacy-compat keychain tests) keep holding.
func TestMain(m *testing.M) {
	SetKeyDerivationIterationsForTesting(1000)
	os.Exit(m.Run())
}

// TestKeyDerivationOverrideIsTestOnly enforces that the override setter is
// never referenced from production code: walk the module, skip vendor/, and
// fail if any file that is not a _test.go references the setter outside
// internal/remote/crypto.go (its definition site).
func TestKeyDerivationOverrideIsTestOnly(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
	violations := []string{}
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			if entry != nil && entry.IsDir() && (entry.Name() == "vendor" || entry.Name() == ".git" || entry.Name() == "dist") {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if strings.HasSuffix(path, "internal/remote/crypto.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(body), "SetKeyDerivationIterationsForTesting") {
			violations = append(violations, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("production files reference SetKeyDerivationIterationsForTesting: %v", violations)
	}
}
