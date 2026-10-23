package main

import (
	"io"
	"strings"
	"testing"
)

func TestBuildNotebookDailyProfile(t *testing.T) {
	config := buildConfigForProfile("notebook-daily", "daily-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"./internal/transportcatalog/...", "./internal/templateengine", "./internal/operation", "./internal/app", "./internal/api", "./pkg/pinaxclient", "TestWriterDaily", "TestDailyBounded", "TestReconcile", "TestWriterContract", "TestOpenAPI"} {
		if !strings.Contains(command, required) {
			t.Fatalf("missing check: %s", required)
		}
	}
	if strings.Contains(command, "-race") || config.Layer != "component" || config.ExtraChecks["fixture_only"] != true || config.ExtraChecks["remote_write"] != false {
		t.Fatal("unsafe daily profile")
	}
}

func TestBuildNotebookNavigationProfile(t *testing.T) {
	config := buildConfigForProfile("notebook-navigation", "navigation-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api", "./pkg/pinaxclient", "TestReaderNavigation", "TestReaderContract", "TestOpenAPI"} {
		if !strings.Contains(command, required) {
			t.Fatalf("missing check: %s", required)
		}
	}
	if strings.Contains(command, "-race") || config.Layer != "component" || config.ExtraChecks["fixture_only"] != true || config.ExtraChecks["remote_write"] != false {
		t.Fatal("unsafe navigation profile")
	}
}

func TestBuildNotebookEditorSourceProfile(t *testing.T) {
	config := buildConfigForProfile("notebook-editor-source", "editor-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"./internal/transportcatalog/...", "./internal/app", "./internal/api", "./pkg/pinaxclient", "TestWriterEditorSource", "TestWriterContract", "TestOpenAPI"} {
		if !strings.Contains(command, required) {
			t.Fatalf("missing check: %s", required)
		}
	}
	if strings.Contains(command, "-race") || config.ParentDir != "temp/integration-test-runs" || config.Layer != "component" || config.ExtraChecks["fixture_only"] != true || config.ExtraChecks["remote_write"] != false {
		t.Fatal("unsafe notebook profile")
	}
}

func TestBuildDailyReviewFollowthroughProfile(t *testing.T) {
	config := buildConfigForProfile("daily-review-followthrough", "review-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"TestScopedOrganize", "TestAgentNoteProposalQuery", "TestDailyReviewFollowthrough", "TestSDKNoteProposalQuery", "./tests/e2e"} {
		if !strings.Contains(command, required) {
			t.Fatalf("missing check: %s", required)
		}
	}
	if strings.Contains(command, "-race") || config.ParentDir != "temp/integration-test-runs" || config.ExtraChecks["remote_write"] != false || config.ExtraChecks["fixture_only"] != true {
		t.Fatal("unsafe profile")
	}
}

func TestBuildConfigIncludesPublishEvidenceEntrypoint(t *testing.T) {
	config := buildConfig("test-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	if !strings.Contains(command, "./tests/e2e") || !strings.Contains(command, "TestPublishProfile") || !strings.Contains(command, "TestPublishStaticSite") || !strings.Contains(command, "TestPublishDoc") || !strings.Contains(command, "TestShareLANReadOnly") {
		t.Fatalf("integration evidence command does not include publish e2e entrypoint: %s", command)
	}
	if config.ParentDir != "temp/integration-test-runs" {
		t.Fatalf("parent dir = %q", config.ParentDir)
	}
	if config.ExtraChecks["publish_static_profile"] != true {
		t.Fatalf("publish_static_profile check missing: %#v", config.ExtraChecks)
	}
	if config.ExtraChecks["publish_static_site"] != true {
		t.Fatalf("publish_static_site check missing: %#v", config.ExtraChecks)
	}
	if config.ExtraChecks["publish_doc"] != true {
		t.Fatalf("publish_doc check missing: %#v", config.ExtraChecks)
	}
	if config.ExtraChecks["share_lan_readonly"] != true {
		t.Fatalf("share_lan_readonly check missing: %#v", config.ExtraChecks)
	}
}

func TestBuildIdentityConfigUsesRequiredEvidenceDirectoryAndEntrypoint(t *testing.T) {
	config := buildConfigForProfile("identity", "identity-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	if !strings.Contains(command, "IdentityFirstTwoDeviceKernel") || !strings.Contains(command, "ManifestMigration") {
		t.Fatalf("identity evidence command = %s", command)
	}
	if config.ParentDir != "temp/integration-test-runs" || config.ExtraChecks["canonical_object_identity"] != true {
		t.Fatalf("config = %#v", config)
	}
}

