// Command mcpevidence runs Pinax MCP process/component tests and writes the
// standard redacted integration evidence bundle.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yeisme/pinax/tools/testkit/evidence"
)

func main() {
	runID := time.Now().UTC().Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	result, err := evidence.Run(evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./internal/mcpserver", "./internal/cli", "./cmd/pinax", "./tests/e2e", "-run", "InputIntake|MCPProtocolLifecycle|MCPTransportParity|Stdout|Stderr|JSONRPC|Signal|Panic|ToolSchema|StructuredContent|ResourceTemplate", "-count=1"},
		PassThroughStdout: os.Stdout,
		PassThroughStderr: os.Stderr,
		PassStatus:        "passed",
		Layer:             "e2e",
		ExtraChecks: map[string]any{
			"current_protocol_discovery": true,
			"legacy_initialize":          true,
			"manifest_parity":            true,
			"resource_read":              true,
			"tool_call":                  true,
			"stdout_jsonrpc_only":        true,
		},
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "MCP integration evidence error: %v\n", err)
		if result.ExitCode == 0 {
			os.Exit(1)
		}
	}
	_, _ = fmt.Fprintf(os.Stdout, "integration evidence: %s\n", result.RunDir)
	os.Exit(result.ExitCode)
}
