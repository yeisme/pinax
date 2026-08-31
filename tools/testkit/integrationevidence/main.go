// Package main writes local integration/e2e evidence for Pinax test runs.
//
// 实际证据写入逻辑在 tools/testkit/evidence，本入口只负责拼装 command 和
// pass-through stdout/stderr，再把退出码透传给调用方。
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/yeisme/pinax/tools/testkit/evidence"
)

func main() {
	profile := flag.String("profile", "default", "Integration evidence profile: default, identity, identity-benchmark, agent-memory, agent-continuity, or dsh-pane")
	flag.Parse()
	runID := time.Now().UTC().Format("20060102T150405Z") + fmt.Sprintf("-%d", os.Getpid())
	config := buildConfigForProfile(*profile, runID, os.Stdout, os.Stderr)
	result, err := evidence.Run(config)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "integration evidence error: %v\n", err)
		if result.ExitCode == 0 {
			os.Exit(1)
		}
	}
	_, _ = fmt.Fprintf(os.Stdout, "integration evidence: %s\n", result.RunDir)
	os.Exit(result.ExitCode)
}

func buildConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	command := []string{"go", "test", "./internal/api", "./internal/app", "./internal/dashboard", "./internal/mcpserver", "./tests/e2e", "./internal/cloudclient", "./cmd/pinax", "-run", "TestLocalAPIProjectBoardMatchesProjectionEnvelope|TestLocalAPINoteReadAndProjectItemWritePlan|TestLocalRPCProjectBoardNoteAndProjectItemPlan|TestLocalAPIDatabaseTaskAndGraphCapabilities|TestLocalRPCDatabaseTaskAndGraphCapabilities|TestReadonlyDashboardServesDatabaseTabProjection|TestReadonlyMCPQueryAndDatabaseView|TestProjectBoardWorkspace|TestUnifiedWorkspace|TestObsidianCompat|TestCloud|TestSyncDaemon|TestPluginRuntime|TestDataviewDatabase|TestPublishProfile|TestPublishStaticSite|TestPublishDoc|TestShareLANReadOnly|BidirectionalLinks|JournalIndexTemplate|StarterTemplates|IndexPageRefresh|TemplateRecommend|TemplateCompletion|TemplateNextAction|TestProofLoop|TestServerTransportTwoDeviceConvergence|TestServerTransportConflictPreservesBothSides|TestServerTransportNeverRemoteWriteBeforeCommit|TestClientBootstrapPrincipalAndVaultLifecycle|TestVersionRestoreApplyRevertsBadLocalApply|TestProofLoopRunPreviewEmitsRunIDAndStageFacts|TestProofLoopRunContractAcrossModes|TestPromptImportSearchShowResolveCommands|TestPromptLifecycleAndFeedbackCommands|TestMemoryCaptureListRecallAndContext|TestMemoryRecallRankingSignalsAndRedaction|TestReleaseCore|TestMCPReleaseCore|TestProofLoopReleaseCoreFiveMinute|TestAPIRoutesJSONExposesReleaseCore", "-count=1"}
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           command,
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		ExtraChecks: map[string]any{
			"api_readonly_capabilities": true,
			"project_board_remote":      true,
			"dashboard_database_tab":    true,
			"mcp_database_view":         true,
			"unified_workspace":         true,
			"obsidian_compat":           true,
			"cloud_sync_cli":            true,
			"dataview_database":         true,
			"memory_recall_ranking":     true,
			"prompt_asset_vault":        true,
			"proof_loop":                true,
			"publish_static_profile":    true,
			"publish_static_site":       true,
			"publish_doc":               true,
			"share_lan_readonly":        true,
			"server_sync":               true,
			"sync_daemon":               true,
			"restore_apply":             true,
			"release_core_convergence":  true,
		},
	}
}

