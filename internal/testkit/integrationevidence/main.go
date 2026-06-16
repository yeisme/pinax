// Package main writes local integration/e2e evidence for Pinax test runs.
//
// 实际证据写入逻辑在 internal/testkit/evidence，本入口只负责拼装 command 和
// pass-through stdout/stderr，再把退出码透传给调用方。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yeisme/pinax/internal/testkit/evidence"
)

func main() {
	runID := time.Now().UTC().Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	command := []string{"go", "test", "./internal/api", "./tests/e2e", "./internal/cloudclient", "./cmd/pinax", "-run", "TestLocalAPIProjectBoardMatchesProjectionEnvelope|TestLocalAPINoteReadAndProjectItemWritePlan|TestLocalRPCProjectBoardNoteAndProjectItemPlan|TestProjectBoardWorkspace|BidirectionalLinks|JournalIndexTemplate|StarterTemplates|IndexPageRefresh|TemplateRecommend|TemplateCompletion|TemplateNextAction|TestProofLoop|TestServerTransportTwoDeviceConvergence|TestServerTransportConflictPreservesBothSides|TestServerTransportNeverRemoteWriteBeforeCommit|TestClientBootstrapPrincipalAndVaultLifecycle|TestVersionRestoreApplyRevertsBadLocalApply|TestProofLoopRunPreviewEmitsRunIDAndStageFacts|TestProofLoopRunContractAcrossModes", "-count=1"}
	result, err := evidence.Run(evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           command,
		PassThroughStdout: os.Stdout,
		PassThroughStderr: os.Stderr,
		ExtraChecks: map[string]any{
			"project_board_remote": true,
			"proof_loop":           true,
			"server_sync":          true,
			"restore_apply":        true,
		},
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "integration evidence error: %v\n", err)
		if result.ExitCode == 0 {
			os.Exit(1)
		}
	}
	_, _ = fmt.Fprintf(os.Stdout, "integration evidence: %s\n", result.RunDir)
	os.Exit(result.ExitCode)
}
