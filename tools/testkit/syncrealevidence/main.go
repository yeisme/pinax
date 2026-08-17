package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yeisme/pinax/tools/testkit/evidence"
)

func main() {
	if os.Getenv("PINAX_SYNC_REAL_ENDPOINT") == "" {
		_, _ = fmt.Fprintln(os.Stderr, "PINAX_SYNC_REAL_ENDPOINT is required for real sync evidence")
		os.Exit(2)
	}
	runID := time.Now().UTC().Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	result, err := evidence.Run(evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./tests/e2e", "-run", "IdentityFirstRealSyncSmoke", "-count=1"},
		PassThroughStdout: os.Stdout,
		PassThroughStderr: os.Stderr,
		ExtraChecks: map[string]any{
			"real_transport":        true,
			"manifest_v2_promotion": true,
			"credential_safe":       true,
		},
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "real sync evidence error: %v\n", err)
		if result.ExitCode == 0 {
			os.Exit(1)
		}
	}
	_, _ = fmt.Fprintf(os.Stdout, "real sync evidence: %s\n", result.RunDir)
	os.Exit(result.ExitCode)
}
