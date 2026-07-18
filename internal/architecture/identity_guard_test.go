package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestCanonicalIdentityIsNotDerivedInsideApplicationCode(t *testing.T) {
	repoRoot := findRepoRoot(t)
	allowedLegacyCalls := map[string]map[string]bool{
		"internal/app/service.go": {
			"ensureFrontmatter":          true,
			"buildNoteContentWithStatus": true,
			"loadMutableResolvedNote":    true,
			"loadMutableNote":            true,
		},
		"internal/app/records.go": {
			"appendNoteRecordEvent": true,
		},
	}

	err := filepath.WalkDir(filepath.Join(repoRoot, "internal", "app"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || filepath.Base(path) == "legacy_identity.go" {
			return nil
		}
		fileSet := token.NewFileSet()
		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch target := call.Fun.(type) {
				case *ast.Ident:
					if target.Name == "stableNoteID" {
						t.Errorf("%s:%d calls forbidden stableNoteID; canonical IDs must come from IdentityAllocator", rel, fileSet.Position(call.Pos()).Line)
					}
					if target.Name == "legacyNoteIDForPath" && !allowedLegacyCalls[rel][function.Name.Name] {
						t.Errorf("%s:%d calls legacyNoteIDForPath from %s; add a migration path instead of deriving canonical identity", rel, fileSet.Position(call.Pos()).Line, function.Name.Name)
					}
				case *ast.SelectorExpr:
					packageName, ok := target.X.(*ast.Ident)
					if !ok || packageName.Name != "identity" {
						return true
					}
					if target.Sel.Name == "NewObjectID" {
						t.Errorf("%s:%d calls identity.NewObjectID directly; use Service IdentityAllocator", rel, fileSet.Position(call.Pos()).Line)
					}
					if target.Sel.Name == "LegacyNoteIDFromPath" {
						t.Errorf("%s:%d calls identity.LegacyNoteIDFromPath directly; only internal/app/legacy_identity.go may bridge legacy IDs", rel, fileSet.Position(call.Pos()).Line)
					}
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLegacyIdentityAdapterIsSinglePurpose(t *testing.T) {
	repoRoot := findRepoRoot(t)
	path := filepath.Join(repoRoot, "internal", "app", "legacy_identity.go")
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse legacy identity adapter: %v", err)
	}
	for _, spec := range file.Imports {
		importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
		if unquoteErr != nil {
			t.Fatal(unquoteErr)
		}
		if importPath != modulePath+"/internal/identity" {
			t.Fatalf("legacy identity adapter imports %q; it must only bridge internal/identity", importPath)
		}
	}
}
