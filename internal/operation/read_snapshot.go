package operation

import (
	"os"
	"path/filepath"

	"github.com/yeisme/pinax/internal/sqlitedsn"
)

// OpenReadSnapshot opens a bounded private copy without migration or source
// sidecar writes. Missing ledgers remain missing; callers must not initialize.
func OpenReadSnapshot(root string) (*Store, func(), error) {
	path := root
	for _, part := range []string{".pinax", "api", "operations.sqlite"} {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil, nil, operationError(CodeNotFound, "Operation ledger was not found", nil)
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (part != "operations.sqlite" && !info.IsDir()) || (part == "operations.sqlite" && !info.Mode().IsRegular()) {
			return nil, nil, operationError(CodeStoreUnavailable, "Operation ledger is unavailable", nil)
		}
	}
	db, cleanup, err := sqlitedsn.OpenReadSnapshot(path)
	if err != nil {
		return nil, nil, operationError(CodeStoreUnavailable, "Operation ledger snapshot is unavailable", nil)
	}
	return &Store{db: db}, cleanup, nil
}