func TestBuildKnowledgeSourceAdapterProfile(t *testing.T) {
	config := buildConfigForProfile("knowledge-source-adapter", "knowledge-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	if !strings.Contains(command, "./internal/app/knowledgeops") || !strings.Contains(command, "./cmd/pinax") || !strings.Contains(command, "Knowledge") {
		t.Fatalf("knowledge evidence command = %s", command)
	}
	if config.ParentDir != "temp/integration-test-runs" || config.Layer != "component" || config.ExtraChecks["vault_readonly"] != true {
		t.Fatalf("config = %#v", config)
	}
}

func TestBuildPaneProfile(t *testing.T) {
	config := buildConfigForProfile("pane", "pane-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	if !strings.Contains(command, "./internal/app") || !strings.Contains(command, "Pane") {
		t.Fatalf("pane evidence command = %s", command)
	}
	if config.Layer != "component" || config.ExtraChecks["handwritten_metadata_rejected"] != true {
		t.Fatalf("config = %#v", config)
	}
}

func TestBuildIdentityBenchmarkProfile(t *testing.T) {
	config := buildConfigForProfile("identity-benchmark", "benchmark-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"-bench", "Identity|IndexV2|LinkGraph|ObjectSync", "-benchmem"} {
		if !strings.Contains(command, required) {
			t.Fatalf("benchmark command %q missing %q", command, required)
		}
	}
	if config.ExtraChecks["link_graph_100k"] != true {
		t.Fatalf("benchmark checks = %#v", config.ExtraChecks)
	}
}

func TestBuildOperationRecoveryProfile(t *testing.T) {
	config := buildConfigForProfile("operation-recovery", "operation-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"./pkg/pinaxclient", "./tests/e2e", "RemoteMutationRecovery", "OperationReconcile"} {
		if !strings.Contains(command, required) {
			t.Fatalf("operation recovery command %q missing %q", command, required)
		}
	}
	if config.ParentDir != "temp/integration-test-runs" || config.Layer != "e2e" || config.PassStatus != "passed" || config.ExtraChecks["same_operation_identity"] != true {
		t.Fatalf("operation recovery config = %#v", config)
	}
}

func TestBuildMCPProtocolProfile(t *testing.T) {
	config := buildConfigForProfile("mcp-protocol", "mcp-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"./internal/mcpserver", "./tests/e2e", "MCPProtocolLifecycle", "MCPTransportParity", "InitializeAcceptsLegacy"} {
		if !strings.Contains(command, required) {
			t.Fatalf("MCP protocol command %q missing %q", command, required)
		}
	}
	if config.Layer != "e2e" || config.PassStatus != "passed" || config.ExtraChecks["stdout_jsonrpc_only"] != true {
		t.Fatalf("MCP protocol config = %#v", config)
	}
}

func TestBuildSDKSecurityProfile(t *testing.T) {
	config := buildConfigForProfile("sdk-security", "sdk-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"./pkg/pinaxclient", "./internal/remoteapi", "TokenFile", "Redirect", "RemoteModeTokenSources"} {
		if !strings.Contains(command, required) {
			t.Fatalf("SDK security command %q missing %q", command, required)
		}
	}
	if config.Layer != "component" || config.PassStatus != "passed" || config.ExtraChecks["secure_token_file"] != true {
		t.Fatalf("SDK security config = %#v", config)
	}
}

func TestBuildPersonalAssistantGroundingProfile(t *testing.T) {
	config := buildConfigForProfile("personal-assistant-grounding", "pa-grounding-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	for _, required := range []string{"./internal/agentcontext", "./internal/agentmemory", "./internal/app", "./internal/mcpserver", "./tests/e2e", "PersonalAssistant", "AgentMemoryTransportParity"} {
		if !strings.Contains(command, required) {
			t.Fatalf("Personal Assistant grounding command %q missing %q", command, required)
		}
	}
	if config.ParentDir != "temp/integration-test-runs" || config.Layer != "e2e" || config.PassStatus != "passed" || config.ExtraChecks["deleted_source_suppression"] != true {
		t.Fatalf("Personal Assistant grounding config = %#v", config)
	}
}