func buildConfigForProfile(profile, runID string, stdout, stderr io.Writer) evidence.Config {
	if profile == "dsh-pane" {
		return evidence.Config{
			RunID:             runID,
			ParentDir:         filepath.Join("temp", "integration-test-runs"),
			Command:           []string{"go", "test", "./internal/app", "-run", "Pane", "-count=1"},
			PassThroughStdout: stdout,
			PassThroughStderr: stderr,
			PassStatus:        "passed",
			Layer:             "component",
			ExtraChecks: map[string]any{
				"pane_snapshot_redaction":       true,
				"handwritten_metadata_rejected": true,
				"backlinks_bounded":             true,
				"graph_summary_no_paths":        true,
				"history_timeline_bounded":      true,
			},
		}
	}
	if profile == "agent-memory" {
		return buildAgentMemoryConfig(runID, stdout, stderr)
	}
	if profile == "agent-continuity" {
		return buildAgentContinuityConfig(runID, stdout, stderr)
	}
	if profile == "identity-benchmark" {
		return evidence.Config{
			RunID:             runID,
			ParentDir:         filepath.Join("temp", "integration-test-runs"),
			Command:           []string{"go", "test", "./internal/records", "./internal/index", "./internal/sync", "-run", "^$", "-bench", "Identity|IndexV2|LinkGraph|ObjectSync", "-benchtime=1x", "-benchmem"},
			PassThroughStdout: stdout,
			PassThroughStderr: stderr,
			ExtraChecks: map[string]any{
				"ledger_10k":      true,
				"index_10k":       true,
				"link_graph_100k": true,
				"object_sync_10k": true,
			},
		}
	}
	if profile != "identity" {
		return buildConfig(runID, stdout, stderr)
	}
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./internal/identity", "./internal/records", "./internal/index", "./internal/sync", "./internal/app", "./internal/api", "./internal/mcpserver", "./cmd/pinax", "./tests/e2e", "-run", "Identity|ObjectID|Record|IndexV2|ObjectSync|ManifestMigration|Promotion|SyncDaemon|Proof|Repair|Organize|Metadata|Receipt|ObjectResolver|IdentityFirstTwoDeviceKernel", "-count=1"},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		ExtraChecks: map[string]any{
			"canonical_object_identity": true,
			"record_ledger":             true,
			"object_first_index":        true,
			"object_first_sync":         true,
			"manifest_v2_migration":     true,
			"agent_proof_receipts":      true,
			"two_device_kernel":         true,
		},
	}
}

// buildAgentMemoryConfig 构造 agent memory runtime 的 integration evidence profile。
func buildAgentMemoryConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./internal/agentprotocol", "./internal/agentmemory", "./internal/agentcontext", "./internal/agentadapter", "./internal/app", "./internal/mcpserver", "./internal/api", "./pkg/agentmemory", "./tests/e2e", "-run", "AgentMemory|AgentContext|AgentHandoff|AgentFeedback|AgentTool|CrossAgent|Redaction|Negotiate|Lifecycle|Policy|Compile|Recall", "-count=1"},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		ExtraChecks: map[string]any{
			"protocol_validation":      true,
			"additive_store":           true,
			"permission_first_context": true,
			"proposal_lifecycle":       true,
			"cross_agent_handoff":      true,
			"transport_parity":         true,
			"credential_safe":          true,
		},
	}
}

// buildAgentContinuityConfig 构造 agent continuity experience 的 integration evidence profile。
func buildAgentContinuityConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./internal/agentcontinuity", "./internal/memoryinbox", "./internal/app", "./internal/dashboard", "./internal/output", "./cmd/pinax", "./tests/e2e", "-run", "Continuity|Continue|Inbox|Review|TrustCenter|CrossAgent|Redaction|Compile|Aggregate|Recorded|WeeklyReview|Feedback|LegacyDefault|Checkpoint", "-count=1"},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		ExtraChecks: map[string]any{
			"continuity_pack":            true,
			"memory_inbox":               true,
			"trust_center":               true,
			"cross_agent_flow":           true,
			"credential_safe":            true,
			"continuity_binding":         true,
			"continuity_evidence":        true,
			"repository_source_resolver": true,
			"cross_runtime_handoff":      true,
		},
	}
}
