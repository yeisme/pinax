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
	current := runMCPProcess(t, vault, []string{
		`{"jsonrpc":"2.0","id":"discover","method":"server/discover","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"call","method":"tools/call","params":{"name":"pinax.git.snapshot_plan","arguments":{},"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"templates","method":"resources/templates/list","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"manifest","method":"resources/read","params":{"uri":"pinax://manifest","_meta":` + currentMCPMeta + `}}`,
	})
	if len(current) != 6 {
		t.Fatalf("current responses = %d, want 6", len(current))
	}
	assertMCPResultType(t, current, "discover")
	assertMCPResultType(t, current, "tools")
	assertMCPResultType(t, current, "call")
	assertMCPResultType(t, current, "resources")
	assertMCPResultType(t, current, "templates")
	assertMCPResultType(t, current, "manifest")
	call := responseResult(t, current, "call")
	structured, _ := call["structuredContent"].(map[string]any)
	if structured["schema_version"] != "pinax.mcp.tool_result.v1" || structured["status"] != "success" {
		t.Fatalf("current call structured content = %#v", structured)
	}

	legacy := runMCPProcess(t, vault, []string{
		`{"jsonrpc":"2.0","id":"initialize","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"pinax-e2e","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{}}`,
		`{"jsonrpc":"2.0","id":"manifest","method":"resources/read","params":{"uri":"pinax://manifest"}}`,
	})
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
}

func TestMCPTransportParityWithAuthoritativeManifest(t *testing.T) {
	t.Parallel()

	vault := t.TempDir()
	responses := runMCPProcess(t, vault, []string{
		`{"jsonrpc":"2.0","id":"tools","method":"tools/list","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"resources","method":"resources/list","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"templates","method":"resources/templates/list","params":{"_meta":` + currentMCPMeta + `}}`,
		`{"jsonrpc":"2.0","id":"manifest","method":"resources/read","params":{"uri":"pinax://manifest","_meta":` + currentMCPMeta + `}}`,
	})
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

func runMCPProcess(t *testing.T, vault string, frames []string) []map[string]any {
	t.Helper()
	cmd := exec.Command(filepath.Join(sharedBinDir, "pinax"), "mcp", "serve", "--vault", vault)
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
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
