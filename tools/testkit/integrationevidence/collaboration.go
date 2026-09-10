package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/yeisme/pinax/tools/testkit/evidence"
)

func collaborationEvidenceConfig(profile, runID string, stdout, stderr io.Writer) evidence.Config {
	command := []string{"go", "test", "./internal/app", "./internal/mcpserver", "./internal/cli", "-run", "Collaboration|InputIntake|ReadonlyMCP|ToolSchema|ResourceTemplate", "-count=1"}
	layer := "component"
	if profile == "mcp-strict-client" {
		command = []string{"node", "tools/testkit/mcpstrictclient.mjs", os.Getenv("PINAX_TEST_BINARY"), os.Getenv("PINAX_MCP_SDK_DIR")}
		layer = "e2e"
	}
	if profile == "mcp-collaboration-go" {
		command = []string{"go", "test", "-timeout=10m", "./..."}
		layer = "system"
	}
	if profile == "mcp-collaboration-quality" {
		command = []string{"task", "check"}
		layer = "system"
	}
	if profile == "mcp-collaboration-race" {
		command = []string{"go", "test", "-race", "./internal/app", "./internal/mcpserver", "./internal/operation", "-run", "Collaboration|Concurrent", "-count=1"}
	}
	if profile == "mcp-official-sdk" {
		// pinax-mcp-official-sdk-v1：candidate runtime 组件 + 进程级验收。
		// strict-client 变体（PINAX_MCP_RUNTIME=official-sdk + mcp-strict-client
		// profile）复用同一 evidence 管道，验证官方 TS SDK 客户端兼容。
		command = []string{"go", "test", "./internal/mcpserver/sdkruntime", "./tests/e2e", "-run", "TestSDKRuntime|TestMCPOfficialSDK", "-count=1"}
	}
	if profile == "mcp-inspector" {
		// pinax-mcp-official-sdk-v1 4.1：官方 Inspector CLI 自动验收。
		// PINAX_TEST_BINARY（默认 dist/pinax）、PINAX_INSPECTOR_VAULT、
		// PINAX_INSPECTOR_METHOD（tools/list|resources/list|tools/call）、
		// PINAX_INSPECTOR_TOOL 与 PINAX_MCP_RUNTIME 由调用方注入。
		binary := os.Getenv("PINAX_TEST_BINARY")
		if binary == "" {
			binary = "dist/pinax"
		}
		method := os.Getenv("PINAX_INSPECTOR_METHOD")
		if method == "" {
			method = "tools/list"
		}
		command = []string{"npx", "-y", "@modelcontextprotocol/inspector@latest", "--cli", binary, "mcp", "serve", "--vault", os.Getenv("PINAX_INSPECTOR_VAULT"), "--method", method}
		if tool := os.Getenv("PINAX_INSPECTOR_TOOL"); tool != "" {
			command = append(command, "--tool-name", tool)
		}
		layer = "e2e"
	}
	return evidence.Config{RunID: runID, ParentDir: filepath.Join("temp", "integration-test-runs"), Command: command, PassThroughStdout: stdout, PassThroughStderr: stderr, PassStatus: "passed", Layer: layer, ExtraChecks: map[string]any{"fixture_only": true, "codex_model_session_verified": false}}
}
