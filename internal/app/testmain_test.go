package app

import (
	"os"
	"testing"
)

// TestMain redirects the pinax profile config dir to a scratch directory so
// app-level tests that persist stored:// secrets without an explicit
// encryption ref can never write into the developer's real user config.
func TestMain(m *testing.M) {
	if os.Getenv("XDG_CONFIG_HOME") == "" {
		if dir, err := os.MkdirTemp("", "pinax-app-test-xdg-"); err == nil {
			os.Setenv("XDG_CONFIG_HOME", dir)
			defer func() { _ = os.RemoveAll(dir) }()
		}
	}
	os.Exit(m.Run())
}
