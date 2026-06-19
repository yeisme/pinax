package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePath = "github.com/yeisme/pinax"

var capabilityPackages = []string{
	"noteops",
	"searchops",
	"vaultops",
	"templateops",
	"syncops",
	"versionops",
	"briefingops",
	"planningops",
	"publishops",
}

func TestCapabilityPackagesDeclareOwnership(t *testing.T) {
	repoRoot := findRepoRoot(t)
	for _, pkg := range capabilityPackages {
		pkg := pkg
		t.Run(pkg, func(t *testing.T) {
			docPath := filepath.Join(repoRoot, "internal", "app", pkg, "doc.go")
			content, err := os.ReadFile(docPath)
			if err != nil {
				t.Fatalf("capability package %s must declare ownership in doc.go: %v", pkg, err)
			}

			text := string(content)
			required := []string{"Command family:", "Responsibility:", "Prohibited dependencies:", "Focused tests:"}
			for _, marker := range required {
				if !strings.Contains(text, marker) {
					t.Fatalf("%s missing %q marker", docPath, marker)
				}
			}
		})
	}
}

func TestCLIImportsAppFacadeOnly(t *testing.T) {
	repoRoot := findRepoRoot(t)
	blocked := capabilityImportSet()
	checkGoImports(t, repoRoot, []string{"internal/cli", "cmd/pinax"}, func(path, imp string) {
		if blocked[imp] {
			t.Fatalf("%s imports %s directly; use internal/app Service facade", path, imp)
		}
	})
}

func TestCapabilityPackagesDoNotImportCLIOrOutput(t *testing.T) {
	repoRoot := findRepoRoot(t)
	var dirs []string
	for _, pkg := range capabilityPackages {
		dirs = append(dirs, filepath.Join("internal/app", pkg))
	}

	blocked := map[string]bool{
		modulePath + "/internal/cli":    true,
		modulePath + "/internal/output": true,
	}
	checkGoImports(t, repoRoot, dirs, func(path, imp string) {
		if blocked[imp] {
			t.Fatalf("%s imports %s; capability packages must return domain or projection data", path, imp)
		}
	})
}

func capabilityImportSet() map[string]bool {
	imports := make(map[string]bool, len(capabilityPackages))
	for _, pkg := range capabilityPackages {
		imports[modulePath+"/internal/app/"+pkg] = true
	}
	return imports
}

func checkGoImports(t *testing.T, repoRoot string, dirs []string, visit func(path, imp string)) {
	t.Helper()
	for _, dir := range dirs {
		absDir := filepath.Join(repoRoot, dir)
		if _, err := os.Stat(absDir); os.IsNotExist(err) {
			continue
		}

		err := filepath.WalkDir(absDir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}

			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, spec := range file.Imports {
				visit(path, strings.Trim(spec.Path.Value, "\""))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan imports under %s: %v", absDir, err)
		}
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root from working directory")
		}
		dir = parent
	}
}