func TestBuildHumanReadingProfiles(t *testing.T) {
	laneFilters := map[string]string{
		"human-reading-access":    "TestReaderAccess",
		"human-reading-document":  "TestReaderDocument",
		"human-reading-resources": "TestReaderResources",
		"human-reading-session":   "TestReaderSession",
		"human-reading-client":    "TestRemoteTransport",
		"human-reading-api":       "TestReaderComponent",
	}
	for profile, laneFilter := range laneFilters {
		config := buildConfigForProfile(profile, "reader-run", io.Discard, io.Discard)
		command := strings.Join(config.Command, " ")
		if !strings.Contains(command, "TestReaderContract") || !strings.Contains(command, laneFilter) {
			t.Fatalf("%s command %q missing contract test or lane filter %q", profile, command, laneFilter)
		}
		if !strings.Contains(command, "./internal/transportcatalog/...") {
			t.Fatalf("%s command %q must anchor on the transport catalog contract", profile, command)
		}
		if strings.Contains(command, "-race") || strings.Contains(command, "-count=2") {
			t.Fatalf("%s unsafe command %q", profile, command)
		}
		if config.ParentDir != "temp/integration-test-runs" || config.Layer != "component" || config.PassStatus != "passed" {
			t.Fatalf("%s config = %#v", profile, config)
		}
		if config.ExtraChecks["fixture_only"] != true || config.ExtraChecks["remote_write"] != false || config.ExtraChecks["model_calls"] != 0 || config.ExtraChecks["race_detector"] != false || config.ExtraChecks["reader_bindings_available"] != true {
			t.Fatalf("%s unsafe extra checks = %#v", profile, config.ExtraChecks)
		}
	}
}

func TestBuildHumanReadingQualityProfile(t *testing.T) {
	config := buildConfigForProfile("human-reading-quality", "reader-quality-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	if command != "task check" {
		t.Fatalf("human-reading-quality command = %q, want task check", command)
	}
	if strings.Contains(command, "race") {
		t.Fatal("human-reading-quality must not run the race detector")
	}
	if config.ParentDir != "temp/integration-test-runs" || config.Layer != "system" || config.PassStatus != "passed" || config.ExtraChecks["fixture_only"] != true || config.ExtraChecks["remote_write"] != false {
		t.Fatalf("human-reading-quality config = %#v", config)
	}
}

func TestBuildHumanReadingWriteProfiles(t *testing.T) {
	laneFilters := map[string]string{
		"human-reading-write-capture":  "TestWriterCapture",
		"human-reading-write-edit":     "TestWriterEdit",
		"human-reading-write-organize": "TestWriterOrganize",
		"human-reading-write-client":   "TestWriterClient",
		"human-reading-write-api":      "TestWriterComponent",
	}
	for profile, laneFilter := range laneFilters {
		config := buildConfigForProfile(profile, "writer-run", io.Discard, io.Discard)
		command := strings.Join(config.Command, " ")
		if !strings.Contains(command, "TestWriterContract") || !strings.Contains(command, laneFilter) {
			t.Fatalf("%s command %q missing contract test or lane filter %q", profile, command, laneFilter)
		}
		if !strings.Contains(command, "./internal/transportcatalog/...") {
			t.Fatalf("%s command %q must anchor on the transport catalog contract", profile, command)
		}
		if strings.Contains(command, "-race") || strings.Contains(command, "-count=2") {
			t.Fatalf("%s unsafe command %q", profile, command)
		}
		if config.ParentDir != "temp/integration-test-runs" || config.Layer != "component" || config.PassStatus != "passed" {
			t.Fatalf("%s config = %#v", profile, config)
		}
		if config.ExtraChecks["fixture_only"] != true || config.ExtraChecks["remote_write"] != false || config.ExtraChecks["model_calls"] != 0 || config.ExtraChecks["race_detector"] != false || config.ExtraChecks["writer_bindings_available"] != true {
			t.Fatalf("%s unsafe extra checks = %#v", profile, config.ExtraChecks)
		}
	}
}

func TestBuildHumanReadingWriteQualityProfile(t *testing.T) {
	config := buildConfigForProfile("human-reading-write-quality", "writer-quality-run", io.Discard, io.Discard)
	command := strings.Join(config.Command, " ")
	if command != "task check" {
		t.Fatalf("human-reading-write-quality command = %q, want task check", command)
	}
	if strings.Contains(command, "race") {
		t.Fatal("human-reading-write-quality must not run the race detector")
	}
	if config.ParentDir != "temp/integration-test-runs" || config.Layer != "system" || config.PassStatus != "passed" || config.ExtraChecks["fixture_only"] != true || config.ExtraChecks["remote_write"] != false {
		t.Fatalf("human-reading-write-quality config = %#v", config)
	}
}
