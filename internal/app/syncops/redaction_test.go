package syncops

import (
	"strings"
	"testing"

	syncplan "github.com/yeisme/pinax/internal/sync"
)

func TestSanitizeOperationsRedactsObjectMovePaths(t *testing.T) {
	t.Parallel()
	operations := SanitizeOperations([]syncplan.Operation{{Kind: "move", Path: "notes/current.md", FromPath: "notes/old.md", ToPath: "projects/new.md"}}, "hash")
	if len(operations) != 1 {
		t.Fatalf("operations = %#v", operations)
	}
	operation := operations[0]
	for name, value := range map[string]string{"path": operation.Path, "from_path": operation.FromPath, "to_path": operation.ToPath} {
		if !strings.HasPrefix(value, "path_sha256:") || strings.Contains(value, "notes/") || strings.Contains(value, "projects/") {
			t.Fatalf("%s = %q", name, value)
		}
	}
}
