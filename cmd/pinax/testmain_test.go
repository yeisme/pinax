package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain redirects the pinax profile config dir (XDG_CONFIG_HOME) to a
// per-binary scratch directory so tests that trigger EnsureStoredSecret
// without an explicit encryption ref can never write capsa-sync-* secrets
// into the developer's real user config. Tests that need isolated profile
// state still override this with t.Setenv.
func TestMain(m *testing.M) {
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		if dir, err := os.MkdirTemp("", "pinax-cli-test-xdg-"); err == nil {
			os.Setenv("XDG_CONFIG_HOME", dir)
			defer func() { _ = os.RemoveAll(dir) }()
		}
	}
	_ = filepath.Separator
	os.Exit(m.Run())
}
