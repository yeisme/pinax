package cli

import (
	"strings"
	"testing"
)

// TestSyncDiffUnlockFlagsPresent verifies task 6.7: `pinax sync diff` exposes
// the same symmetric unlock flags as `pinax sync pull` so a repository-encrypted
// vault can be unlocked for a remote-aware diff.
func TestSyncDiffUnlockFlagsPresent(t *testing.T) {
	out, _, err := runCLIWithStdin(t, "", "sync", "diff", "--help")
	if err != nil {
		t.Fatalf("sync diff help: %v", err)
	}
	for _, flag := range []string{"--unlock", "--unlock-ref", "--passphrase-file", "--env-var"} {
		if !strings.Contains(out, flag) {
			t.Fatalf("sync diff help missing %s:\n%s", flag, out)
		}
	}
}

// TestSyncPushUnlockFlagsPresent verifies task 6.7: `pinax sync push` exposes
// the symmetric unlock flags so a repository-encrypted vault can be unlocked
// for a remote commit without falling back to the device-local profile chain.
func TestSyncPushUnlockFlagsPresent(t *testing.T) {
	out, _, err := runCLIWithStdin(t, "", "sync", "push", "--help")
	if err != nil {
		t.Fatalf("sync push help: %v", err)
	}
	for _, flag := range []string{"--unlock", "--unlock-ref", "--passphrase-file", "--env-var"} {
		if !strings.Contains(out, flag) {
			t.Fatalf("sync push help missing %s:\n%s", flag, out)
		}
	}
}
