package main

import (
	"io"
	"strings"
	"testing"
)

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

func TestBuildDshPaneProfile(t *testing.T) {
	config := buildConfigForProfile("dsh-pane", "pane-run", io.Discard, io.Discard)
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
