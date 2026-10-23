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
	profile := flag.String("profile", "default", "Integration evidence profile: default, identity, identity-benchmark, agent-memory, agent-continuity, personal-assistant-grounding, prompt-catalog, mcp-protocol, sdk-security, operation-recovery, remote-write-parity, pane, knowledge-source-adapter, research-brief, daily-knowledge, daily-knowledge-quality, daily-review-followthrough, agent-note-proposal, rendered-view-parity, human-reading-api, human-reading-access, human-reading-document, human-reading-resources, human-reading-session, human-reading-client, human-reading-quality, human-reading-write-capture, human-reading-write-edit, human-reading-write-organize, human-reading-write-client, human-reading-write-api, human-reading-write-quality, notebook-editor-source, notebook-navigation, notebook-daily, notebook-backend, notebook-backend-packages, notebook-quality, or notebook-backend-final")
	flag.Parse()
	if *profile == "mcp-apps-browser" {
		_, _ = fmt.Fprintln(os.Stderr, "The MCP Apps browser profile has been retired. Use --profile mcp-collaboration for Markdown tool verification.")
		os.Exit(2)
	}
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
	command := []string{"go", "test", "./internal/api", "./internal/app", "./internal/dashboard", "./internal/mcpserver", "./tests/e2e", "./internal/cloudclient", "./cmd/pinax", "-run", "TestLocalAPIProjectBoardMatchesProjectionEnvelope|TestLocalAPINoteReadAndProjectItemWritePlan|TestLocalRPCProjectBoardNoteAndProjectItemPlan|TestLocalAPIDatabaseTaskAndGraphCapabilities|TestLocalRPCDatabaseTaskAndGraphCapabilities|TestReadonlyDashboardServesDatabaseTabProjection|TestReadonlyMCPQueryAndDatabaseView|TestProjectBoardWorkspace|TestUnifiedWorkspace|TestObsidianCompat|TestCloud|TestSyncDaemon|TestPluginRuntime|TestDataviewDatabase|TestPublishProfile|TestPublishStaticSite|TestPublishDoc|TestShareLANReadOnly|BidirectionalLinks|JournalIndexTemplate|StarterTemplates|IndexPageRefresh|TemplateRecommend|TemplateCompletion|TemplateNextAction|TestProofLoop|TestServerTransportTwoDeviceConvergence|TestServerTransportConflictPreservesBothSides|TestServerTransportNeverRemoteWriteBeforeCommit|TestClientBootstrapPrincipalAndVaultLifecycle|TestVersionRestoreApplyRevertsBadLocalApply|TestProofLoopRunPreviewEmitsRunIDAndStageFacts|TestProofLoopRunContractAcrossModes|TestPromptImportSearchShowResolveCommands|TestPromptLifecycleAndFeedbackCommands|TestMemoryCaptureListRecallAndContext|TestMemoryRecallRankingSignalsAndRedaction|TestReleaseCore|TestMCPReleaseCore|TestProofLoopReleaseCoreFiveMinute|TestAPIRoutesJSONExposesReleaseCore|TestPipelineUnifiedE2E", "-count=1"}
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
	if profile == "notebook-quality" {
		return evidence.Config{RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"), Command: []string{"go", "test", "./cmd/pinax", "./internal/cli", "./internal/output", "./tests/e2e", "-run", "Completion|TestCLITreeHelpSmoke|TestPromptCatalogCommandTreeIsAdditive|TestBackendProviderCLI|TestProjectAndStorageCLIJSON|TestProjectSubprojectWorkspaceCLI|TestPipelineStatus|TestBackendNotesCommands|TestRenderRunSnapshot|TestSummaryRendersProject|TestNarrowSummary|TestResource", "-count=1"}, PassThroughStdout: stdout, PassThroughStderr: stderr, PassStatus: "passed", Layer: "component", ExtraChecks: map[string]any{"profile": profile, "fixture_only": true, "remote_write": false, "race_detector": false}}
	}
	if profile == "notebook-backend-packages" {
		return evidence.Config{RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"), Command: []string{"go", "test", "-timeout=10m", "./internal/app/...", "./internal/api", "./internal/inputrequests", "./internal/mcpserver/...", "./internal/operation", "./pkg/pinaxclient", "./pkg/inputcontract", "./tools/testkit/integrationevidence", "-count=1"}, PassThroughStdout: stdout, PassThroughStderr: stderr, PassStatus: "passed", Layer: "component", ExtraChecks: map[string]any{"profile": profile, "fixture_only": true, "real_user_evaluations": 0, "model_calls": 0, "remote_write": false, "race_detector": false}}
	}

	if profile == "notebook-backend" || profile == "notebook-backend-final" {
		command := []string{"go", "test", "./internal/app", "./internal/api", "./internal/inputrequests", "./internal/mcpserver", "./internal/mcpserver/sdkruntime", "./internal/cli", "./pkg/pinaxclient", "./pkg/inputcontract", "./tests/e2e", "-run", "TestInputAttachment|TestInputIntake|TestSyncOperation|TestSyncDaemonRunPerformsStartupCycleBeforeFirstTick|TestSyncLockStalePidRecovery|TestCollaborationConcurrentPreviewsDoNotLoseUpdate|TestLocalRPCSync|TestSyncControl|TestRemoteMutationRecoverySync|TestOpenAPI", "-count=1", "-v"}
		if profile == "notebook-backend-final" {
			command = []string{"task", "check"}
		}
		return evidence.Config{RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"), Command: command, PassThroughStdout: stdout, PassThroughStderr: stderr, PassStatus: "passed", Layer: "e2e", ExtraChecks: map[string]any{"profile": profile, "fixture_only": true, "real_user_evaluations": 0, "model_calls": 0, "remote_write": false, "race_detector": false}}
	}

	if profile == "notebook-daily" {
		return evidence.Config{RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"), Command: []string{"go", "test", "./internal/transportcatalog/...", "./internal/templateengine", "./internal/operation", "./internal/app", "./internal/api", "./pkg/pinaxclient", "-run", "TestWriterDaily|TestDailyBounded|TestReconcile|TestWriterContract|TestOpenAPI", "-count=1", "-v"}, PassThroughStdout: stdout, PassThroughStderr: stderr, PassStatus: "passed", Layer: "component", ExtraChecks: map[string]any{"profile": profile, "fixture_only": true, "real_user_evaluations": 0, "model_calls": 0, "remote_write": false, "race_detector": false}}
	}
	if profile == "notebook-navigation" {
		return evidence.Config{RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"), Command: []string{"go", "test", "./internal/transportcatalog/...", "./internal/app", "./internal/api", "./pkg/pinaxclient", "-run", "TestReaderNavigation|TestReaderContract|TestReaderAccess|TestReaderResources|TestOpenAPI", "-count=1", "-v"}, PassThroughStdout: stdout, PassThroughStderr: stderr, PassStatus: "passed", Layer: "component", ExtraChecks: map[string]any{"profile": profile, "fixture_only": true, "real_user_evaluations": 0, "model_calls": 0, "remote_write": false, "race_detector": false}}
	}
	if profile == "notebook-editor-source" {
		return evidence.Config{RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"), Command: []string{"go", "test", "./internal/transportcatalog/...", "./internal/app", "./internal/api", "./pkg/pinaxclient", "-run", "TestWriterEditorSource|TestWriterContract|TestWriterEdit|TestWriterClient|TestOpenAPI|TestTagNote|TestWriterTagsPlan|TestWriterTagsApply|TestWriterTagsLookup|TestWriterTagsHTTP|TestWriterTagsClient|TestCollaboration|TestNoteHardeningFrontmatterPatch", "-count=1", "-v"}, PassThroughStdout: stdout, PassThroughStderr: stderr, PassStatus: "passed", Layer: "component", ExtraChecks: map[string]any{"profile": profile, "fixture_only": true, "real_user_evaluations": 0, "model_calls": 0, "remote_write": false, "race_detector": false}}
	}
	if isHumanReadingProfile(profile) {
		return buildHumanReadingConfig(profile, runID, stdout, stderr)
	}
	if isHumanReadingWriteProfile(profile) {
		return buildHumanReadingWriteConfig(profile, runID, stdout, stderr)
	}
	if profile == "daily-review-followthrough" {
		return evidence.Config{RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"), Command: []string{"go", "test", "./internal/app", "./internal/cli", "./internal/mcpserver", "./internal/mcpserver/sdkruntime", "./cmd/pinax", "./tests/e2e", "-run", "TestScopedOrganize|TestAgentNoteProposalQuery|TestDailyReviewFollowthrough|TestNoteProposalToolsQuery|TestSDKNoteProposalQuery|TestOrganize|TestOpenAPISemanticValidation", "-count=1", "-v"}, PassThroughStdout: stdout, PassThroughStderr: stderr, PassStatus: "passed", Layer: "e2e", ExtraChecks: map[string]any{"profile": profile, "fixture_only": true, "real_user_evaluations": 0, "model_calls": 0, "remote_write": false}}
	}
	if profile == "agent-note-proposal" || profile == "rendered-view-parity" {
		filter := "TestAgentNoteProposal|TestNoteProposalValidation|TestNoteProposalTools|TestSDKNoteProposal|TestNoteProposalWorkflow|TestNoteProposalTransportParity"
		if profile == "rendered-view-parity" {
			filter = "TestRendered|TestRenderNoteTool|TestSDKNoteProposal|TestNoteProposalWorkflow"
		}
		return evidence.Config{RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"), Command: []string{"go", "test", "./internal/agentprotocol", "./internal/agentmemory", "./internal/render", "./internal/app", "./internal/cli", "./internal/mcpserver", "./internal/mcpserver/sdkruntime", "./tests/e2e", "-run", filter, "-count=1", "-v"}, PassThroughStdout: stdout, PassThroughStderr: stderr, PassStatus: "passed", Layer: "e2e", ExtraChecks: map[string]any{"profile": profile, "fixture_only": true, "real_user_evaluations": 0, "model_calls": 0, "remote_write": false}}
	}
	if profile == "daily-knowledge-quality" {
		return evidence.Config{
			RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"),
			Command:           []string{"task", "check"},
			PassThroughStdout: stdout, PassThroughStderr: stderr,
			PassStatus: "passed", Layer: "system",
			ExtraChecks: map[string]any{"fixture_only": true, "real_user_evaluations": 0},
		}
	}
	if profile == "daily-knowledge" {
		return evidence.Config{
			RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"),
			Command:           []string{"go", "test", "./internal/app", "./internal/cli", "./cmd/pinax", "./tests/e2e", "-run", "TestDailyKnowledge|TestSearchShowCardCLI|TestVersionRestoreApplyRefusesStalePlan|TestVersionRestoreApplyRevertsBadLocalApply|TestResearchBriefWorkflow", "-count=1"},
			PassThroughStdout: stdout, PassThroughStderr: stderr,
			PassStatus: "passed", Layer: "e2e",
			ExtraChecks: map[string]any{"fixture_only": true, "real_user_evaluations": 0, "model_calls": 0},
		}
	}
	if profile == "mcp-collaboration" || profile == "mcp-collaboration-quality" || profile == "mcp-collaboration-race" || profile == "mcp-collaboration-go" || profile == "mcp-strict-client" || profile == "mcp-official-sdk" || profile == "mcp-inspector" {
		return collaborationEvidenceConfig(profile, runID, stdout, stderr)
	}
	if profile == "research-brief" {
		return evidence.Config{
			RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"),
			Command:           []string{"go", "test", "./internal/app/searchops", "./internal/app", "./tests/e2e", "-run", "TestFirstSnippetUnicode|TestResearchBriefWorkflow|TestMonitorRecordsSearchStepsAndActivitySource|Test.*(Link|Orphan|GraphSummary)", "-count=1"},
			PassThroughStdout: stdout, PassThroughStderr: stderr,
			PassStatus: "passed", Layer: "e2e",
			// 测试成功只证明合成流程；真实检索效果和人工采纳必须另行观察。
			ExtraChecks: map[string]any{"fixture_only": true, "real_user_evaluations": 0, "model_calls": 0},
		}
	}
	if profile == "knowledge-source-adapter" {
		return buildKnowledgeSourceAdapterConfig(runID, stdout, stderr)
	}
	if profile == "pane" {
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
	if profile == "personal-assistant-grounding" {
		return buildPersonalAssistantGroundingConfig(runID, stdout, stderr)
	}
	if profile == "prompt-catalog" {
		return buildPromptCatalogConfig(runID, stdout, stderr)
	}
	if profile == "mcp-protocol" {
		return buildMCPProtocolConfig(runID, stdout, stderr)
	}
	if profile == "sdk-security" {
		return buildSDKSecurityConfig(runID, stdout, stderr)
	}
	if profile == "operation-recovery" {
		return buildOperationRecoveryConfig(runID, stdout, stderr)
	}
	if profile == "remote-write-parity" {
		return buildRemoteWriteParityConfig(runID, stdout, stderr)
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

func buildRemoteWriteParityConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	return evidence.Config{
		RunID:     runID,
		ParentDir: filepath.Join("temp", "integration-test-runs"),
		Command: []string{
			"go", "test", "./internal/transportcatalog/...", "./internal/app", "./internal/api", "./internal/cli", "./pkg/pinaxclient", "./cmd/pinax",
			"-run", "RemoteWriteInteractionMetadata|TypedRemoteWriteMethods|InboxCapture|FolderRename|Writer|RemoteTransportManifest|OpenAPI|OperationRecovery|RemoteMutationRecovery|RemoteMode",
			"-count=1", "-v",
		},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		PassStatus:        "passed",
		Layer:             "e2e",
		ExtraChecks: map[string]any{
			"manifest_metadata_parity":     true,
			"remote_cli_write_contract":    true,
			"operation_identity_preserved": true,
			"revision_and_snapshot_gates":  true,
			"no_blind_retry":               true,
			"mcp_readonly_boundary":        true,
			"fixture_only":                 true,
			"real_user_evaluations":        0,
			"model_calls":                  0,
			"remote_write":                 false,
		},
	}
}

func buildKnowledgeSourceAdapterConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./internal/app/knowledgeops", "./cmd/pinax", "-run", "Knowledge", "-count=1"},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		PassStatus:        "passed",
		Layer:             "component",
		ExtraChecks: map[string]any{
			"default_empty_allowlist": true,
			"dual_condition_export":   true,
			"tombstone_incremental":   true,
			"vault_readonly":          true,
		},
	}
}

func buildPersonalAssistantGroundingConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	return evidence.Config{
		RunID:     runID,
		ParentDir: filepath.Join("temp", "integration-test-runs"),
		Command: []string{
			"go", "test",
			"./internal/agentcontext",
			"./internal/agentmemory",
			"./internal/app",
			"./internal/mcpserver",
			"./tests/e2e",
			"-run", "PersonalAssistant|ProposalSources|AgentMemoryTransportParity",
			"-count=1",
		},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		PassStatus:        "passed",
		Layer:             "e2e",
		ExtraChecks: map[string]any{
			"four_state_grounding":           true,
			"canonical_source_invariants":    true,
			"proposal_source_round_trip":     true,
			"deleted_source_suppression":     true,
			"delete_index_refresh":           true,
			"mcp_additive_contract":          true,
			"canonical_memory_preserved":     true,
			"transport_parity_valid_sources": true,
		},
	}
}

func buildMCPProtocolConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./internal/mcpserver", "./internal/cli", "./cmd/pinax", "./tests/e2e", "-run", "MCPProtocolLifecycle|MCPTransportParity|Stdout|Stderr|JSONRPC|Signal|Panic|ToolSchema|StructuredContent|ResourceTemplate|ModernProtocol|ProtocolNegotiation|InitializeAcceptsLegacy", "-count=1"},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
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
	}
}

func buildSDKSecurityConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./pkg/pinaxclient", "./internal/remoteapi", "./internal/connection", "./internal/cli", "./cmd/pinax", "-run", "TokenFile|BaseURL|Timeout|Redirect|ResponseLimit|Malformed2xx|TypedError|Manifest|Readiness|Operation|RemoteModeTokenSources|Connection", "-count=1"},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		PassStatus:        "passed",
		Layer:             "component",
		ExtraChecks: map[string]any{
			"secure_token_file":        true,
			"owner_mode_resolution":    true,
			"redirect_rejected":        true,
			"bounded_io_and_timeout":   true,
			"typed_error_redaction":    true,
			"compatibility_facade":     true,
			"credential_header_only":   true,
			"absolute_path_not_logged": true,
		},
	}
}

func buildOperationRecoveryConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./pkg/pinaxclient", "./internal/remoteapi", "./internal/app", "./internal/cli", "./tests/e2e", "-run", "Ambiguous|NoBlindRetry|ReplaySafe|RemoteMutationRecovery|OperationReconcile|OperationShow", "-count=1"},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		PassStatus:        "passed",
		Layer:             "e2e",
		ExtraChecks: map[string]any{
			"response_loss_single_apply": true,
			"finalize_crash_reconcile":   true,
			"same_operation_identity":    true,
			"no_blind_mutation_retry":    true,
			"evidence_redacted":          true,
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

// buildPromptCatalogConfig 构造 prompt repository import 的 integration evidence profile。
func buildPromptCatalogConfig(runID string, stdout, stderr io.Writer) evidence.Config {
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           []string{"go", "test", "./internal/promptbridge/...", "./internal/architecture", "./internal/cli", "./internal/app", "./internal/promptasset", "./cmd/pinax", "./tests/e2e", "-run", "PromptCatalog|PromptBridge|PromptRepository|PromptInstall", "-count=1"},
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		ExtraChecks: map[string]any{
			"repository_shared_store":  true,
			"catalog_provider_free":    true,
			"rights_gated_install":     true,
			"conflict_plan":            true,
			"effective_scope_policy":   true,
			"source_adapters_offline":  true,
			"catalog_quarantine":       true,
			"body_input_secret_backup": true,
		},
	}
}

// isHumanReadingProfile reports the pinax-human-reading-api-v1 evidence
// profiles registered by P01: focused lanes for P02-P06 plus the P07
// component run and the P08 quality gate.
func isHumanReadingProfile(profile string) bool {
	switch profile {
	case "human-reading-api", "human-reading-access", "human-reading-document",
		"human-reading-resources", "human-reading-session", "human-reading-client", "human-reading-quality":
		return true
	}
	return false
}

// buildHumanReadingConfig wires each focused reader lane to the shared
// contract test plus its implementation tests. Until P02-P06 land, the
// contract cases keep every profile non-empty; lanes must never fabricate a
// passing run with zero matched tests. No race, no real vault, no model, no
// sync — synthetic fixtures only.
func buildHumanReadingConfig(profile, runID string, stdout, stderr io.Writer) evidence.Config {
	base := []string{"go", "test"}
	contract := "TestReaderContract"
	layer := "component"
	var packages []string
	var filter string
	switch profile {
	case "human-reading-access":
		packages = []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api"}
		filter = contract + "|TestReaderAccess"
	case "human-reading-document":
		packages = []string{"./internal/transportcatalog/...", "./internal/app", "./internal/render", "./internal/api", "./pkg/pinaxclient"}
		filter = contract + "|TestReaderDocument"
	case "human-reading-resources":
		packages = []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api"}
		filter = contract + "|TestReaderResources"
	case "human-reading-session":
		packages = []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api", "./internal/cli", "./cmd/pinax", "./tests/e2e", "./pkg/localsession", "./pkg/pinaxclient", "./internal/index", "./internal/sqlitedsn"}
		filter = contract + "|TestReaderSession|TestInspectReadOnly|TestReadSnapshot"
	case "human-reading-client":
		packages = []string{"./internal/transportcatalog/...", "./pkg/pinaxclient", "./internal/remoteapi"}
		filter = contract + "|TestReader|TestRemoteTransport"
	case "human-reading-api":
		packages = []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api", "./tests/e2e"}
		filter = contract + "|TestReaderComponent|TestReaderE2E"
	case "human-reading-quality":
		// P08 stable gate: task check has no race stage; verify explicitly before
		// running when in doubt. Complete logs stay with the owner.
		return evidence.Config{
			RunID:             runID,
			ParentDir:         filepath.Join("temp", "integration-test-runs"),
			Command:           []string{"task", "check"},
			PassThroughStdout: stdout,
			PassThroughStderr: stderr,
			PassStatus:        "passed",
			Layer:             "system",
			ExtraChecks: map[string]any{
				"profile":                   profile,
				"fixture_only":              true,
				"real_user_evaluations":     0,
				"model_calls":               0,
				"remote_write":              false,
				"race_detector":             false,
				"reader_bindings_available": true,
			},
		}
	}
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           append(append(base, packages...), "-run", filter, "-count=1"),
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		PassStatus:        "passed",
		Layer:             layer,
		ExtraChecks: map[string]any{
			"profile":                   profile,
			"fixture_only":              true,
			"real_user_evaluations":     0,
			"model_calls":               0,
			"remote_write":              false,
			"race_detector":             false,
			"reader_bindings_available": true,
		},
	}
}

// isHumanReadingWriteProfile reports the pinax-human-reading-write-v1
// evidence profiles registered by W01: focused lanes for W02-W04 plus the
// W05 client lane, the W06 component run and the W07 quality gate.
func isHumanReadingWriteProfile(profile string) bool {
	switch profile {
	case "human-reading-write-capture", "human-reading-write-edit",
		"human-reading-write-organize", "human-reading-write-client",
		"human-reading-write-api", "human-reading-write-quality":
		return true
	}
	return false
}

// buildHumanReadingWriteConfig wires each focused writer lane to the frozen
// writer contract test plus its implementation tests. Until W02-W04 land,
// the contract cases keep every profile non-empty; lanes must never
// fabricate a passing run with zero matched tests. No race, no real vault,
// no model, no sync — synthetic fixtures only.
func buildHumanReadingWriteConfig(profile, runID string, stdout, stderr io.Writer) evidence.Config {
	base := []string{"go", "test"}
	contract := "TestWriterContract"
	layer := "component"
	var packages []string
	var filter string
	switch profile {
	case "human-reading-write-capture":
		packages = []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api"}
		filter = contract + "|TestWriterCapture"
	case "human-reading-write-edit":
		packages = []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api"}
		filter = contract + "|TestWriterEdit"
	case "human-reading-write-organize":
		packages = []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api"}
		filter = contract + "|TestWriterOrganize"
	case "human-reading-write-client":
		packages = []string{"./internal/transportcatalog/...", "./pkg/pinaxclient", "./internal/remoteapi"}
		filter = contract + "|TestWriterClient"
	case "human-reading-write-api":
		packages = []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api", "./tests/e2e"}
		filter = contract + "|TestWriterComponent|TestWriterE2E"
	case "human-reading-write-quality":
		// W07 stable gate: task check has no race stage; verify explicitly
		// before running when in doubt. Complete logs stay with the owner.
		return evidence.Config{
			RunID:             runID,
			ParentDir:         filepath.Join("temp", "integration-test-runs"),
			Command:           []string{"task", "check"},
			PassThroughStdout: stdout,
			PassThroughStderr: stderr,
			PassStatus:        "passed",
			Layer:             "system",
			ExtraChecks: map[string]any{
				"profile":                   profile,
				"fixture_only":              true,
				"real_user_evaluations":     0,
				"model_calls":               0,
				"remote_write":              false,
				"race_detector":             false,
				"writer_bindings_available": true,
			},
		}
	}
	return evidence.Config{
		RunID:             runID,
		ParentDir:         filepath.Join("temp", "integration-test-runs"),
		Command:           append(append(base, packages...), "-run", filter, "-count=1"),
		PassThroughStdout: stdout,
		PassThroughStderr: stderr,
		PassStatus:        "passed",
		Layer:             layer,
		ExtraChecks: map[string]any{
			"profile":                   profile,
			"fixture_only":              true,
			"real_user_evaluations":     0,
			"model_calls":               0,
			"remote_write":              false,
			"race_detector":             false,
			"writer_bindings_available": true,
		},
	}
}
