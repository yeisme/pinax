package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const currentMCPMeta = `{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

func TestMCPProtocolLifecycleCurrentAndLegacy(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	// pinax-mcp-official-sdk-v1 §4.4：默认 runtime 已切官方 SDK；legacy 私有
	// envelope 合同（含 server/discover）经 PINAX_MCP_RUNTIME=legacy 显式覆盖验证。
	legacyPrivate := runMCPProcessEnv(t, vault, []string{
		`{"jsonrpc":"2.0","id":"discover","method":"server/discover","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"call","method":"tools/call","params":{"name":"pinax.git.snapshot_plan","arguments":{},"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"templates","method":"resources/templates/list","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"manifest","method":"resources/read","params":{"uri":"pinax://manifest","_meta":` + currentMCPMeta + `}}`,
	}, []string{"PINAX_MCP_RUNTIME=legacy"})
	if len(legacyPrivate) != 6 {
		t.Fatalf("legacy private responses = %d, want 6", len(legacyPrivate))
	}
	assertMCPResultType(t, legacyPrivate, "discover")
	assertMCPResultType(t, legacyPrivate, "tools")
	assertMCPResultType(t, legacyPrivate, "call")
	assertMCPResultType(t, legacyPrivate, "resources")
	assertMCPResultType(t, legacyPrivate, "templates")
	assertMCPResultType(t, legacyPrivate, "manifest")
	call := responseResult(t, legacyPrivate, "call")
	structured, _ := call["structuredContent"].(map[string]any)
	if structured["schema_version"] != "pinax.mcp.tool_result.v1" || structured["status"] != "success" {
		t.Fatalf("current call structured content = %#v", structured)
	}

	legacy := runMCPProcessEnv(t, vault, []string{
		`{"jsonrpc":"2.0","id":"initialize","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"pinax-e2e","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"manifest","method":"resources/read","params":{"uri":"pinax://manifest"}}`,
	}, []string{"PINAX_MCP_RUNTIME=legacy"})
	if len(legacy) != 4 {
		t.Fatalf("legacy responses = %d, want 4", len(legacy))
	}
	initialize := responseResult(t, legacy, "initialize")
	if initialize["protocolVersion"] != "2025-11-25" || initialize["read_only"] != true {
		t.Fatalf("legacy initialize = %#v", initialize)
	}
	legacyTools, _ := responseResult(t, legacy, "tools")["tools"].([]any)
	legacyResources, _ := responseResult(t, legacy, "resources")["resources"].([]any)
	for _, id := range []string{"tools", "resources"} {
		for key := range responseByID(t, legacy, id) {
			if key != "jsonrpc" && key != "id" && key != "result" {
				t.Fatalf("non-standard JSON-RPC response field: %s", key)
			}
		}
	}
	if len(legacyTools) != 20 || len(legacyResources) != 9 {
		t.Fatalf("legacy inventory tools=%d resources=%d", len(legacyTools), len(legacyResources))
	}

	// 默认 runtime（无 env）= 官方 SDK：标准 initialize 流程可用，结果不含
	// legacy 私有 read_only envelope；私有 server/discover（带协议 _meta）经
	// Server.Handle 单点在两个 runtime 均可用（4.4 受控探针修正冻结表）。
	defaultRuntime, _ := runMCPPiped(t, vault, nil, []string{
		`{"jsonrpc":"2.0","id":"initialize","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"pinax-e2e-default","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"discover","method":"server/discover","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{}}`,
	}, 4)
	if len(defaultRuntime) != 4 {
		t.Fatalf("default runtime responses = %d, want 4", len(defaultRuntime))
	}
	if initialize := responseResult(t, defaultRuntime, "initialize"); initialize["protocolVersion"] != "2025-11-25" {
		t.Fatalf("default runtime initialize = %#v", initialize)
	}
	if _, ok := responseResult(t, defaultRuntime, "initialize")["read_only"]; ok {
		t.Fatalf("default runtime must not project legacy read_only envelope")
	}
	if discover := responseByID(t, defaultRuntime, "discover"); discover == nil || discover["result"] == nil {
		t.Fatalf("private server/discover with _meta must stay available on the default runtime: %#v", discover)
	}
	tools, _ := responseResult(t, defaultRuntime, "tools")["tools"].([]any)
	resources, _ := responseResult(t, defaultRuntime, "resources")["resources"].([]any)
	if len(tools) != 20 || len(resources) == 0 {
		t.Fatalf("default runtime inventory tools=%d resources=%d", len(tools), len(resources))
	}
}

func TestMCPTransportParityWithAuthoritativeManifest(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	responses, _ := runMCPPiped(t, vault, nil, []string{
		`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"pinax-e2e-parity","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"templates","method":"resources/templates/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"manifest","method":"resources/read","params":{"uri":"pinax://manifest"}}`,
	}, 5)
	responses = responses[1:] // 丢弃 initialize 响应，保持后续断言不变
	tools, _ := responseResult(t, responses, "tools")["tools"].([]any)
	resources, _ := responseResult(t, responses, "resources")["resources"].([]any)
	templates, _ := responseResult(t, responses, "templates")["resourceTemplates"].([]any)
	manifest := manifestFromMCPResponse(t, responses, "manifest")

	mcpTools := 0
	mcpResources := 0
	capabilities, _ := manifest["capabilities"].([]any)
	for _, rawCapability := range capabilities {
		capability, _ := rawCapability.(map[string]any)
		bindings, _ := capability["bindings"].([]any)
		for _, rawBinding := range bindings {
			binding, _ := rawBinding.(map[string]any)
			if binding["availability"] != "available" {
				continue
			}
			switch binding["transport"] {
			case "mcp_tool":
				mcpTools++
			case "mcp_resource":
				mcpResources++
			}
		}
	}
	if len(tools) != mcpTools || len(resources)+len(templates) != mcpResources {
		t.Fatalf("MCP parity tools discovery=%d manifest=%d resources discovery=%d manifest=%d", len(tools), mcpTools, len(resources)+len(templates), mcpResources)
	}

	apiManifest := runAPIManifest(t, vault)
	if manifest["schema_version"] != apiManifest["schema_version"] || manifest["digest"] != apiManifest["digest"] {
		t.Fatalf("manifest identity mismatch: mcp=%#v api=%#v", manifest["digest"], apiManifest["digest"])
	}
}

func runMCPProcessEnv(t *testing.T, vault string, frames []string, extraEnv []string) []map[string]any {
	t.Helper()
	cmd := exec.Command(filepath.Join(sharedBinDir, "pinax"), "mcp", "serve", "--vault", vault)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Stdin = strings.NewReader(strings.Join(frames, "\n") + "\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("pinax mcp serve failed: %v\nstderr=%s", err, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("pinax mcp serve emitted diagnostics on successful flow: %q", stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	responses := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil {
			t.Fatalf("non-JSON MCP stdout frame: %v\n%s", err, line)
		}
		if response["jsonrpc"] != "2.0" || response["error"] != nil {
			t.Fatalf("MCP response failure: %#v", response)
		}
		responses = append(responses, response)
	}
	return responses
}

func runAPIManifest(t *testing.T, vault string) map[string]any {
	t.Helper()
	cmd := exec.Command(filepath.Join(sharedBinDir, "pinax"), "api", "manifest", "--vault", vault, "--json")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("pinax api manifest failed: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	manifest, _ := data["manifest"].(map[string]any)
	if manifest["digest"] == nil {
		t.Fatalf("API manifest = %#v", manifest)
	}
	return manifest
}

func responseByID(t *testing.T, responses []map[string]any, id string) map[string]any {
	t.Helper()
	for _, response := range responses {
		if fmt.Sprint(response["id"]) == id {
			return response
		}
	}
	t.Fatalf("response %q not found", id)
	return nil
}

func responseResult(t *testing.T, responses []map[string]any, id string) map[string]any {
	t.Helper()
	result, _ := responseByID(t, responses, id)["result"].(map[string]any)
	if result == nil {
		t.Fatalf("response %q result is missing", id)
	}
	return result
}

func assertMCPResultType(t *testing.T, responses []map[string]any, id string) {
	t.Helper()
	if result := responseResult(t, responses, id); result["resultType"] != "complete" {
		t.Fatalf("response %q resultType = %#v", id, result["resultType"])
	}
}

func manifestFromMCPResponse(t *testing.T, responses []map[string]any, id string) map[string]any {
	t.Helper()
	result := responseResult(t, responses, id)
	contents, _ := result["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("manifest contents = %#v", result["contents"])
	}
	content, _ := contents[0].(map[string]any)
	text, _ := content["text"].(string)
	var resource map[string]any
	if err := json.Unmarshal([]byte(text), &resource); err != nil {
		t.Fatal(err)
	}
	data, _ := resource["data"].(map[string]any)
	manifest, _ := data["manifest"].(map[string]any)
	if manifest["digest"] == nil {
		t.Fatalf("MCP manifest = %#v", manifest)
	}
	return manifest
}
